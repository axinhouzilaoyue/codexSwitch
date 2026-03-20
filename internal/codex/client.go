package codex

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"codexswitch/internal/auth"
	"codexswitch/internal/buildinfo"
	"codexswitch/internal/conv"
	"codexswitch/internal/model"
)

type AppServerError struct {
	Message string
}

func (err *AppServerError) Error() string {
	return err.Message
}

type AppServerClient struct {
	CodexHome   string
	Cwd         string
	ExtraConfig []string

	cmd           *exec.Cmd
	stdin         io.WriteCloser
	pendingMu     sync.Mutex
	pending       map[int64]chan map[string]any
	notifications chan map[string]any
	nextID        atomic.Int64
}

func NewClient(codexHome string, cwd string, extraConfig []string) *AppServerClient {
	return &AppServerClient{
		CodexHome:     codexHome,
		Cwd:           cwd,
		ExtraConfig:   extraConfig,
		pending:       map[int64]chan map[string]any{},
		notifications: make(chan map[string]any, 32),
	}
}

func (client *AppServerClient) Start() error {
	if client.cmd != nil {
		return nil
	}
	args := []string{"app-server", "-c", `cli_auth_credentials_store="file"`}
	for _, item := range client.ExtraConfig {
		args = append(args, "-c", item)
	}
	cmd := exec.Command("codex", args...)
	env := os.Environ()
	if client.CodexHome != "" {
		env = append(env, "CODEX_HOME="+client.CodexHome)
	}
	cmd.Env = env
	if client.Cwd != "" {
		cmd.Dir = client.Cwd
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	client.cmd = cmd
	client.stdin = stdin
	go client.readStdout(stdout)
	go drain(stderr)
	_, err = client.Request("initialize", map[string]any{
		"clientInfo": map[string]any{
			"name":    "codexswitch",
			"version": buildinfo.Version,
		},
	}, 10*time.Second)
	return err
}

func drain(reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	buffer := make([]byte, 0, 64*1024)
	scanner.Buffer(buffer, 8*1024*1024)
	for scanner.Scan() {
	}
}

func (client *AppServerClient) readStdout(reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	buffer := make([]byte, 0, 64*1024)
	scanner.Buffer(buffer, 8*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		message := map[string]any{}
		if err := json.Unmarshal([]byte(line), &message); err != nil {
			continue
		}
		if rawID, ok := message["id"]; ok && (message["result"] != nil || message["error"] != nil) {
			if id := conv.Int64(rawID); id != nil {
				client.pendingMu.Lock()
				ch := client.pending[*id]
				client.pendingMu.Unlock()
				if ch != nil {
					ch <- message
				}
			}
			continue
		}
		if conv.String(message["method"]) != "" {
			select {
			case client.notifications <- message:
			default:
			}
		}
	}
}

func (client *AppServerClient) Close() {
	if client.cmd == nil {
		return
	}
	if client.stdin != nil {
		_ = client.stdin.Close()
	}
	if client.cmd.Process != nil {
		_ = client.cmd.Process.Signal(os.Interrupt)
		done := make(chan struct{})
		go func() {
			_, _ = client.cmd.Process.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			_ = client.cmd.Process.Kill()
		}
	}
	client.cmd = nil
}

func (client *AppServerClient) Request(method string, params map[string]any, timeout time.Duration) (map[string]any, error) {
	if err := client.Start(); err != nil {
		return nil, err
	}
	requestID := client.nextID.Add(1)
	responseCh := make(chan map[string]any, 1)
	client.pendingMu.Lock()
	client.pending[requestID] = responseCh
	client.pendingMu.Unlock()
	defer func() {
		client.pendingMu.Lock()
		delete(client.pending, requestID)
		client.pendingMu.Unlock()
	}()

	payload := map[string]any{
		"id":     requestID,
		"method": method,
	}
	if params != nil {
		payload["params"] = params
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	if _, err := client.stdin.Write(append(encoded, '\n')); err != nil {
		return nil, err
	}

	select {
	case message := <-responseCh:
		if message["error"] != nil {
			return nil, &AppServerError{Message: fmt.Sprintf("%s failed: %v", method, message["error"])}
		}
		if result := conv.Map(message["result"]); result != nil {
			return result, nil
		}
		return map[string]any{}, nil
	case <-time.After(timeout):
		return nil, &AppServerError{Message: fmt.Sprintf("timed out waiting for %s", method)}
	}
}

func (client *AppServerClient) NextNotification(timeout time.Duration) (map[string]any, bool) {
	select {
	case message := <-client.notifications:
		return message, true
	case <-time.After(timeout):
		return nil, false
	}
}

func extractUserConfigPath(configResponse map[string]any) string {
	for _, layerRaw := range conv.Slice(configResponse["layers"]) {
		layer := conv.Map(layerRaw)
		name := conv.Map(layer["name"])
		if conv.String(name["type"]) == "user" && conv.String(name["file"]) != "" {
			return conv.String(name["file"])
		}
	}
	for _, originRaw := range conv.Map(configResponse["origins"]) {
		origin := conv.Map(originRaw)
		name := conv.Map(origin["name"])
		if conv.String(name["type"]) == "user" && conv.String(name["file"]) != "" {
			return conv.String(name["file"])
		}
	}
	return ""
}

func DetectEffectiveCodexHome(cwd string) (string, error) {
	client := NewClient("", cwd, nil)
	defer client.Close()
	response, err := client.Request("config/read", map[string]any{
		"includeLayers": true,
		"cwd":           cwd,
	}, 20*time.Second)
	if err == nil {
		if configPath := extractUserConfigPath(response); configPath != "" {
			return filepath.Dir(configPath), nil
		}
	}
	if envOverride := os.Getenv("CODEX_HOME"); envOverride != "" {
		return auth.ExpandPath(envOverride), nil
	}
	home, homeErr := os.UserHomeDir()
	if homeErr != nil {
		if err != nil {
			return "", err
		}
		return "", homeErr
	}
	return filepath.Join(home, ".codex"), nil
}

func ReadAccount(codexHome string, refreshToken bool) (*model.AccountInfo, error) {
	return ReadAccountWithTimeout(codexHome, refreshToken, 60*time.Second)
}

func ReadAccountWithTimeout(codexHome string, refreshToken bool, timeout time.Duration) (*model.AccountInfo, error) {
	client := NewClient(codexHome, "", nil)
	defer client.Close()
	response, err := client.Request("account/read", map[string]any{
		"refreshToken": refreshToken,
	}, timeout)
	if err != nil {
		return nil, err
	}
	return model.AccountInfoFromResponse(response), nil
}

func ReadRateLimits(codexHome string) (*model.RateLimitSnapshot, error) {
	return ReadRateLimitsWithTimeout(codexHome, 60*time.Second)
}

func ReadRateLimitsWithTimeout(codexHome string, timeout time.Duration) (*model.RateLimitSnapshot, error) {
	client := NewClient(codexHome, "", nil)
	defer client.Close()
	response, err := client.Request("account/rateLimits/read", nil, timeout)
	if err != nil {
		return nil, err
	}
	byID := conv.Map(response["rateLimitsByLimitId"])
	if codexLimits := conv.Map(byID["codex"]); codexLimits != nil {
		return model.RateLimitSnapshotFromMap(codexLimits), nil
	}
	return model.RateLimitSnapshotFromMap(conv.Map(response["rateLimits"])), nil
}

func ProbeCodexHome(codexHome string, refreshToken bool) (model.AuthSnapshot, *model.AccountInfo, *model.RateLimitSnapshot, error) {
	return ProbeCodexHomeWithTimeout(codexHome, refreshToken, 60*time.Second)
}

func ProbeCodexHomeWithTimeout(codexHome string, refreshToken bool, timeout time.Duration) (model.AuthSnapshot, *model.AccountInfo, *model.RateLimitSnapshot, error) {
	authPath := filepath.Join(codexHome, "auth.json")
	if _, err := os.Stat(authPath); err != nil {
		return model.AuthSnapshot{}, nil, nil, &AppServerError{Message: fmt.Sprintf("auth.json not found in %s", codexHome)}
	}
	snapshot, err := auth.LoadAuthSnapshot(authPath)
	if err != nil {
		return model.AuthSnapshot{}, nil, nil, err
	}
	var account *model.AccountInfo
	var quota *model.RateLimitSnapshot
	account, accountErr := ReadAccountWithTimeout(codexHome, refreshToken, timeout)
	quota, quotaErr := ReadRateLimitsWithTimeout(codexHome, timeout)
	if account != nil {
		snapshot = auth.SnapshotWithAccount(snapshot, account)
	}
	switch {
	case accountErr != nil && quotaErr != nil:
		return snapshot, account, quota, errors.New(accountErr.Error() + "; " + quotaErr.Error())
	case accountErr != nil:
		return snapshot, account, quota, accountErr
	case quotaErr != nil:
		return snapshot, account, quota, quotaErr
	default:
		return snapshot, account, quota, nil
	}
}

func LoginChatGPT(codexHome string, timeout time.Duration, openBrowser bool, status func(string)) (string, error) {
	if err := os.MkdirAll(codexHome, 0o700); err != nil {
		return "", err
	}
	client := NewClient(codexHome, "", nil)
	defer client.Close()
	response, err := client.Request("account/login/start", map[string]any{"type": "chatgpt"}, 20*time.Second)
	if err != nil {
		return "", err
	}
	authURL := conv.String(response["authUrl"])
	loginID := conv.String(response["loginId"])
	if status != nil {
		status("Open this URL to finish login:\n" + authURL)
	}
	if openBrowser && authURL != "" {
		_ = openURL(authURL)
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		notification, ok := client.NextNotification(500 * time.Millisecond)
		if !ok {
			continue
		}
		if conv.String(notification["method"]) != "account/login/completed" {
			continue
		}
		params := conv.Map(notification["params"])
		notificationLoginID := conv.String(params["loginId"])
		if notificationLoginID != "" && notificationLoginID != loginID {
			continue
		}
		if conv.Bool(params["success"]) {
			authPath := filepath.Join(codexHome, "auth.json")
			if _, err := os.Stat(authPath); err != nil {
				return "", &AppServerError{Message: "login succeeded but auth.json was not created"}
			}
			return authPath, nil
		}
		return "", &AppServerError{Message: conv.String(params["error"])}
	}
	_, _ = client.Request("account/login/cancel", map[string]any{"loginId": loginID}, 5*time.Second)
	return "", &AppServerError{Message: "login timed out"}
}

func openURL(url string) error {
	if url == "" {
		return nil
	}
	command := "open"
	args := []string{url}
	if runtime.GOOS == "linux" {
		command = "xdg-open"
	}
	return exec.Command(command, args...).Start()
}
