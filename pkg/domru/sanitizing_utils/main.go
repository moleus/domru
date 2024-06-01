package sanitizing_utils

import "strings"

func KeepFirstNCharacters(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func maskFromThirdCharacter(s string) string {
	if len(s) > 2 {
		return s[:2] + strings.Repeat("*", len(s)-2)
	}
	return s
}
