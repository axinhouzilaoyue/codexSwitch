package auth

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func jwt(payload map[string]any) string {
	headerBytes, _ := json.Marshal(map[string]any{"alg": "none"})
	bodyBytes, _ := json.Marshal(payload)
	header := base64.RawURLEncoding.EncodeToString(headerBytes)
	body := base64.RawURLEncoding.EncodeToString(bodyBytes)
	return header + "." + body + "."
}

func TestDecodeJWTPayload(t *testing.T) {
	token := jwt(map[string]any{"sub": "user-1", "exp": 123})
	if DecodeJWTPayload(token)["sub"] != "user-1" {
		t.Fatalf("expected user-1")
	}
}

func TestLoadAuthSnapshot(t *testing.T) {
	tempDir := t.TempDir()
	authPath := filepath.Join(tempDir, "auth.json")
	content, _ := json.Marshal(map[string]any{
		"auth_mode":      "chatgpt",
		"OPENAI_API_KEY": nil,
		"tokens": map[string]any{
			"id_token": jwt(map[string]any{
				"email": "team@example.com",
				"name":  "tester",
				"exp":   111,
				AuthNamespace: map[string]any{
					"chatgpt_account_id": "acct-1",
					"chatgpt_plan_type":  "team",
				},
			}),
			"access_token": jwt(map[string]any{
				"exp": 222,
				AuthNamespace: map[string]any{
					"chatgpt_account_id": "acct-1",
					"chatgpt_plan_type":  "team",
				},
			}),
			"refresh_token": "rt_xxx",
			"account_id":    "acct-1",
		},
		"last_refresh": "2026-03-20T00:00:00Z",
	})
	if err := os.WriteFile(authPath, content, 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := LoadAuthSnapshot(authPath)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Email != "team@example.com" {
		t.Fatalf("unexpected email: %s", snapshot.Email)
	}
	if snapshot.AccountID != "acct-1" {
		t.Fatalf("unexpected account id: %s", snapshot.AccountID)
	}
	if snapshot.PlanType != "team" {
		t.Fatalf("unexpected plan: %s", snapshot.PlanType)
	}
	if !snapshot.HasRefreshToken {
		t.Fatal("expected refresh token")
	}
	if snapshot.AccessExp == nil || *snapshot.AccessExp != 222 {
		t.Fatalf("unexpected access exp: %+v", snapshot.AccessExp)
	}
}

func TestCanonicalProfileIDPrefersAccountID(t *testing.T) {
	tempDir := t.TempDir()
	authPath := filepath.Join(tempDir, "auth.json")
	content, _ := json.Marshal(map[string]any{
		"auth_mode": "chatgpt",
		"tokens": map[string]any{
			"id_token": jwt(map[string]any{
				AuthNamespace: map[string]any{
					"chatgpt_account_id": "acct-xyz",
				},
			}),
			"access_token":  jwt(map[string]any{}),
			"refresh_token": "rt",
			"account_id":    "acct-xyz",
		},
	})
	if err := os.WriteFile(authPath, content, 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := LoadAuthSnapshot(authPath)
	if err != nil {
		t.Fatal(err)
	}
	if CanonicalProfileID(snapshot) != "acct-xyz" {
		t.Fatalf("unexpected profile id: %s", CanonicalProfileID(snapshot))
	}
}
