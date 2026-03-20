package store

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"codexswitch/internal/auth"
	"codexswitch/internal/model"
)

type ProfileStore struct {
	Root         string
	ProfilesDir  string
	BackupsDir   string
	TmpDir       string
	SettingsPath string
}

func New(root string) (*ProfileStore, error) {
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		root = filepath.Join(home, ".codex-switch")
	}
	root = auth.ExpandPath(root)
	profilesDir := filepath.Join(root, "profiles")
	backupsDir := filepath.Join(root, "backups")
	tmpDir := filepath.Join(root, "tmp")
	return &ProfileStore{
		Root:         root,
		ProfilesDir:  profilesDir,
		BackupsDir:   backupsDir,
		TmpDir:       tmpDir,
		SettingsPath: filepath.Join(root, "settings.json"),
	}, nil
}

func ensurePrivateDir(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	_ = os.Chmod(path, 0o700)
	return nil
}

func WriteJSONAtomic(path string, payload any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	handle, err := os.CreateTemp(filepath.Dir(path), ".tmp-*.json")
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(handle)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(payload); err != nil {
		_ = handle.Close()
		_ = os.Remove(handle.Name())
		return err
	}
	if err := handle.Close(); err != nil {
		_ = os.Remove(handle.Name())
		return err
	}
	if err := os.Rename(handle.Name(), path); err != nil {
		_ = os.Remove(handle.Name())
		return err
	}
	_ = os.Chmod(path, 0o600)
	return nil
}

func CopyFileAtomic(source string, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.CreateTemp(filepath.Dir(target), ".tmp-*")
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(out.Name())
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(out.Name())
		return err
	}
	if err := os.Rename(out.Name(), target); err != nil {
		_ = os.Remove(out.Name())
		return err
	}
	_ = os.Chmod(target, 0o600)
	return nil
}

func (store *ProfileStore) profileHome(profileID string) string {
	return filepath.Join(store.ProfilesDir, profileID)
}

func (store *ProfileStore) LoadSettings() (model.AppSettings, error) {
	if _, err := os.Stat(store.SettingsPath); os.IsNotExist(err) {
		return model.AppSettings{}, nil
	}
	content, err := os.ReadFile(store.SettingsPath)
	if err != nil {
		return model.AppSettings{}, err
	}
	var settings model.AppSettings
	if err := json.Unmarshal(content, &settings); err != nil {
		return model.AppSettings{}, err
	}
	return settings, nil
}

func (store *ProfileStore) SaveSettings(settings model.AppSettings) error {
	return WriteJSONAtomic(store.SettingsPath, settings)
}

func (store *ProfileStore) CreateTempHome(prefix string) (string, error) {
	if err := ensurePrivateDir(store.TmpDir); err != nil {
		return "", err
	}
	path, err := os.MkdirTemp(store.TmpDir, prefix+"-")
	if err != nil {
		return "", err
	}
	_ = os.Chmod(path, 0o700)
	return path, nil
}

func (store *ProfileStore) CleanupTempHome(path string) {
	_ = os.RemoveAll(path)
}

func (store *ProfileStore) ListProfiles() ([]model.StoredProfile, error) {
	if _, err := os.Stat(store.ProfilesDir); os.IsNotExist(err) {
		return []model.StoredProfile{}, nil
	}
	entries, err := os.ReadDir(store.ProfilesDir)
	if err != nil {
		return nil, err
	}
	profiles := make([]model.StoredProfile, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") || strings.HasPrefix(entry.Name(), "_") {
			continue
		}
		home := filepath.Join(store.ProfilesDir, entry.Name())
		metaPath := filepath.Join(home, "meta.json")
		authPath := filepath.Join(home, "auth.json")
		if _, err := os.Stat(metaPath); os.IsNotExist(err) {
			if _, err := os.Stat(authPath); err != nil {
				continue
			}
			snapshot, err := auth.LoadAuthSnapshot(authPath)
			if err != nil {
				continue
			}
			now := auth.UTCNowISO()
			meta := model.ProfileMeta{
				ProfileID:               entry.Name(),
				Label:                   auth.DefaultProfileLabel(snapshot, nil),
				AccountID:               snapshot.AccountID,
				Email:                   snapshot.Email,
				PlanType:                snapshot.PlanType,
				AuthMode:                snapshot.AuthMode,
				HasRefreshToken:         snapshot.HasRefreshToken,
				SubscriptionActiveStart: snapshot.SubscriptionActiveStart,
				SubscriptionActiveUntil: snapshot.SubscriptionActiveUntil,
				SubscriptionLastChecked: snapshot.SubscriptionLastChecked,
				AuthSHA256:              snapshot.AuthSHA256,
				LastRefresh:             snapshot.LastRefresh,
				AccessExp:               snapshot.AccessExp,
				IDExp:                   snapshot.IDExp,
				CreatedAt:               now,
				UpdatedAt:               now,
			}
			if err := WriteJSONAtomic(metaPath, meta); err != nil {
				return nil, err
			}
		}
		profile, err := store.GetProfile(entry.Name())
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, profile)
	}
	slices.SortFunc(profiles, func(left model.StoredProfile, right model.StoredProfile) int {
		leftEmail := strings.ToLower(left.Meta.Email)
		rightEmail := strings.ToLower(right.Meta.Email)
		if leftEmail < rightEmail {
			return -1
		}
		if leftEmail > rightEmail {
			return 1
		}
		return strings.Compare(left.Meta.ProfileID, right.Meta.ProfileID)
	})
	return profiles, nil
}

