package store

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func jwt(payload map[string]any) string {
	headerBytes, _ := json.Marshal(map[string]any{"alg": "none"})
	bodyBytes, _ := json.Marshal(payload)
	header := base64.RawURLEncoding.EncodeToString(headerBytes)
	body := base64.RawURLEncoding.EncodeToString(bodyBytes)
	return header + "." + body + "."
}

func writeAuthFile(t *testing.T, authPath string, email string, accountID string) {
	t.Helper()
	content, _ := json.Marshal(map[string]any{
		"auth_mode": "chatgpt",
		"tokens": map[string]any{
			"id_token": jwt(map[string]any{
				"email": email,
				"https://api.openai.com/auth": map[string]any{
					"chatgpt_account_id": accountID,
					"chatgpt_plan_type":  "team",
				},
			}),
			"access_token": jwt(map[string]any{
				"https://api.openai.com/auth": map[string]any{
					"chatgpt_account_id": accountID,
					"chatgpt_plan_type":  "team",
				},
			}),
			"refresh_token": "rt_" + accountID,
			"account_id":    accountID,
		},
	})
	if err := os.MkdirAll(filepath.Dir(authPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(authPath, content, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestListProfilesCleansObsoleteStoreArtifacts(t *testing.T) {
	root := t.TempDir()
	profileHome := filepath.Join(root, "profiles", "acct-1")
	if err := os.MkdirAll(filepath.Join(profileHome, "skills", ".system"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(profileHome, "tmp", "arg0"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(profileHome, "memories"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "backups"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "tmp"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profileHome, "auth.json"), []byte(`not-json`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profileHome, "meta.json"), []byte(`{"profile_id":"acct-1","label":"acct-1 [team]"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profileHome, "state_5.sqlite"), []byte("junk"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".DS_Store"), []byte("junk"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "profiles", ".DS_Store"), []byte("junk"), 0o600); err != nil {
		t.Fatal(err)
	}

	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	profiles, err := store.ListProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(profiles))
	}

	for _, path := range []string{
		filepath.Join(root, "backups"),
		filepath.Join(root, "tmp"),
		filepath.Join(root, ".DS_Store"),
		filepath.Join(root, "profiles", ".DS_Store"),
		filepath.Join(profileHome, "skills"),
		filepath.Join(profileHome, "tmp"),
		filepath.Join(profileHome, "memories"),
		filepath.Join(profileHome, "state_5.sqlite"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be removed", path)
		}
	}

	for _, path := range []string{
		filepath.Join(profileHome, "auth.json"),
		filepath.Join(profileHome, "meta.json"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s to remain: %v", path, err)
		}
	}
}

func TestCreateRuntimeHomeKeepsProbeDataOutsideStore(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	authSource := filepath.Join(root, "profiles", "acct-1", "auth.json")
	if err := os.MkdirAll(filepath.Dir(authSource), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(authSource, []byte(`{"auth_mode":"chatgpt"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	runtimeHome, err := store.CreateRuntimeHome("probe", authSource)
	if err != nil {
		t.Fatal(err)
	}
	defer store.CleanupRuntimeHome(runtimeHome)

	relative, err := filepath.Rel(root, runtimeHome)
	if err != nil {
		t.Fatal(err)
	}
	prefix := ".." + string(os.PathSeparator)
	if relative != ".." && !strings.HasPrefix(relative, prefix) {
		t.Fatalf("expected runtime home outside store root, got %s", runtimeHome)
	}
	content, err := os.ReadFile(filepath.Join(runtimeHome, "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != `{"auth_mode":"chatgpt"}` {
		t.Fatalf("unexpected copied auth content: %s", string(content))
	}
}

func TestUpsertProfileDoesNotOverwriteDifferentEmailsWithSameAccountID(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}

	firstHome := filepath.Join(root, "source-a")
	secondHome := filepath.Join(root, "source-b")
	writeAuthFile(t, filepath.Join(firstHome, "auth.json"), "team17a@711511.xyz", "acct-shared")
	writeAuthFile(t, filepath.Join(secondHome, "auth.json"), "team17@711511.xyz", "acct-shared")

	firstProfile, err := store.UpsertProfileFromHome(firstHome, "login", nil, nil, "ok", "")
	if err != nil {
		t.Fatal(err)
	}
	secondProfile, err := store.UpsertProfileFromHome(secondHome, "login", nil, nil, "ok", "")
	if err != nil {
		t.Fatal(err)
	}

	if firstProfile.Meta.ProfileID == secondProfile.Meta.ProfileID {
		t.Fatalf("expected different profile ids, got %s", firstProfile.Meta.ProfileID)
	}
	if firstProfile.Meta.ProfileID != "team17a_at_711511.xyz" {
		t.Fatalf("unexpected first profile id: %s", firstProfile.Meta.ProfileID)
	}
	if secondProfile.Meta.ProfileID != "team17_at_711511.xyz" {
		t.Fatalf("unexpected second profile id: %s", secondProfile.Meta.ProfileID)
	}

	profiles, err := store.ListProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(profiles))
	}
}
