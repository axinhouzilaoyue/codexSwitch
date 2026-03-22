package state

import (
	"os"
	"path/filepath"

	"codexswitch/internal/auth"
	"codexswitch/internal/codex"
	"codexswitch/internal/model"
	"codexswitch/internal/store"
)

type Snapshot struct {
	Settings         model.AppSettings
	TargetCodexHome  string
	Profiles         []model.StoredProfile
	CurrentSnapshot  *model.AuthSnapshot
	CurrentQuota     *model.RateLimitSnapshot
	CurrentError     string
	CurrentQuotaNote string
	CurrentProfileID string
}

func ResolveTargetHome(settings model.AppSettings, runtimeOverride string) string {
	if runtimeOverride != "" {
		return auth.ExpandPath(runtimeOverride)
	}
	if settings.TargetCodexHomeOverride != "" {
		return auth.ExpandPath(settings.TargetCodexHomeOverride)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return fallbackTargetHome()
	}
	path, err := codex.DetectEffectiveCodexHome(cwd)
	if err != nil {
		return fallbackTargetHome()
	}
	return path
}

func fallbackTargetHome() string {
	if envHome := os.Getenv("CODEX_HOME"); envHome != "" {
		return auth.ExpandPath(envHome)
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".codex")
}

func Load(profileStore *store.ProfileStore, runtimeOverride string) (Snapshot, error) {
	settings, err := profileStore.LoadSettings()
	if err != nil {
		return Snapshot{}, err
	}
	target := ResolveTargetHome(settings, runtimeOverride)
	profiles, err := profileStore.ListProfiles()
	if err != nil {
		return Snapshot{}, err
	}
	var currentSnapshot *model.AuthSnapshot
	var currentQuota *model.RateLimitSnapshot
	currentError := ""
	currentQuotaNote := ""
	authPath := filepath.Join(target, "auth.json")
	if _, err := os.Stat(authPath); err == nil {
		if snapshot, err := auth.LoadAuthSnapshot(authPath); err == nil {
			currentSnapshot = &snapshot
		}
	}
	currentProfileID := ""
	if currentSnapshot != nil {
		if profile := matchCurrentProfile(profiles, *currentSnapshot); profile != nil {
			currentProfileID = profile.Meta.ProfileID
			currentQuota = profile.Meta.Quota
			if profile.Meta.LastError != "" {
				currentError = profile.Meta.LastError
			}
			if currentQuota != nil {
				currentQuotaNote = "saved profile cache"
			}
		}
		if currentQuota == nil {
			if fallbackQuota, snapshotTimestamp, sourcePath, fallbackErr := codex.LatestSessionRateLimits(target); fallbackErr == nil && fallbackQuota != nil {
				currentQuota = fallbackQuota
				currentQuotaNote = "recent session snapshot"
				if snapshotTimestamp != "" {
					currentQuotaNote += " " + snapshotTimestamp
				}
				if sourcePath != "" {
					currentQuotaNote += " from " + sourcePath
				}
			}
		}
	}
	return Snapshot{
		Settings:         settings,
		TargetCodexHome:  target,
		Profiles:         profiles,
		CurrentSnapshot:  currentSnapshot,
		CurrentQuota:     currentQuota,
		CurrentError:     currentError,
		CurrentQuotaNote: currentQuotaNote,
		CurrentProfileID: currentProfileID,
	}, nil
}

func matchCurrentProfile(profiles []model.StoredProfile, snapshot model.AuthSnapshot) *model.StoredProfile {
	currentEmail := auth.NormalizeEmail(snapshot.Email)
	if currentEmail != "" {
		for idx := range profiles {
			if auth.NormalizeEmail(profiles[idx].Meta.Email) == currentEmail {
				return &profiles[idx]
			}
		}
	}
	if snapshot.AuthSHA256 != "" {
		for idx := range profiles {
			if profiles[idx].Meta.AuthSHA256 == snapshot.AuthSHA256 {
				return &profiles[idx]
			}
		}
	}
	if snapshot.AccountID != "" {
		for idx := range profiles {
			if profiles[idx].Meta.AccountID == snapshot.AccountID {
				return &profiles[idx]
			}
		}
	}
	return nil
}
