package model

import (
	"path/filepath"

	"codexswitch/internal/conv"
)

type RateLimitWindow struct {
	UsedPercent        int    `json:"usedPercent"`
	WindowDurationMins *int   `json:"windowDurationMins,omitempty"`
	ResetsAt           *int64 `json:"resetsAt,omitempty"`
}

func RateLimitWindowFromMap(data map[string]any) *RateLimitWindow {
	if len(data) == 0 {
		return nil
	}
	usedPercent := 0
	if value := conv.Int(data["usedPercent"]); value != nil {
		usedPercent = *value
	}
	return &RateLimitWindow{
		UsedPercent:        usedPercent,
		WindowDurationMins: conv.Int(data["windowDurationMins"]),
		ResetsAt:           conv.Int64(data["resetsAt"]),
	}
}

type CreditsSnapshot struct {
	HasCredits bool   `json:"hasCredits"`
	Unlimited  bool   `json:"unlimited"`
	Balance    string `json:"balance,omitempty"`
}

func CreditsSnapshotFromMap(data map[string]any) *CreditsSnapshot {
	if len(data) == 0 {
		return nil
	}
	return &CreditsSnapshot{
		HasCredits: conv.Bool(data["hasCredits"]),
		Unlimited:  conv.Bool(data["unlimited"]),
		Balance:    conv.String(data["balance"]),
	}
}

type RateLimitSnapshot struct {
	LimitID   string           `json:"limitId,omitempty"`
	LimitName string           `json:"limitName,omitempty"`
	PlanType  string           `json:"planType,omitempty"`
	Primary   *RateLimitWindow `json:"primary,omitempty"`
	Secondary *RateLimitWindow `json:"secondary,omitempty"`
	Credits   *CreditsSnapshot `json:"credits,omitempty"`
}

func RateLimitSnapshotFromMap(data map[string]any) *RateLimitSnapshot {
	if len(data) == 0 {
		return nil
	}
	return &RateLimitSnapshot{
		LimitID:   conv.String(data["limitId"]),
		LimitName: conv.String(data["limitName"]),
		PlanType:  conv.String(data["planType"]),
		Primary:   RateLimitWindowFromMap(conv.Map(data["primary"])),
		Secondary: RateLimitWindowFromMap(conv.Map(data["secondary"])),
		Credits:   CreditsSnapshotFromMap(conv.Map(data["credits"])),
	}
}

type AccountInfo struct {
	AccountType        string
	Email              string
	PlanType           string
	RequiresOpenAIAuth bool
}

func AccountInfoFromResponse(data map[string]any) *AccountInfo {
	if len(data) == 0 {
		return nil
	}
	account := conv.Map(data["account"])
	if len(account) == 0 {
		return &AccountInfo{
			AccountType:        "unknown",
			RequiresOpenAIAuth: conv.Bool(data["requiresOpenaiAuth"]),
		}
	}
	return &AccountInfo{
		AccountType:        conv.String(account["type"]),
		Email:              conv.String(account["email"]),
		PlanType:           conv.String(account["planType"]),
		RequiresOpenAIAuth: conv.Bool(data["requiresOpenaiAuth"]),
	}
}

type AuthSnapshot struct {
	AuthMode                string
	AccountID               string
	Email                   string
	Name                    string
	PlanType                string
	SubscriptionActiveStart string
	SubscriptionActiveUntil string
	SubscriptionLastChecked string
	AccessExp               *int64
	IDExp                   *int64
	LastRefresh             string
	AuthSHA256              string
	SourcePath              string
	HasRefreshToken         bool
}

func (snapshot AuthSnapshot) DisplayLabel() string {
	switch {
	case snapshot.Email != "":
		return snapshot.Email
	case snapshot.Name != "":
		return snapshot.Name
	case snapshot.AccountID != "":
		return snapshot.AccountID
	default:
		return "unknown"
	}
}

type ProfileMeta struct {
	ProfileID               string             `json:"profile_id"`
	Label                   string             `json:"label"`
	AccountID               string             `json:"account_id,omitempty"`
	Email                   string             `json:"email,omitempty"`
	PlanType                string             `json:"plan_type,omitempty"`
	AuthMode                string             `json:"auth_mode,omitempty"`
	HasRefreshToken         bool               `json:"has_refresh_token,omitempty"`
	SubscriptionActiveStart string             `json:"subscription_active_start,omitempty"`
	SubscriptionActiveUntil string             `json:"subscription_active_until,omitempty"`
	SubscriptionLastChecked string             `json:"subscription_last_checked,omitempty"`
	AuthSHA256              string             `json:"auth_sha256"`
	LastRefresh             string             `json:"last_refresh,omitempty"`
	AccessExp               *int64             `json:"access_exp,omitempty"`
	IDExp                   *int64             `json:"id_exp,omitempty"`
	CreatedAt               string             `json:"created_at"`
	UpdatedAt               string             `json:"updated_at"`
	LastChecked             string             `json:"last_checked,omitempty"`
	Source                  string             `json:"source,omitempty"`
	Status                  string             `json:"status,omitempty"`
	LastError               string             `json:"last_error,omitempty"`
	Quota                   *RateLimitSnapshot `json:"quota,omitempty"`
}

type StoredProfile struct {
	Home string
	Meta ProfileMeta
}

func (profile StoredProfile) ProfileID() string {
	return profile.Meta.ProfileID
}

func (profile StoredProfile) AuthPath() string {
	return filepath.Join(profile.Home, "auth.json")
}

func (profile StoredProfile) MetaPath() string {
	return filepath.Join(profile.Home, "meta.json")
}

type AppSettings struct {
	TargetCodexHomeOverride string `json:"target_codex_home_override,omitempty"`
}
