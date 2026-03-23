package app

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
	"unicode"

	"codexswitch/internal/auth"
	"codexswitch/internal/buildinfo"
	"codexswitch/internal/codex"
	"codexswitch/internal/model"
	"codexswitch/internal/state"
	"codexswitch/internal/store"
	"codexswitch/internal/ui"
)

type viewMode int

const (
	viewNormal viewMode = iota
	viewSwitch
	viewDetail
	viewHelp
	viewDeleteConfirm
)

type styledLine struct {
	text     string
	style    string
	preserve bool
}

type refreshEvent struct {
	message string
	final   bool
}

const refreshWorkerLimit = 3

type App struct {
	store            *store.ProfileStore
	settings         model.AppSettings
	runtimeOverride  string
	targetCodexHome  string
	profiles         []model.StoredProfile
	selection        int
	status           string
	currentSnapshot  *model.AuthSnapshot
	currentQuota     *model.RateLimitSnapshot
	currentProfileID string
	mode             viewMode
	refreshing       bool
	refreshEvents    chan refreshEvent
	needsRedraw      bool
}

func New(profileStore *store.ProfileStore, targetOverride string) (*App, error) {
	app := &App{
		store:           profileStore,
		runtimeOverride: auth.ExpandPath(targetOverride),
		status:          "正在加载账号...",
		mode:            viewNormal,
		refreshEvents:   make(chan refreshEvent, 32),
		needsRedraw:     true,
	}
	if err := app.syncState(); err != nil {
		return nil, err
	}
	return app, nil
}

func (app *App) syncState() error {
	selectedProfileID := ""
	if app.selection >= 0 && app.selection < len(app.profiles) {
		selectedProfileID = app.profiles[app.selection].Meta.ProfileID
	}

	snapshot, err := state.Load(app.store, app.runtimeOverride)
	if err != nil {
		return err
	}

	app.settings = snapshot.Settings
	app.targetCodexHome = snapshot.TargetCodexHome
	app.profiles = snapshot.Profiles
	app.currentSnapshot = snapshot.CurrentSnapshot
	app.currentQuota = snapshot.CurrentQuota
	app.currentProfileID = snapshot.CurrentProfileID

	app.selection = 0
	if selectedProfileID != "" {
		for idx, profile := range app.profiles {
			if profile.Meta.ProfileID == selectedProfileID {
				app.selection = idx
				return nil
			}
		}
	}
	if app.currentProfileID != "" {
		for idx, profile := range app.profiles {
			if profile.Meta.ProfileID == app.currentProfileID {
				app.selection = idx
				return nil
			}
		}
	}
	return nil
}

func (app *App) Run() error {
	terminal, err := ui.NewTerminal()
	if err != nil {
		return err
	}
	defer terminal.Close()

	app.status = "已加载缓存信息"
	app.beginRefreshAll(true)

	for {
		app.pumpRefreshEvents()
		if app.needsRedraw {
			app.draw(terminal)
			app.needsRedraw = false
		}

		key, err := terminal.ReadKey()
		if err != nil {
			return err
		}
		if key == "" {
			continue
		}
		if key == "q" {
			return nil
		}

		switch app.mode {
		case viewHelp:
			app.mode = viewNormal
			app.status = "已关闭帮助"
			app.needsRedraw = true
		case viewDetail:
			app.handleDetailKey(key)
		case viewDeleteConfirm:
			app.handleDeleteConfirmKey(key)
		case viewSwitch:
			app.handleSwitchKey(key)
		default:
			app.handleNormalKey(terminal, key)
		}
	}
}

