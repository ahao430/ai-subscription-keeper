package sensitize

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
)

// Keys whose values must be masked in logs / raw responses.
var sensitiveKeyPattern = regexp.MustCompile(`(?i)(token|secret|password|passwd|authorization|api[_-]?key|access[_-]?key|private[_-]?key|session|credential)`)

// bearerPattern masks long bearer-style tokens that appear as plain values.
var bearerPattern = regexp.MustCompile(`(?i)(bearer\s+|sk-|cy_)[A-Za-z0-9_\-\.]{8,}`)

const keepPrefix = 4

// JSON walks the given JSON document and masks values of sensitive keys as well
// as bearer-like token strings. It returns the original bytes when the document
// is not valid JSON.
func JSON(raw []byte) []byte {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return raw
	}
	var doc any
	if err := json.Unmarshal(trimmed, &doc); err != nil {
		// Not JSON: mask bearer-like tokens textually.
		return bearerPattern.ReplaceAllFunc(trimmed, maskValue)
	}
	out := walk(doc)
	buf, err := json.Marshal(out)
	if err != nil {
		return trimmed
	}
	return buf
}

func walk(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			if sensitiveKeyPattern.MatchString(k) {
				if s, ok := val.(string); ok && s != "" {
					t[k] = maskString(s)
					continue
				}
			}
			t[k] = walk(val)
		}
		return t
	case []any:
		for i, val := range t {
			t[i] = walk(val)
		}
		return t
	case string:
		return string(bearerPattern.ReplaceAllFunc([]byte(t), maskValue))
	default:
		return v
	}
}

func maskValue(b []byte) []byte {
	s := string(b)
	return []byte(maskString(s))
}

func maskString(s string) string {
	if len(s) <= keepPrefix {
		return strings.Repeat("*", len(s))
	}
	return s[:keepPrefix] + "****"
}
