package fx

import "encoding/json"

func rawString(raw map[string]json.RawMessage, keys ...string) string {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok {
			continue
		}
		var out string
		if err := json.Unmarshal(value, &out); err == nil {
			return out
		}
	}
	return ""
}

func rawInt(raw map[string]json.RawMessage, keys ...string) int {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok {
			continue
		}
		var out int
		if err := json.Unmarshal(value, &out); err == nil {
			return out
		}
	}
	return 0
}

func rawBool(raw map[string]json.RawMessage, keys ...string) bool {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok {
			continue
		}
		var out bool
		if err := json.Unmarshal(value, &out); err == nil {
			return out
		}
	}
	return false
}

func rawLiteral(raw map[string]json.RawMessage, key string) string {
	value, ok := raw[key]
	if !ok || string(value) == "null" {
		return ""
	}
	return string(value)
}

func rawNonNull(raw map[string]json.RawMessage, key string) json.RawMessage {
	value, ok := raw[key]
	if !ok || string(value) == "null" {
		return nil
	}
	return append(json.RawMessage(nil), value...)
}
