package conv

import (
	"encoding/json"
	"fmt"
)

func Map(value any) map[string]any {
	if value == nil {
		return nil
	}
	if typed, ok := value.(map[string]any); ok {
		return typed
	}
	return nil
}

func Slice(value any) []any {
	if value == nil {
		return nil
	}
	if typed, ok := value.([]any); ok {
		return typed
	}
	return nil
}

func String(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return fmt.Sprintf("%v", typed)
	}
}

func Bool(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return typed == "true"
	default:
		return false
	}
}

func Int(value any) *int {
	switch typed := value.(type) {
	case nil:
		return nil
	case int:
		v := typed
		return &v
	case int64:
		v := int(typed)
		return &v
	case float64:
		v := int(typed)
		return &v
	case json.Number:
		if parsed, err := typed.Int64(); err == nil {
			v := int(parsed)
			return &v
		}
	}
	return nil
}

func Int64(value any) *int64 {
	switch typed := value.(type) {
	case nil:
		return nil
	case int:
		v := int64(typed)
		return &v
	case int64:
		v := typed
		return &v
	case float64:
		v := int64(typed)
		return &v
	case json.Number:
		if parsed, err := typed.Int64(); err == nil {
			v := parsed
			return &v
		}
	}
	return nil
}