func (app *App) handleNormalKey(terminal *ui.Terminal, key string) {
	switch key {
	case "?":
		app.mode = viewHelp
	case "up", "k":
		app.moveSelection(-1)
	case "down", "j":
		app.moveSelection(1)
	case "enter":
		if app.selected() == nil {
			app.status = "没有可查看详情的账号"
		} else {
			app.mode = viewDetail
			app.status = "已打开账号详情"
		}
	case "s":
		if len(app.profiles) == 0 {
			app.status = "没有可切换的已保存账号"
			break
		}
		if app.currentProfileID != "" {
			for idx, profile := range app.profiles {
				if profile.Meta.ProfileID == app.currentProfileID {
					app.selection = idx
					break
				}
			}
		}
		app.mode = viewSwitch
		app.status = "已进入切换模式，用 ↑/↓ 选择账号，回车切换，Esc 取消"
	case "r":
		app.beginRefreshSelected()
	case "R":
		app.beginRefreshAll(false)
	case "n":
		if app.refreshing {
			app.status = "后台刷新中，请稍后再登录新账号"
			break
		}
		app.loginNewAccount(terminal)
	case "d":
		if app.refreshing {
			app.status = "后台刷新中，请稍后再删除账号"
			break
		}
		if app.selected() == nil {
			app.status = "没有可删除的账号"
			break
		}
		app.mode = viewDeleteConfirm
		app.status = "请再次确认是否删除当前选中账号"
	case "h":
		if app.refreshing {
			app.status = "后台刷新中，请稍后再修改生效目录"
			break
		}
		app.configureTargetHome(terminal)
	}
	app.needsRedraw = true
}

func (app *App) handleSwitchKey(key string) {
	switch key {
	case "esc":
		app.mode = viewNormal
		app.status = "已取消切换"
	case "up", "k":
		app.moveSelection(-1)
	case "down", "j":
		app.moveSelection(1)
	case "enter":
		app.switchSelected()
		app.mode = viewNormal
	}
	app.needsRedraw = true
}

func (app *App) handleDetailKey(key string) {
	switch key {
	case "up", "k":
		app.moveSelection(-1)
	case "down", "j":
		app.moveSelection(1)
	case "esc", "enter":
		app.mode = viewNormal
		app.status = "已关闭账号详情"
	}
	app.needsRedraw = true
}

func (app *App) handleDeleteConfirmKey(key string) {
	switch key {
	case "esc":
		app.mode = viewNormal
		app.status = "已取消删除"
	case "enter":
		app.performDeleteSelected()
		app.mode = viewNormal
	}
	app.needsRedraw = true
}

func (app *App) moveSelection(delta int) {
	if len(app.profiles) == 0 {
		return
	}
	app.selection += delta
	if app.selection < 0 {
		app.selection = 0
	}
	if app.selection >= len(app.profiles) {
		app.selection = len(app.profiles) - 1
	}
}

func (app *App) draw(terminal *ui.Terminal) {
	rows, cols := terminal.Size()
	width := min(max(cols-6, 76), 118)
	indent := max(0, (cols-width)/2)

	screen := []string{"\x1b[2J\x1b[H"}
	screen = append(screen, renderStyledBox(app.accountLines(width-4, max(6, rows-18)), width, indent)...)

	switch app.mode {
	case viewHelp:
		screen = append(screen, "")
		screen = append(screen, renderStyledBox(app.helpLines(), min(width, 70), indent+max(0, (width-min(width, 70))/2))...)
	case viewDetail:
		screen = append(screen, "")
		screen = append(screen, renderStyledBox(app.detailLines(), min(width, 78), indent+max(0, (width-min(width, 78))/2))...)
	case viewDeleteConfirm:
		screen = append(screen, "")
		screen = append(screen, renderStyledBox(app.deleteConfirmLines(), min(width, 74), indent+max(0, (width-min(width, 74))/2))...)
	}

	screen = append(screen, "")
	screen = append(screen, strings.Repeat(" ", indent)+app.footerText())
	_ = terminal.Write(strings.Join(screen, "\n"))
}

func (app *App) accountLines(width int, maxRows int) []styledLine {
	lines := []styledLine{
		{text: ">_ CodexSwitch (" + buildinfo.Version + ")", style: "1", preserve: true},
		{text: ""},
	}
	lines = append(lines, app.currentSummaryLines(width)...)
	lines = append(lines, app.profileTableLines(width, maxRows)...)
	return lines
}

