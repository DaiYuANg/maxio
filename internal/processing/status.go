package processing

import "strings"

func NormalizeStatus(status string) string {
	return strings.TrimSpace(strings.ToLower(status))
}

func ValidStatus(status string) bool {
	switch NormalizeStatus(status) {
	case StatusSkipped, StatusQueued, StatusRunning, StatusSucceeded, StatusFailed, StatusBlocked:
		return true
	default:
		return false
	}
}
