package codex

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"codexswitch/internal/model"
)

type sessionQuotaSnapshot struct {
	Quota     *model.RateLimitSnapshot
	Timestamp string
	Source    string
}

func LatestSessionRateLimits(codexHome string) (*model.RateLimitSnapshot, string, string, error) {
	sessionsRoot := filepath.Join(codexHome, "sessions")
	entries := []fileEntry{}
	if err := filepath.WalkDir(sessionsRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".jsonl") || !strings.HasPrefix(d.Name(), "rollout-") {
			return nil
		}
		info, statErr := d.Info()
		if statErr != nil {
			return nil
		}
		entries = append(entries, fileEntry{Path: path, ModTime: info.ModTime()})
		return nil
	}); err != nil {
		return nil, "", "", err
	}
	sort.Slice(entries, func(i int, j int) bool {
		return entries[i].ModTime.After(entries[j].ModTime)
	})
	for _, entry := range entries {
		snapshot, err := parseLatestQuotaFromRollout(entry.Path)
		if err != nil || snapshot == nil || snapshot.Quota == nil {
			continue
		}
		return snapshot.Quota, snapshot.Timestamp, snapshot.Source, nil
	}
	return nil, "", "", fmt.Errorf("no rate_limits snapshot found in %s", sessionsRoot)
}

type fileEntry struct {
	Path    string
	ModTime time.Time
}

func parseLatestQuotaFromRollout(path string) (*sessionQuotaSnapshot, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	buffer := make([]byte, 0, 64*1024)
	scanner.Buffer(buffer, 8*1024*1024)

	var latest *sessionQuotaSnapshot
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		record := map[string]any{}
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			continue
		}
		payload, _ := record["payload"].(map[string]any)
		if payload == nil {
			continue
		}
		rateLimits, _ := payload["rate_limits"].(map[string]any)
		if rateLimits == nil {
			continue
		}
		timestamp, _ := record["timestamp"].(string)
		quota := sessionRateLimitSnapshot(rateLimits, timestamp)
		if quota == nil {
			continue
		}
		latest = &sessionQuotaSnapshot{
			Quota:     quota,
			Timestamp: timestamp,
			Source:    path,
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return latest, nil
}

func sessionRateLimitSnapshot(data map[string]any, timestamp string) *model.RateLimitSnapshot {
	return &model.RateLimitSnapshot{
		LimitID:   stringValue(data["limit_id"]),
		LimitName: stringValue(data["limit_name"]),
		PlanType:  stringValue(data["plan_type"]),
		Primary:   sessionRateLimitWindow(mapValue(data["primary"]), timestamp),
		Secondary: sessionRateLimitWindow(mapValue(data["secondary"]), timestamp),
	}
}

func sessionRateLimitWindow(data map[string]any, timestamp string) *model.RateLimitWindow {
	if data == nil {
		return nil
	}
	usedPercent := 0
	switch value := data["used_percent"].(type) {
	case float64:
		usedPercent = int(math.Round(value))
	case int:
		usedPercent = value
	}
	windowMinutes := intPointer(data["window_minutes"])
	resetsAt := int64Pointer(data["resets_at"])
	if resetsAt == nil {
		if resetsIn := int64Pointer(data["resets_in_seconds"]); resetsIn != nil && timestamp != "" {
			if parsed, err := time.Parse(time.RFC3339, timestamp); err == nil {
				value := parsed.Unix() + *resetsIn
				resetsAt = &value
			}
		}
	}
	return &model.RateLimitWindow{
		UsedPercent:        usedPercent,
		WindowDurationMins: windowMinutes,
		ResetsAt:           resetsAt,
	}
}

func mapValue(value any) map[string]any {
	if typed, ok := value.(map[string]any); ok {
		return typed
	}
	return nil
}

func stringValue(value any) string {
	if typed, ok := value.(string); ok {
		return typed
	}
	return ""
}

func intPointer(value any) *int {
	switch typed := value.(type) {
	case float64:
		parsed := int(math.Round(typed))
		return &parsed
	case int:
		parsed := typed
		return &parsed
	default:
		return nil
	}
}

func int64Pointer(value any) *int64 {
	switch typed := value.(type) {
	case float64:
		parsed := int64(math.Round(typed))
		return &parsed
	case int:
		parsed := int64(typed)
		return &parsed
	case int64:
		parsed := typed
		return &parsed
	default:
		return nil
	}
}