func (app *App) currentSummaryLines(width int) []styledLine {
	if app.currentSnapshot == nil {
		return []styledLine{
			{text: "当前：无", preserve: true},
			{text: "状态：" + shorten(app.status, width-6), style: statusStyle(app.status), preserve: true},
			{text: ""},
			{text: accountBoxTitle(len(app.profiles), app.mode), style: "1", preserve: true},
			{text: ""},
		}
	}

	lines := []styledLine{
		{text: "当前：" + currentAccountLabel(app.currentSnapshot) + "   5h：" + quotaCompact(app.currentQuota, "primary") + "   周：" + quotaCompact(app.currentQuota, "secondary"), style: "32;1", preserve: true},
		{text: "状态：" + shorten(app.status, width-6), style: statusStyle(app.status), preserve: true},
	}
	if app.currentSnapshot.SubscriptionActiveUntil != "" {
		lines = append(lines, styledLine{text: "订阅到期：" + formatISO(app.currentSnapshot.SubscriptionActiveUntil), style: "36", preserve: true})
	}
	if app.currentProfileID == "" {
		lines = append(lines, styledLine{text: "提示：当前生效账号还没有保存到 CodexSwitch。", style: "33"})
	}
	lines = append(lines,
		styledLine{text: ""},
		styledLine{text: accountBoxTitle(len(app.profiles), app.mode), style: "1", preserve: true},
		styledLine{text: ""},
	)
	return lines
}

func (app *App) profileTableLines(width int, maxRows int) []styledLine {
	lines := []styledLine{}
	if len(app.profiles) == 0 {
		return []styledLine{
			styledLine{text: "还没有已保存账号。"},
			styledLine{text: ""},
			styledLine{text: "按 n 登录一个新账号。"},
		}
	}

	labelWidth := max(18, width-58)
	header := "  " + tableCell("账号", labelWidth) + " " +
		tableCell("套餐", 6) + " " +
		tableCell("5h", 8) + " " +
		tableCell("7d", 8) + " " +
		tableCell("状态", 6) + " " +
		tableCell("到期", 11)
	lines = append(lines, styledLine{text: header, style: "2", preserve: true})

	start := 0
	if app.selection >= maxRows {
		start = app.selection - maxRows + 1
	}
	end := min(len(app.profiles), start+maxRows)
	for idx := start; idx < end; idx++ {
		profile := app.profiles[idx]
		active := profile.Meta.ProfileID == app.currentProfileID
		accountLabel := shorten(savedProfileLabel(profile), labelWidth)
		row := rowMarker(app.mode, idx == app.selection, active) + " " +
			tableCell(accountLabel, labelWidth) + " " +
			tableCell(shorten(planLabel(profile.Meta.PlanType), 6), 6) + " " +
			tableCell(quotaCompact(profile.Meta.Quota, "primary"), 8) + " " +
			tableCell(quotaCompact(profile.Meta.Quota, "secondary"), 8) + " " +
			tableCell(shorten(displayStatus(profile.Meta.Status), 6), 6) + " " +
			tableCell(formatCheckedAt(app.subscriptionUntilForProfile(profile)), 11)
		lines = append(lines, styledLine{text: row, style: app.rowStyle(idx, active), preserve: true})
	}

	lines = append(lines, styledLine{text: ""})
	if app.mode == viewSwitch {
		lines = append(lines, styledLine{text: "切换模式：用 ↑/↓ 选择账号，回车确认切换，Esc 取消。", style: "36;1"})
	} else if app.mode == viewDetail {
		lines = append(lines, styledLine{text: "详情模式：可直接用 ↑/↓ 切换账号，详情会跟着更新。", style: "2"})
	} else {
		lines = append(lines, styledLine{text: "绿色行表示当前生效账号，按 Enter 查看详情，按 s 进入切换模式。", style: "2"})
	}
	return lines
}

