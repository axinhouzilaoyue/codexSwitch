package state

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"codexswitch/internal/model"
	"codexswitch/internal/store"
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

func writeMetaFile(t *testing.T, metaPath string, meta model.ProfileMeta) {
	t.Helper()
	content, _ := json.Marshal(meta)
	if err := os.MkdirAll(filepath.Dir(metaPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(metaPath, content, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadMatchesCurrentProfileByEmailBeforeAccountID(t *testing.T) {
	root := t.TempDir()
	targetHome := filepath.Join(root, "target")

	store, err := store.New(filepath.Join(root, "profiles-store"))
	if err != nil {
		t.Fatal(err)
	}

	firstHome := filepath.Join(store.ProfilesDir, "team17_at_711511.xyz")
	writeAuthFile(t, filepath.Join(firstHome, "auth.json"), "team17@711511.xyz", "acct-shared")
	writeMetaFile(t, filepath.Join(firstHome, "meta.json"), model.ProfileMeta{
		ProfileID: "team17_at_711511.xyz",
		Email:     "team17@711511.xyz",
		AccountID: "acct-shared",
	})

	secondHome := filepath.Join(store.ProfilesDir, "team17a_at_711511.xyz")
	writeAuthFile(t, filepath.Join(secondHome, "auth.json"), "team17a@711511.xyz", "acct-shared")
	writeMetaFile(t, filepath.Join(secondHome, "meta.json"), model.ProfileMeta{
		ProfileID: "team17a_at_711511.xyz",
		Email:     "team17a@711511.xyz",
		AccountID: "acct-shared",
	})

	writeAuthFile(t, filepath.Join(targetHome, "auth.json"), "team17a@711511.xyz", "acct-shared")

	snapshot, err := Load(store, targetHome)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.CurrentProfileID != "team17a_at_711511.xyz" {
		t.Fatalf("unexpected current profile id: %s", snapshot.CurrentProfileID)
	}
}
