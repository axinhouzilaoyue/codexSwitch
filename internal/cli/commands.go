package cli

import (
	"fmt"
	"os/exec"

	"codexswitch/internal/buildinfo"
	"codexswitch/internal/model"
	"codexswitch/internal/state"
	"codexswitch/internal/store"
)

func RunList(profileStore *store.ProfileStore, targetOverride string) error {
	snapshot, err := state.Load(profileStore, targetOverride)
	if err != nil {
		return err
	}
	if len(snapshot.Profiles) == 0 {
		fmt.Println("No saved profiles.")
		return nil
	}
	fmt.Printf("%-2s %-32s %-8s %-10s %-11s %-11s %-10s\n", "*", "LABEL", "PLAN", "STATUS", "PRIMARY", "SECONDARY", "ACCOUNT")
	for _, profile := range snapshot.Profiles {
		marker := " "
		if profile.Meta.ProfileID == snapshot.CurrentProfileID {
			marker = "*"
		}
		fmt.Printf(
			"%-2s %-32s %-8s %-10s %-11s %-11s %-10s\n",
			marker,
			truncate(profile.Meta.Label, 32),
			fallback(profile.Meta.PlanType),
			fallback(profile.Meta.Status),
			quotaCell(profile.Meta.Quota, true),
			quotaCell(profile.Meta.Quota, false),
			truncate(fallback(profile.Meta.AccountID), 10),
		)
	}
	return nil
}

func RunCurrent(profileStore *store.ProfileStore, targetOverride string) error {
	snapshot, err := state.Load(profileStore, targetOverride)
	if err != nil {
		return err
	}
	fmt.Printf("CodexSwitch %s\n", buildinfo.Version)
	fmt.Printf("Target CODEX_HOME: %s\n", snapshot.TargetCodexHome)
	if snapshot.CurrentSnapshot == nil {
		fmt.Println("Current account: none")
		return nil
	}
	fmt.Printf("Current account: %s [%s]\n", snapshot.CurrentSnapshot.DisplayLabel(), fallback(snapshot.CurrentSnapshot.PlanType))
	fmt.Printf("Account ID: %s\n", fallback(snapshot.CurrentSnapshot.AccountID))
	fmt.Printf("Last refresh: %s\n", fallback(snapshot.CurrentSnapshot.LastRefresh))
	fmt.Printf("Primary quota: %s\n", quotaCell(snapshot.CurrentQuota, true))
	fmt.Printf("Secondary quota: %s\n", quotaCell(snapshot.CurrentQuota, false))
	if snapshot.CurrentQuotaNote != "" {
		fmt.Printf("Quota source: %s\n", snapshot.CurrentQuotaNote)
	}
	if snapshot.CurrentError != "" {
		fmt.Printf("Quota/status warning: %s\n", snapshot.CurrentError)
	}
	if snapshot.CurrentProfileID == "" {
		fmt.Println("Managed profile: no")
		return nil
	}
	fmt.Printf("Managed profile: yes (%s)\n", snapshot.CurrentProfileID)
	return nil
}

func RunDoctor(profileStore *store.ProfileStore, targetOverride string) error {
	snapshot, err := state.Load(profileStore, targetOverride)
	if err != nil {
		return err
	}
	codexPath, codexErr := exec.LookPath("codex")
	fmt.Printf("CodexSwitch %s\n", buildinfo.Version)
	fmt.Printf("Store root: %s\n", profileStore.Root)
	fmt.Printf("Profiles: %d\n", len(snapshot.Profiles))
	fmt.Printf("Target CODEX_HOME: %s\n", snapshot.TargetCodexHome)
	switch {
	case targetOverride != "":
		fmt.Println("Target source: runtime override")
	case snapshot.Settings.TargetCodexHomeOverride != "":
		fmt.Println("Target source: saved override")
	default:
		fmt.Println("Target source: codex app-server auto-detect")
	}
	if codexErr != nil {
		fmt.Printf("codex binary: missing (%v)\n", codexErr)
	} else {
		fmt.Printf("codex binary: %s\n", codexPath)
	}
	if snapshot.CurrentSnapshot == nil {
		fmt.Println("Active auth.json: missing or unreadable")
	} else {
		fmt.Printf("Active auth.json: %s\n", snapshot.CurrentSnapshot.SourcePath)
		fmt.Printf("Active account: %s [%s]\n", snapshot.CurrentSnapshot.DisplayLabel(), fallback(snapshot.CurrentSnapshot.PlanType))
		fmt.Printf("Active primary quota: %s\n", quotaCell(snapshot.CurrentQuota, true))
		fmt.Printf("Active secondary quota: %s\n", quotaCell(snapshot.CurrentQuota, false))
		if snapshot.CurrentQuotaNote != "" {
			fmt.Printf("Active quota source: %s\n", snapshot.CurrentQuotaNote)
		}
		if snapshot.CurrentError != "" {
			fmt.Printf("Active quota/status warning: %s\n", snapshot.CurrentError)
		}
		if snapshot.CurrentProfileID == "" {
			fmt.Println("Managed profile: no")
		} else {
			fmt.Printf("Managed profile: yes (%s)\n", snapshot.CurrentProfileID)
		}
	}
	return nil
}

func quotaCell(quota *model.RateLimitSnapshot, primary bool) string {
	if quota == nil {
		return "-"
	}
	window := quota.Primary
	if !primary {
		window = quota.Secondary
	}
	if window == nil {
		return "-"
	}
	duration := "?"
	if window.WindowDurationMins != nil {
		duration = fmt.Sprintf("%d", *window.WindowDurationMins)
	}
	return fmt.Sprintf("%d%%/%sm", window.UsedPercent, duration)
}

func fallback(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func truncate(value string, limit int) string {
	if len([]rune(value)) <= limit {
		return value
	}
	return string([]rune(value)[:limit])
}
