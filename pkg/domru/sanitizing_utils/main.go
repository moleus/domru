package sanitizing_utils

import "strings"

func KeepFirstNCharacters(s string, n int) string {
	if len(s) <= n {
		return strings.Repeat("*", len(s))
	}
	return s[:n] + strings.Repeat("*", len(s)-n)
}
