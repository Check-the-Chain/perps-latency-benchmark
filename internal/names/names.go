package names

import "strings"

func Normalize(name string) string {
	return strings.ReplaceAll(strings.ReplaceAll(strings.ToLower(strings.TrimSpace(name)), "-", "_"), " ", "_")
}
