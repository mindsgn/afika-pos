package posstore

import (
	"crypto/rand"
	"encoding/hex"
)

// NewID returns a random 128-bit hex identifier. Random ids let the API build a
// complete sale, its lines, and its stock movements before writing anything, so
// the whole sale fits in one atomic batch.
func NewID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// crypto/rand never fails on the supported platforms; a panic here is
		// safer than emitting a predictable primary key.
		panic("posstore: generate id: " + err.Error())
	}
	return hex.EncodeToString(buf[:])
}

func newID() string { return NewID() }
