package sanitizing_utils

func KeepFirstNCharacters(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