func (store *ProfileStore) GetProfile(profileID string) (model.StoredProfile, error) {
	home := store.profileHome(profileID)
	content, err := os.ReadFile(filepath.Join(home, "meta.json"))
	if err != nil {
		return model.StoredProfile{}, fmt.Errorf("profile %s not found: %w", profileID, err)
	}
	var meta model.ProfileMeta
	if err := json.Unmarshal(content, &meta); err != nil {
		return model.StoredProfile{}, err
	}
	return model.StoredProfile{Home: home, Meta: meta}, nil
}

func (store *ProfileStore) UpsertProfileFromHome(sourceHome string, source string, account *model.AccountInfo, quota *model.RateLimitSnapshot, status string, lastError string) (model.StoredProfile, error) {
	authPath := filepath.Join(sourceHome, "auth.json")
	snapshot, err := auth.LoadAuthSnapshot(authPath)
	if err != nil {
		return model.StoredProfile{}, err
	}
	profileID := auth.CanonicalProfileID(snapshot)
	finalHome := store.profileHome(profileID)
	if err := ensurePrivateDir(finalHome); err != nil {
		return model.StoredProfile{}, err
	}
	finalAuth := filepath.Join(finalHome, "auth.json")
	if filepath.Clean(authPath) != filepath.Clean(finalAuth) {
		if err := CopyFileAtomic(authPath, finalAuth); err != nil {
			return model.StoredProfile{}, err
		}
	}
	var existing *model.ProfileMeta
	metaPath := filepath.Join(finalHome, "meta.json")
	if content, err := os.ReadFile(metaPath); err == nil {
		var meta model.ProfileMeta
		if json.Unmarshal(content, &meta) == nil {
			existing = &meta
		}
	}
	now := auth.UTCNowISO()
	label := auth.DefaultProfileLabel(snapshot, account)
	createdAt := now
	if existing != nil {
		if existing.Label != "" {
			label = existing.Label
		}
		if existing.CreatedAt != "" {
			createdAt = existing.CreatedAt
		}
	}
	email := snapshot.Email
	planType := snapshot.PlanType
	if account != nil {
		if account.Email != "" {
			email = account.Email
		}
		if account.PlanType != "" {
			planType = account.PlanType
		}
	}
	meta := model.ProfileMeta{
		ProfileID:               profileID,
		Label:                   label,
		AccountID:               snapshot.AccountID,
		Email:                   email,
		PlanType:                planType,
		AuthMode:                snapshot.AuthMode,
		HasRefreshToken:         snapshot.HasRefreshToken,
		SubscriptionActiveStart: snapshot.SubscriptionActiveStart,
		SubscriptionActiveUntil: snapshot.SubscriptionActiveUntil,
		SubscriptionLastChecked: snapshot.SubscriptionLastChecked,
		AuthSHA256:              snapshot.AuthSHA256,
		LastRefresh:             snapshot.LastRefresh,
		AccessExp:               snapshot.AccessExp,
		IDExp:                   snapshot.IDExp,
		CreatedAt:               createdAt,
		UpdatedAt:               now,
		LastChecked:             now,
		Source:                  source,
		Status:                  status,
		LastError:               lastError,
		Quota:                   quota,
	}
	if err := WriteJSONAtomic(metaPath, meta); err != nil {
		return model.StoredProfile{}, err
	}
	return model.StoredProfile{Home: finalHome, Meta: meta}, nil
}

func (store *ProfileStore) DeleteProfile(profileID string) error {
	return os.RemoveAll(store.profileHome(profileID))
}

func (store *ProfileStore) SwitchProfile(profileID string, targetCodexHome string) (string, string, error) {
	profile, err := store.GetProfile(profileID)
	if err != nil {
		return "", "", err
	}
	targetCodexHome = auth.ExpandPath(targetCodexHome)
	if err := ensurePrivateDir(targetCodexHome); err != nil {
		return "", "", err
	}
	targetAuth := filepath.Join(targetCodexHome, "auth.json")
	backupPath := ""
	if _, err := os.Stat(targetAuth); err == nil {
		if err := ensurePrivateDir(store.BackupsDir); err != nil {
			return "", "", err
		}
		backupPath = filepath.Join(store.BackupsDir, "auth-backup-"+strings.ReplaceAll(auth.UTCNowISO(), ":", "-")+".json")
		if err := CopyFileAtomic(targetAuth, backupPath); err != nil {
			return "", "", err
		}
	}
	if err := CopyFileAtomic(profile.AuthPath(), targetAuth); err != nil {
		return "", "", err
	}
	return targetAuth, backupPath, nil
}