func (app *App) detailLines() []styledLine {
	profile := app.selected()
	if profile == nil {
		return []styledLine{
			{text: "账号详情", style: "1"},
			{text: ""},
			{text: "当前没有选中账号。"},
			{text: ""},
			{text: "按 Esc 返回。", style: "2"},
		}
	}

	lines := []styledLine{
		{text: "账号详情", style: "1"},
		{text: ""},
		{text: summaryLine("账号", savedProfileLabel(*profile)), preserve: true},
		{text: summaryLine("当前生效", yesNo(profile.Meta.ProfileID == app.currentProfileID)), preserve: true},
		{text: summaryLine("套餐", planLabel(profile.Meta.PlanType)), preserve: true},
		{text: summaryLine("状态", displayStatus(profile.Meta.Status)), preserve: true},
		{text: summaryLine("账号 ID", emptyFallback(profile.Meta.AccountID)), preserve: true},
		{text: summaryLine("RefreshToken", yesNo(profile.Meta.HasRefreshToken)), preserve: true},
		{text: summaryLine("上次刷新", formatISO(profile.Meta.LastRefresh)), preserve: true},
		{text: summaryLine("上次检查", formatISO(profile.Meta.LastChecked)), preserve: true},
		{text: summaryLine("订阅到期", formatISO(app.subscriptionUntilForProfile(*profile))), preserve: true},
		{text: summaryLine("5 小时额度", quotaStatusText(profile.Meta.Quota, "primary")), style: quotaStyle(profile.Meta.Quota, "primary"), preserve: true},
		{text: summaryLine("周额度", quotaStatusText(profile.Meta.Quota, "secondary")), style: quotaStyle(profile.Meta.Quota, "secondary"), preserve: true},
		{text: summaryLine("来源", detailSourceText(profile.Meta.Source)), preserve: true},
	}
	if profile.Meta.LastError != "" {
		lines = append(lines, styledLine{text: summaryLine("错误信息", shorten(profile.Meta.LastError, 48)), style: "33", preserve: true})
	}
	lines = append(lines,
		styledLine{text: ""},
		styledLine{text: "按 ↑/↓ 切换查看其他账号，按 Enter 或 Esc 返回。", style: "2"},
	)
	return lines
}

func (app *App) deleteConfirmLines() []styledLine {
	profile := app.selected()
	if profile == nil {
		return []styledLine{
			{text: "删除确认", style: "1"},
			{text: ""},
			{text: "当前没有选中账号。"},
			{text: ""},
			{text: "按 Esc 返回。", style: "2"},
		}
	}

	lines := []styledLine{
		{text: "删除确认", style: "31;1"},
		{text: ""},
		{text: "将要删除：" + savedProfileLabel(*profile)},
	}
	if profile.Meta.ProfileID == app.currentProfileID {
		lines = append(lines, styledLine{text: "这是当前生效账号。删除保存记录后，当前 auth.json 不会立刻变化，但你之后不能再一键切回。", style: "33"})
	} else {
		lines = append(lines, styledLine{text: "只会删除保存记录，不会修改当前正在生效的 auth.json。"})
	}
	lines = append(lines,
		styledLine{text: ""},
		styledLine{text: "按 Enter 确认删除，按 Esc 取消。", style: "2"},
	)
	return lines
}

func (app *App) helpLines() []styledLine {
	return []styledLine{
		{text: "操作帮助", style: "1"},
		{text: ""},
		{text: "↑/↓ 或 j/k   移动选中项"},
		{text: "Enter   查看账号详情"},
		{text: "s   进入切换模式"},
		{text: "r   刷新当前选中账号"},
		{text: "R   刷新全部账号"},
		{text: "n   登录新账号"},
		{text: "d   删除当前选中账号（会再次确认）"},
		{text: "h   设置或清空目标 CODEX_HOME"},
		{text: "q   退出"},
		{text: ""},
		{text: "按任意键返回。", style: "2"},
	}
}

func (app *App) footerText() string {
	if app.mode == viewSwitch {
		return "切换模式 • ↑/↓ 选择 • Enter 切换 • Esc 取消 • q 退出"
	}
	if app.mode == viewDetail {
		return "详情模式 • ↑/↓ 切换账号 • Enter/Esc 返回 • q 退出"
	}
	if app.mode == viewDeleteConfirm {
		return "删除确认 • Enter 确认 • Esc 取消 • q 退出"
	}
	return "↑/↓ 浏览 • Enter 详情 • s 切换 • r 刷新当前 • R 刷新全部 • n 登录 • d 删除 • h 生效目录 • ? 帮助 • q 退出"
}

func (app *App) rowStyle(idx int, active bool) string {
	if app.mode == viewSwitch && idx == app.selection {
		if active {
			return "7;32;1"
		}
		return "7;36;1"
	}
	if idx == app.selection {
		if active {
			return "32;1"
		}
		return "36"
	}
	if active {
		return "32;1"
	}
	return ""
}

