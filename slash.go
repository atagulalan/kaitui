package main

import "strings"

// parseSlash returns the normalized command (e.g. "/quit") if s is a slash command.
func parseSlash(s string) (cmd string, ok bool) {
	s = strings.TrimSpace(s)
	if s == "" || !strings.HasPrefix(s, "/") {
		return "", false
	}
	// first token only
	if i := strings.IndexAny(s, " \t"); i >= 0 {
		s = s[:i]
	}
	return strings.ToLower(s), true
}
