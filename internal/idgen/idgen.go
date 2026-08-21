// Package idgen generates server-side resource identifiers.
//
// IDs are formatted as UUIDv4-style strings but are produced with only the
// standard library, since a full UUID library is not needed for this
// project's requirements.
package idgen

import (
	"crypto/rand"
	"fmt"
)

// New returns a new random, UUIDv4-formatted identifier.
func New() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("idgen: failed to read random bytes: " + err.Error())
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10

	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