func renderStyledBox(lines []styledLine, width int, indent int) []string {
	if width < 8 {
		width = 8
	}
	innerWidth := width - 4
	prefix := strings.Repeat(" ", indent)
	output := []string{prefix + "╭" + strings.Repeat("─", width-2) + "╮"}
	for _, line := range lines {
		wrapped := wrapLine(line.text, innerWidth)
		if line.preserve {
			wrapped = []string{clipRunes(line.text, innerWidth)}
		}
		if len(wrapped) == 0 {
			wrapped = []string{""}
		}
		for _, segment := range wrapped {
			content := padRight(segment, innerWidth)
			if line.style != "" {
				content = ansi(line.style) + content + ansi("0")
			}
			output = append(output, prefix+"│ "+content+" │")
		}
	}
	output = append(output, prefix+"╰"+strings.Repeat("─", width-2)+"╯")
	return output
}

func ansi(code string) string {
	return "\x1b[" + code + "m"
}

func rowMarker(mode viewMode, selected bool, active bool) string {
	if mode == viewSwitch && selected {
		return "›"
	}
	if active {
		return "●"
	}
	if selected {
		return "›"
	}
	return " "
}

func planLabel(plan string) string {
	if plan == "" {
		return "-"
	}
	return strings.Title(plan)
}

func quotaStatusText(quota *model.RateLimitSnapshot, which string) string {
	if quota == nil {
		return "暂无"
	}
	window := quota.Primary
	if which == "secondary" {
		window = quota.Secondary
	}
	if window == nil {
		return "暂无"
	}
	left := remainingPercent(window)
	return fmt.Sprintf("%s %d%% 剩余（%s重置）", renderRemainingBar(left, 18), left, friendlyReset(window.ResetsAt))
}

func quotaCompact(quota *model.RateLimitSnapshot, which string) string {
	if quota == nil {
		return "--"
	}
	window := quota.Primary
	if which == "secondary" {
		window = quota.Secondary
	}
	if window == nil {
		return "--"
	}
	return fmt.Sprintf("%d%%", remainingPercent(window))
}

func quotaStyle(quota *model.RateLimitSnapshot, which string) string {
	if quota == nil {
		return ""
	}
	window := quota.Primary
	if which == "secondary" {
		window = quota.Secondary
	}
	if window == nil {
		return ""
	}
	left := remainingPercent(window)
	switch {
	case left >= 60:
		return "32"
	case left >= 30:
		return "33"
	default:
		return "31"
	}
}

func remainingPercent(window *model.RateLimitWindow) int {
	if window == nil {
		return 0
	}
	value := 100 - window.UsedPercent
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}

func renderRemainingBar(leftPercent int, slots int) string {
	if slots <= 0 {
		return "[]"
	}
	filled := (leftPercent*slots + 50) / 100
	if filled < 0 {
		filled = 0
	}
	if filled > slots {
		filled = slots
	}
	return "[" + strings.Repeat("█", filled) + strings.Repeat("░", slots-filled) + "]"
}

func statusStyle(message string) string {
	switch {
	case strings.Contains(message, "切换模式"), strings.Contains(message, "刷新"):
		return "36;1"
	case strings.Contains(message, "失败"), strings.Contains(message, "异常"), strings.Contains(message, "重登"), strings.Contains(message, "缺失"):
		return "31"
	case strings.Contains(message, "取消"), strings.Contains(message, "提示"):
		return "33"
	default:
		return "32"
	}
}

func currentAccountLabel(snapshot *model.AuthSnapshot) string {
	if snapshot == nil {
		return "-"
	}
	label := snapshot.DisplayLabel()
	if snapshot.PlanType != "" {
		label += " (" + strings.Title(snapshot.PlanType) + ")"
	}
	return label
}

func (app *App) subscriptionUntilForProfile(profile model.StoredProfile) string {
	if profile.Meta.SubscriptionActiveUntil != "" {
		return profile.Meta.SubscriptionActiveUntil
	}
	if app.currentSnapshot != nil && profile.Meta.ProfileID == app.currentProfileID {
		return app.currentSnapshot.SubscriptionActiveUntil
	}
	return ""
}

func accountBoxTitle(total int, mode viewMode) string {
	title := fmt.Sprintf("账号列表（%d）", total)
	if mode == viewSwitch {
		return title + "  [切换模式]"
	}
	return title
}

