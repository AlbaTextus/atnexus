package mail

import (
	"net/mail"
	"strings"
)

// NormalizeAddress is the form addresses are stored, compared and rate limited in.
func NormalizeAddress(address string) string {
	return strings.ToLower(strings.TrimSpace(address))
}

// ValidAddress reports whether address is a bare deliverable mailbox. The
// reserved .invalid domain marks accounts that have no mailbox.
func ValidAddress(address string) bool {
	parsed, err := mail.ParseAddress(address)
	return err == nil && parsed.Address == address && len(address) <= 320 && !strings.ContainsAny(address, "\r\n") && !strings.HasSuffix(address, ".invalid")
}
