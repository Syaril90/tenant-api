package httpserver

import "strings"

func PathValue(path, prefix string) (string, bool) {
	if !strings.HasPrefix(path, prefix) {
		return "", false
	}

	value := strings.TrimPrefix(path, prefix)
	if value == "" || value == path || strings.Contains(value, "/") {
		return "", false
	}

	return value, true
}
