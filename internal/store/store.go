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
	return &ProfileStore{
		Root:         root,
		ProfilesDir:  profilesDir,
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

func (store *ProfileStore) CreateRuntimeHome(prefix string, authSourcePath string) (string, error) {
	path, err := os.MkdirTemp("", "ccodex-"+prefix+"-")
	if err != nil {
		return "", err
	}
	_ = os.Chmod(path, 0o700)
	if authSourcePath != "" {
		targetAuth := filepath.Join(path, "auth.json")
		if err := CopyFileAtomic(authSourcePath, targetAuth); err != nil {
			_ = os.RemoveAll(path)
			return "", err
		}
	}
	return path, nil
}

func (store *ProfileStore) CleanupRuntimeHome(path string) {
	_ = os.RemoveAll(path)
}

func (store *ProfileStore) ListProfiles() ([]model.StoredProfile, error) {
	if err := store.CleanupLayout(); err != nil {
		return nil, err
	}
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
		authPath := filepath.Join(home, "auth.json")
		if _, err := os.Stat(authPath); err != nil {
			continue
		}
		snapshot, snapshotErr := auth.LoadAuthSnapshot(authPath)
		if snapshotErr == nil {
			var renameErr error
			home, renameErr = store.maybeRenameProfileHome(home, entry.Name(), snapshot)
			if renameErr != nil {
				return nil, renameErr
			}
		}
		profile, err := store.loadStoredProfile(home, snapshot, snapshotErr)
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

func (store *ProfileStore) maybeRenameProfileHome(home string, currentProfileID string, snapshot model.AuthSnapshot) (string, error) {
	desiredProfileID := auth.CanonicalProfileID(snapshot)
	if desiredProfileID == "" || desiredProfileID == currentProfileID {
		return home, nil
	}
	targetHome := store.profileHome(desiredProfileID)
	if _, err := os.Stat(targetHome); err == nil {
		return home, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := os.Rename(home, targetHome); err != nil {
		return "", err
	}
	return targetHome, nil
}

func (store *ProfileStore) loadStoredProfile(home string, snapshot model.AuthSnapshot, snapshotErr error) (model.StoredProfile, error) {
	metaPath := filepath.Join(home, "meta.json")
	var existing model.ProfileMeta
	hasExisting := false
	if content, err := os.ReadFile(metaPath); err == nil {
		if json.Unmarshal(content, &existing) == nil {
			hasExisting = true
		}
	}
	if snapshotErr == nil {
		profileID := filepath.Base(home)
		meta, changed := buildProfileMeta(profileID, snapshot, existing, hasExisting)
		if changed {
			if err := WriteJSONAtomic(metaPath, meta); err != nil {
				return model.StoredProfile{}, err
			}
		}
		return model.StoredProfile{Home: home, Meta: meta}, nil
	}
	if hasExisting {
		return model.StoredProfile{Home: home, Meta: existing}, nil
	}
	return model.StoredProfile{}, snapshotErr
}

func buildProfileMeta(profileID string, snapshot model.AuthSnapshot, existing model.ProfileMeta, hasExisting bool) (model.ProfileMeta, bool) {
	now := auth.UTCNowISO()
	meta := existing
	if !hasExisting {
		meta = model.ProfileMeta{
			CreatedAt: now,
			UpdatedAt: now,
		}
	}
	changed := false
	if meta.ProfileID != profileID {
		meta.ProfileID = profileID
		changed = true
	}
	label := auth.DefaultProfileLabel(snapshot, nil)
	if meta.Label != label {
		meta.Label = label
		changed = true
	}
	email := auth.NormalizeEmail(snapshot.Email)
	if meta.Email != email {
		meta.Email = email
		changed = true
	}
	if meta.AccountID != snapshot.AccountID {
		meta.AccountID = snapshot.AccountID
		changed = true
	}
	if meta.PlanType != snapshot.PlanType {
		meta.PlanType = snapshot.PlanType
		changed = true
	}
	if meta.AuthMode != snapshot.AuthMode {
		meta.AuthMode = snapshot.AuthMode
		changed = true
	}
	if meta.HasRefreshToken != snapshot.HasRefreshToken {
		meta.HasRefreshToken = snapshot.HasRefreshToken
		changed = true
	}
	if meta.SubscriptionActiveStart != snapshot.SubscriptionActiveStart {
		meta.SubscriptionActiveStart = snapshot.SubscriptionActiveStart
		changed = true
	}
	if meta.SubscriptionActiveUntil != snapshot.SubscriptionActiveUntil {
		meta.SubscriptionActiveUntil = snapshot.SubscriptionActiveUntil
		changed = true
	}
	if meta.SubscriptionLastChecked != snapshot.SubscriptionLastChecked {
		meta.SubscriptionLastChecked = snapshot.SubscriptionLastChecked
		changed = true
	}
	if meta.AuthSHA256 != snapshot.AuthSHA256 {
		meta.AuthSHA256 = snapshot.AuthSHA256
		changed = true
	}
	if meta.LastRefresh != snapshot.LastRefresh {
		meta.LastRefresh = snapshot.LastRefresh
		changed = true
	}
	if !sameInt64Ptr(meta.AccessExp, snapshot.AccessExp) {
		meta.AccessExp = snapshot.AccessExp
		changed = true
	}
	if !sameInt64Ptr(meta.IDExp, snapshot.IDExp) {
		meta.IDExp = snapshot.IDExp
		changed = true
	}
	if meta.CreatedAt == "" {
		meta.CreatedAt = now
		changed = true
	}
	if changed || meta.UpdatedAt == "" {
		meta.UpdatedAt = now
	}
	return meta, changed || !hasExisting
}

func sameInt64Ptr(left *int64, right *int64) bool {
	switch {
	case left == nil && right == nil:
		return true
	case left == nil || right == nil:
		return false
	default:
		return *left == *right
	}
}

func (store *ProfileStore) CleanupLayout() error {
	if _, err := os.Stat(store.Root); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	entries, err := os.ReadDir(store.Root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == "profiles" || name == "settings.json" {
			continue
		}
		if err := os.RemoveAll(filepath.Join(store.Root, name)); err != nil {
			return err
		}
	}
	if _, err := os.Stat(store.ProfilesDir); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	profileEntries, err := os.ReadDir(store.ProfilesDir)
	if err != nil {
		return err
	}
	for _, entry := range profileEntries {
		path := filepath.Join(store.ProfilesDir, entry.Name())
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") || strings.HasPrefix(entry.Name(), "_") {
			if err := os.RemoveAll(path); err != nil {
				return err
			}
			continue
		}
		if err := sanitizeProfileHome(path); err != nil {
			return err
		}
	}
	return nil
}

func sanitizeProfileHome(home string) error {
	entries, err := os.ReadDir(home)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == "auth.json" || name == "meta.json" {
			continue
		}
		if err := os.RemoveAll(filepath.Join(home, name)); err != nil {
			return err
		}
	}
	return nil
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
	effectiveSnapshot := snapshot
	if account != nil {
		if effectiveSnapshot.Email == "" && account.Email != "" {
			effectiveSnapshot.Email = account.Email
		}
		if account.PlanType != "" {
			effectiveSnapshot.PlanType = account.PlanType
		}
	}
	profileID := auth.CanonicalProfileID(effectiveSnapshot)
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
	meta := model.ProfileMeta{}
	if existing != nil {
		meta = *existing
	}
	meta, _ = buildProfileMeta(profileID, effectiveSnapshot, meta, existing != nil)
	meta.LastChecked = auth.UTCNowISO()
	meta.Source = source
	meta.Status = status
	meta.LastError = lastError
	meta.Quota = quota
	if err := WriteJSONAtomic(metaPath, meta); err != nil {
		return model.StoredProfile{}, err
	}
	if err := sanitizeProfileHome(finalHome); err != nil {
		return model.StoredProfile{}, err
	}
	return model.StoredProfile{Home: finalHome, Meta: meta}, nil
}

func (store *ProfileStore) DeleteProfile(profileID string) error {
	return os.RemoveAll(store.profileHome(profileID))
}

func (store *ProfileStore) SwitchProfile(profileID string, targetCodexHome string) (string, error) {
	profile, err := store.GetProfile(profileID)
	if err != nil {
		return "", err
	}
	targetCodexHome = auth.ExpandPath(targetCodexHome)
	if err := ensurePrivateDir(targetCodexHome); err != nil {
		return "", err
	}
	targetAuth := filepath.Join(targetCodexHome, "auth.json")
	if err := CopyFileAtomic(profile.AuthPath(), targetAuth); err != nil {
		return "", err
	}
	return targetAuth, nil
}
