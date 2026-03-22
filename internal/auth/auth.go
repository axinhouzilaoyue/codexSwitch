package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"codexswitch/internal/conv"
	"codexswitch/internal/model"
)

const AuthNamespace = "https://api.openai.com/auth"

type authFile struct {
	AuthMode    string `json:"auth_mode"`
	LastRefresh string `json:"last_refresh"`
	Tokens      struct {
		IDToken      string `json:"id_token"`
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		AccountID    string `json:"account_id"`
	} `json:"tokens"`
}

func UTCNowISO() string {
	return time.Now().UTC().Truncate(time.Second).Format(time.RFC3339)
}

func FileSHA256(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:]), nil
}

func DecodeJWTPayload(token string) map[string]any {
	if token == "" {
		return map[string]any{}
	}
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return map[string]any{}
	}
	decoded, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return map[string]any{}
	}
	payload := map[string]any{}
	if err := json.Unmarshal(decoded, &payload); err != nil {
		return map[string]any{}
	}
	return payload
}

func LoadAuthSnapshot(path string) (model.AuthSnapshot, error) {
	var file authFile
	content, err := os.ReadFile(path)
	if err != nil {
		return model.AuthSnapshot{}, err
	}
	if err := json.Unmarshal(content, &file); err != nil {
		return model.AuthSnapshot{}, err
	}
	idToken := DecodeJWTPayload(file.Tokens.IDToken)
	accessToken := DecodeJWTPayload(file.Tokens.AccessToken)
	idAuth := conv.Map(idToken[AuthNamespace])
	accessAuth := conv.Map(accessToken[AuthNamespace])
	sha, err := FileSHA256(path)
	if err != nil {
		return model.AuthSnapshot{}, err
	}
	accountID := file.Tokens.AccountID
	if accountID == "" {
		accountID = conv.String(idAuth["chatgpt_account_id"])
	}
	if accountID == "" {
		accountID = conv.String(accessAuth["chatgpt_account_id"])
	}
	planType := conv.String(idAuth["chatgpt_plan_type"])
	if planType == "" {
		planType = conv.String(accessAuth["chatgpt_plan_type"])
	}
	email := conv.String(idToken["email"])
	if email == "" {
		email = conv.String(accessToken["email"])
	}
	name := conv.String(idToken["name"])
	if name == "" {
		name = conv.String(accessToken["name"])
	}
	return model.AuthSnapshot{
		AuthMode:  file.AuthMode,
		AccountID: accountID,
		Email:     email,
		Name:      name,
		PlanType:  planType,
		SubscriptionActiveStart: firstString(
			conv.String(idAuth["chatgpt_subscription_active_start"]),
			conv.String(accessAuth["chatgpt_subscription_active_start"]),
		),
		SubscriptionActiveUntil: firstString(
			conv.String(idAuth["chatgpt_subscription_active_until"]),
			conv.String(accessAuth["chatgpt_subscription_active_until"]),
		),
		SubscriptionLastChecked: firstString(
			conv.String(idAuth["chatgpt_subscription_last_checked"]),
			conv.String(accessAuth["chatgpt_subscription_last_checked"]),
		),
		AccessExp:       conv.Int64(accessToken["exp"]),
		IDExp:           conv.Int64(idToken["exp"]),
		LastRefresh:     file.LastRefresh,
		AuthSHA256:      sha,
		SourcePath:      path,
		HasRefreshToken: file.Tokens.RefreshToken != "",
	}, nil
}

func firstString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func SnapshotWithAccount(snapshot model.AuthSnapshot, account *model.AccountInfo) model.AuthSnapshot {
	if account == nil {
		return snapshot
	}
	if account.Email != "" {
		snapshot.Email = account.Email
	}
	if account.PlanType != "" {
		snapshot.PlanType = account.PlanType
	}
	return snapshot
}

func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func CanonicalProfileID(snapshot model.AuthSnapshot) string {
	if email := NormalizeEmail(snapshot.Email); email != "" {
		replacer := strings.NewReplacer("@", "_at_", "/", "_")
		return replacer.Replace(email)
	}
	if snapshot.AccountID != "" {
		return snapshot.AccountID
	}
	raw := snapshot.SourcePath + ":" + snapshot.AuthSHA256
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:8])
}

func DefaultProfileLabel(snapshot model.AuthSnapshot, account *model.AccountInfo) string {
	email := snapshot.Email
	plan := snapshot.PlanType
	if account != nil {
		if account.Email != "" {
			email = account.Email
		}
		if account.PlanType != "" {
			plan = account.PlanType
		}
	}
	base := email
	if base == "" {
		base = snapshot.Name
	}
	if base == "" {
		base = snapshot.AccountID
	}
	if base == "" {
		base = "unknown"
	}
	if plan != "" {
		return base + " [" + plan + "]"
	}
	return base
}

func ExpandPath(path string) string {
	if path == "" {
		return ""
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	return path
}
