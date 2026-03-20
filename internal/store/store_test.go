package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
	if err := os.WriteFile(filepath.Join(profileHome, "auth.json"), []byte(`{}`), 0o600); err != nil {
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
