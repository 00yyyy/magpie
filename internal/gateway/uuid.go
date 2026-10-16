package gateway

import (
	"crypto/rand"
	"fmt"
)

// newUUID is a random (version 4) UUID.
func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// joinNonEmpty joins two texts with a blank line, or is the one that isn't
// empty.
func joinNonEmpty(a, b string) string {
	if a == "" || b == "" {
		return a + b
	}
	return a + "\n\n" + b
}
