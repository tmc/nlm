package authuser

import "strings"

// Normalize returns value for an explicitly selected non-default account.
// The browser omits both authuser carriers for the default account.
func Normalize(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "0" {
		return ""
	}
	return value
}
