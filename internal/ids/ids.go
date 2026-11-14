// Package ids makes and checks the random identifiers of rmhub.
package ids

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/hex"
	"strings"
)

// AccountIDLen is the length of an account ID.
const AccountIDLen = 26

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// Random returns n random characters from a-z and 2-7.
func Random(n int) string {
	// Each base32 character holds 5 bits.
	buf := make([]byte, (n*5+7)/8)
	if _, err := rand.Read(buf); err != nil {
		panic("ids: crypto/rand failed: " + err.Error())
	}
	return strings.ToLower(b32.EncodeToString(buf))[:n]
}

// RandomHex returns n random bytes as lowercase hexadecimal text.
func RandomHex(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		panic("ids: crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(buf)
}

// NewAccountID returns a new random account ID.
func NewAccountID() string { return Random(AccountIDLen) }

// IsBase32 tells if s has exactly n characters from a-z and 2-7.
func IsBase32(s string, n int) bool {
	if len(s) != n {
		return false
	}
	for i := range len(s) {
		c := s[i]
		if (c < 'a' || c > 'z') && (c < '2' || c > '7') {
			return false
		}
	}
	return true
}

// IsAccountID tells if s is a valid account ID.
func IsAccountID(s string) bool { return IsBase32(s, AccountIDLen) }

// IsHash tells if s is 64 lowercase hexadecimal characters.
func IsHash(s string) bool {
	if len(s) != 64 {
		return false
	}
	for i := range len(s) {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
