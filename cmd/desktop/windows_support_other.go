//go:build !windows

package main

import (
	"os/exec"
	"strings"
)

func ensureWindowsAdmin() error {
	return nil
}

func hideCommandWindow(_ *exec.Cmd) {}

func splitWindowsCommandLine(line string) ([]string, error) {
	fields := make([]string, 0)
	var current strings.Builder
	inQuote := false
	escaped := false

	for _, r := range strings.TrimSpace(line) {
		switch {
		case escaped:
			current.WriteRune(r)
			escaped = false
		case r == '\\' && inQuote:
			current.WriteRune(r)
		case r == '"':
			inQuote = !inQuote
		case !inQuote && (r == ' ' || r == '\t' || r == '\n' || r == '\r'):
			if current.Len() > 0 {
				fields = append(fields, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}
	if escaped {
		current.WriteRune('\\')
	}
	if current.Len() > 0 {
		fields = append(fields, current.String())
	}
	return fields, nil
}