func savedProfileLabel(profile model.StoredProfile) string {
	if profile.Meta.Email != "" {
		return profile.Meta.Email
	}
	return profile.Meta.Label
}

func displayPath(path string) string {
	if path == "" {
		return "-"
	}
	home, err := os.UserHomeDir()
	if err == nil && strings.HasPrefix(path, home) {
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
}

func summaryLine(label string, value string) string {
	return tableCell(label+"：", 12) + " " + value
}

func friendlyReset(epoch *int64) string {
	if epoch == nil || *epoch == 0 {
		return "-"
	}
	resetTime := time.Unix(*epoch, 0).Local()
	now := time.Now().Local()
	if resetTime.Year() == now.Year() && resetTime.YearDay() == now.YearDay() {
		return "今天 " + resetTime.Format("15:04")
	}
	return resetTime.Format("1月2日 15:04")
}

func formatISO(value string) string {
	if value == "" {
		return "-"
	}
	value = strings.ReplaceAll(value, "T", " ")
	value = strings.TrimSuffix(value, "Z")
	if len(value) >= 16 {
		return value[:16]
	}
	return value
}

func wrapLine(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	if text == "" {
		return []string{""}
	}
	lines := []string{}
	current := ""
	currentWidth := 0
	for _, r := range text {
		rw := runeDisplayWidth(r)
		if currentWidth+rw > width && current != "" {
			lines = append(lines, current)
			current = ""
			currentWidth = 0
		}
		current += string(r)
		currentWidth += rw
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

func clipRunes(value string, width int) string {
	if width <= 0 {
		return ""
	}
	current := ""
	currentWidth := 0
	for _, r := range value {
		rw := runeDisplayWidth(r)
		if currentWidth+rw > width {
			break
		}
		current += string(r)
		currentWidth += rw
	}
	return current
}

func padRight(value string, width int) string {
	currentWidth := displayWidth(value)
	if currentWidth >= width {
		return value
	}
	return value + strings.Repeat(" ", width-currentWidth)
}

func shorten(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if displayWidth(value) <= width {
		return value
	}
	if width <= 3 {
		return clipRunes(value, width)
	}
	return clipRunes(value, width-3) + "..."
}

func itoa(value int) string {
	return fmt.Sprintf("%d", value)
}

func emptyFallback(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func min(left int, right int) int {
	if left < right {
		return left
	}
	return right
}

func max(left int, right int) int {
	if left > right {
		return left
	}
	return right
}

func (app *App) selected() *model.StoredProfile {
	if len(app.profiles) == 0 || app.selection < 0 || app.selection >= len(app.profiles) {
		return nil
	}
	return &app.profiles[app.selection]
}

func (app *App) beginRefreshSelected() {
	profile := app.selected()
	if profile == nil {
		app.status = "没有可刷新的账号"
		return
	}
	app.startRefresh([]model.StoredProfile{*profile}, "刷新当前账号")
}

func (app *App) beginRefreshAll(initial bool) {
	if len(app.profiles) == 0 {
		if initial {
			app.status = "当前没有已保存账号"
		} else {
			app.status = "没有可刷新的已保存账号"
		}
		app.needsRedraw = true
		return
	}
	reason := "刷新全部账号"
	if initial {
		reason = "启动后自动刷新全部账号"
	}
	app.startRefresh(append([]model.StoredProfile(nil), app.profiles...), reason)
}

func (app *App) startRefresh(profiles []model.StoredProfile, reason string) {
	if app.refreshing {
		app.status = "已有刷新任务进行中"
		app.needsRedraw = true
		return
	}
	if len(profiles) == 0 {
		app.status = "没有可刷新的账号"
		app.needsRedraw = true
		return
	}

	app.refreshing = true
	app.status = fmt.Sprintf("%s：0/%d", reason, len(profiles))
	app.needsRedraw = true

	go func(list []model.StoredProfile, total int, label string) {
		workerCount := min(refreshWorkerLimit, total)
		jobs := make(chan model.StoredProfile)
		results := make(chan string, total)
		var workers sync.WaitGroup

		for idx := 0; idx < workerCount; idx++ {
			workers.Add(1)
			go func() {
				defer workers.Done()
				for profile := range jobs {
					results <- app.refreshProfile(profile)
				}
			}()
		}

		go func() {
			for _, profile := range list {
				jobs <- profile
			}
			close(jobs)
			workers.Wait()
			close(results)
		}()

		completed := 0
		for result := range results {
			completed++
			app.refreshEvents <- refreshEvent{
				message: fmt.Sprintf("%s：%d/%d %s", label, completed, total, result),
			}
		}
		app.refreshEvents <- refreshEvent{
			message: label + "完成",
			final:   true,
		}
	}(profiles, len(profiles), reason)
}

func (app *App) refreshProfile(profile model.StoredProfile) string {
	accountName := savedProfileLabel(profile)
	runtimeHome, runtimeErr := app.store.CreateRuntimeHome("probe", profile.AuthPath())
	if runtimeErr != nil {
		_, _ = app.store.UpsertProfileFromHome(profile.Home, profile.Meta.Source, nil, profile.Meta.Quota, statusFromError(runtimeErr.Error()), runtimeErr.Error())
		return accountName + "（有警告）"
	}
	_, account, quota, err := codex.ProbeCodexHomeWithTimeout(runtimeHome, true, 12*time.Second)
	app.store.CleanupRuntimeHome(runtimeHome)
	if err == nil {
		_, _ = app.store.UpsertProfileFromHome(profile.Home, profile.Meta.Source, account, quota, "ok", "")
		return accountName
	}
	_, _ = app.store.UpsertProfileFromHome(profile.Home, profile.Meta.Source, nil, profile.Meta.Quota, statusFromError(err.Error()), err.Error())
	return accountName + "（有警告）"
}

func (app *App) pumpRefreshEvents() {
	for {
		select {
		case event := <-app.refreshEvents:
			_ = app.syncState()
			app.status = event.message
			if event.final {
				app.refreshing = false
			}
			app.needsRedraw = true
		default:
			return
		}
	}
}

func (app *App) switchSelected() {
	profile := app.selected()
	if profile == nil {
		app.status = "没有选中的账号"
		return
	}
	if profile.Meta.ProfileID == app.currentProfileID {
		app.status = "选中的账号已经是当前生效账号"
		return
	}
	target, err := app.store.SwitchProfile(profile.Meta.ProfileID, app.targetCodexHome)
	if err != nil {
		app.status = "切换失败：" + err.Error()
		return
	}
	_ = app.syncState()
	app.status = "已切换生效账号：" + displayPath(target)
}

func (app *App) performDeleteSelected() {
	profile := app.selected()
	if profile == nil {
		app.status = "没有可删除的账号"
		return
	}
	if err := app.store.DeleteProfile(profile.Meta.ProfileID); err != nil {
		app.status = "删除失败：" + err.Error()
		return
	}
	_ = app.syncState()
	app.status = "已删除账号：" + savedProfileLabel(*profile)
}

func (app *App) loginNewAccount(terminal *ui.Terminal) {
	tempHome, err := app.store.CreateRuntimeHome("login", "")
	if err != nil {
		app.status = "登录失败：" + err.Error()
		app.needsRedraw = true
		return
	}

	result := ""
	err = terminal.Suspend(func(tty *os.File) error {
		defer app.store.CleanupRuntimeHome(tempHome)
		reader := bufio.NewReader(tty)
		fmt.Fprintln(tty)
		fmt.Fprintf(tty, "[%s] 正在隔离目录中启动 ChatGPT 登录：%s\n", auth.UTCNowISO(), tempHome)
		fmt.Fprintln(tty, "浏览器会自动打开；如果没有打开，请手动访问下面展示的 URL。")
		fmt.Fprintln(tty)

		_, loginErr := codex.LoginChatGPT(tempHome, 15*time.Minute, true, func(message string) {
			fmt.Fprintln(tty, message)
			fmt.Fprintln(tty)
		})
		if loginErr != nil {
			return loginErr
		}

		fmt.Fprintln(tty, "登录完成，正在读取账号与额度信息...")
		_, account, quota, probeErr := codex.ProbeCodexHomeWithTimeout(tempHome, false, 20*time.Second)
		if probeErr == nil {
			profile, saveErr := app.store.UpsertProfileFromHome(tempHome, "login", account, quota, "ok", "")
			if saveErr != nil {
				return saveErr
			}
			result = "已保存账号：" + savedProfileLabel(profile)
		} else {
			profile, saveErr := app.store.UpsertProfileFromHome(tempHome, "login", nil, nil, statusFromError(probeErr.Error()), probeErr.Error())
			if saveErr != nil {
				return saveErr
			}
			fmt.Fprintf(tty, "读取额度时出现警告：%s\n", probeErr.Error())
			result = "已保存账号：" + savedProfileLabel(profile)
		}

		fmt.Fprintln(tty, result)
		fmt.Fprintln(tty, "按回车返回 CodexSwitch。")
		_, _ = reader.ReadString('\n')
		return nil
	})
	if err != nil {
		app.status = "登录失败：" + err.Error()
		app.needsRedraw = true
		return
	}

	_ = app.syncState()
	app.status = result
	app.needsRedraw = true
}

func (app *App) configureTargetHome(terminal *ui.Terminal) {
	current := app.settings.TargetCodexHomeOverride
	if app.runtimeOverride != "" {
		current = app.runtimeOverride
	}
	entered, err := terminal.Prompt("目标 CODEX_HOME（留空=自动探测）", current, false)
	if err != nil {
		app.status = "更新生效目录失败：" + err.Error()
		app.needsRedraw = true
		return
	}
	if strings.TrimSpace(entered) != "" {
		app.settings.TargetCodexHomeOverride = auth.ExpandPath(entered)
		app.runtimeOverride = app.settings.TargetCodexHomeOverride
	} else {
		app.settings.TargetCodexHomeOverride = ""
		app.runtimeOverride = ""
	}
	if err := app.store.SaveSettings(app.settings); err != nil {
		app.status = "更新生效目录失败：" + err.Error()
		app.needsRedraw = true
		return
	}
	_ = app.syncState()
	app.status = "生效目录已更新为：" + displayPath(app.targetCodexHome)
	app.needsRedraw = true
}

func statusFromError(message string) string {
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "sign in again"), strings.Contains(lower, "unauthorized"), strings.Contains(lower, "401"):
		return "relogin"
	case strings.Contains(lower, "not found"):
		return "missing"
	default:
		return "error"
	}
}

func displayStatus(status string) string {
	switch status {
	case "", "-", "unknown":
		return "-"
	case "ok":
		return "正常"
	case "relogin":
		return "重登"
	case "missing":
		return "缺失"
	case "error":
		return "异常"
	default:
		return status
	}
}

func formatCheckedAt(value string) string {
	if value == "" {
		return "-"
	}
	value = strings.ReplaceAll(value, "T", " ")
	value = strings.TrimSuffix(value, "Z")
	if len(value) >= 16 {
		parts := strings.Split(value[:16], " ")
		if len(parts) == 2 && len(parts[0]) >= 5 {
			return parts[0][5:] + " " + parts[1]
		}
	}
	return shorten(value, 11)
}

func detailSourceText(source string) string {
	switch source {
	case "login":
		return "软件内登录"
	case "":
		return "-"
	default:
		return source
	}
}

func yesNo(value bool) string {
	if value {
		return "是"
	}
	return "否"
}

func tableCell(value string, width int) string {
	return padRight(shorten(value, width), width)
}

func displayWidth(value string) int {
	total := 0
	for _, r := range value {
		total += runeDisplayWidth(r)
	}
	return total
}

func runeDisplayWidth(r rune) int {
	switch {
	case r == 0:
		return 0
	case r < 32 || (r >= 0x7f && r < 0xa0):
		return 0
	case unicode.Is(unicode.Mn, r):
		return 0
	case r >= 0x1100 && (r <= 0x115f ||
		r == 0x2329 || r == 0x232a ||
		(r >= 0x2e80 && r <= 0xa4cf && r != 0x303f) ||
		(r >= 0xac00 && r <= 0xd7a3) ||
		(r >= 0xf900 && r <= 0xfaff) ||
		(r >= 0xfe10 && r <= 0xfe19) ||
		(r >= 0xfe30 && r <= 0xfe6f) ||
		(r >= 0xff00 && r <= 0xff60) ||
		(r >= 0xffe0 && r <= 0xffe6) ||
		(r >= 0x1f300 && r <= 0x1faff)):
		return 2
	default:
		return 1
	}
}
