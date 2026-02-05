package connector

import (
	"fmt"
	"strings"
	"unicode"
)

type QueueDirective struct {
	Cleaned      string
	QueueMode    QueueMode
	QueueReset   bool
	RawMode      string
	DebounceMs   *int
	Cap          *int
	DropPolicy   *QueueDropPolicy
	RawDebounce  string
	RawCap       string
	RawDrop      string
	HasDirective bool
	HasOptions   bool
	HasDebounce  bool
	HasCap       bool
	HasDrop      bool
}

func parseQueueDebounce(raw string) *int {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parsed, err := parseDurationMs(raw, "ms")
	if err != nil {
		return nil
	}
	value := int(parsed)
	if value < 0 {
		return nil
	}
	return &value
}

func parseQueueCap(raw string) *int {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	value := 0
	if _, err := fmt.Sscanf(trimmed, "%d", &value); err != nil {
		return nil
	}
	if value < 1 {
		return nil
	}
	return &value
}

func parseQueueDirectiveArgs(raw string) (consumed int, result QueueDirective) {
	i := 0
	for i < len(raw) && raw[i] <= ' ' {
		i++
	}
	if i < len(raw) && raw[i] == ':' {
		i++
		for i < len(raw) && raw[i] <= ' ' {
			i++
		}
	}
	consumed = i
	takeToken := func() string {
		if i >= len(raw) {
			return ""
		}
		start := i
		for i < len(raw) && raw[i] > ' ' {
			i++
		}
		token := raw[start:i]
		for i < len(raw) && raw[i] <= ' ' {
			i++
		}
		if token == "" {
			return ""
		}
		consumed = i
		return token
	}

	for i < len(raw) {
		token := takeToken()
		if token == "" {
			break
		}
		lowered := strings.ToLower(strings.TrimSpace(token))
		if lowered == "default" || lowered == "reset" || lowered == "clear" {
			result.QueueReset = true
			break
		}
		if strings.HasPrefix(lowered, "debounce:") || strings.HasPrefix(lowered, "debounce=") {
			parts := strings.SplitN(token, ":", 2)
			if len(parts) == 1 {
				parts = strings.SplitN(token, "=", 2)
			}
			if len(parts) > 1 {
				result.RawDebounce = parts[1]
				result.DebounceMs = parseQueueDebounce(parts[1])
				result.HasOptions = true
				result.HasDebounce = true
			}
			continue
		}
		if strings.HasPrefix(lowered, "cap:") || strings.HasPrefix(lowered, "cap=") {
			parts := strings.SplitN(token, ":", 2)
			if len(parts) == 1 {
				parts = strings.SplitN(token, "=", 2)
			}
			if len(parts) > 1 {
				result.RawCap = parts[1]
				result.Cap = parseQueueCap(parts[1])
				result.HasOptions = true
				result.HasCap = true
			}
			continue
		}
		if strings.HasPrefix(lowered, "drop:") || strings.HasPrefix(lowered, "drop=") {
			parts := strings.SplitN(token, ":", 2)
			if len(parts) == 1 {
				parts = strings.SplitN(token, "=", 2)
			}
			if len(parts) > 1 {
				result.RawDrop = parts[1]
				if policy, ok := normalizeQueueDropPolicy(parts[1]); ok {
					result.DropPolicy = &policy
				}
				result.HasOptions = true
				result.HasDrop = true
			}
			continue
		}
		if mode, ok := normalizeQueueMode(token); ok {
			result.QueueMode = mode
			result.RawMode = token
			continue
		}
		break
	}
	return consumed, result
}

func extractQueueDirective(body string) QueueDirective {
	if strings.TrimSpace(body) == "" {
		return QueueDirective{Cleaned: strings.TrimSpace(body)}
	}
	start, argsStart, ok := findQueueDirective(body)
	if !ok {
		return QueueDirective{Cleaned: strings.TrimSpace(body)}
	}
	consumed, parsed := parseQueueDirectiveArgs(body[argsStart:])
	cleanedRaw := strings.TrimSpace(body[:start] + " " + body[argsStart+consumed:])
	parsed.Cleaned = strings.TrimSpace(strings.Join(strings.Fields(cleanedRaw), " "))
	parsed.HasDirective = true
	return parsed
}

func findQueueDirective(body string) (start int, argsStart int, ok bool) {
	lower := strings.ToLower(body)
	offset := 0
	for {
		idx := strings.Index(lower, "/queue")
		if idx < 0 {
			return 0, 0, false
		}
		pos := offset + idx
		beforeOK := pos == 0 || unicode.IsSpace(rune(body[pos-1]))
		afterIdx := pos + len("/queue")
		afterOK := afterIdx == len(body)
		if !afterOK {
			next := rune(body[afterIdx])
			afterOK = unicode.IsSpace(next) || next == ':'
		}
		if beforeOK && afterOK {
			return pos, afterIdx, true
		}
		offset = afterIdx
		if offset >= len(body) {
			return 0, 0, false
		}
		lower = lower[idx+len("/queue"):]
	}
}
