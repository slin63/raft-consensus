package hashing

import (
	"crypto/sha1"
	"encoding/binary"
	"log"
)

// MHash maps an address to one of 2^m logical points on a virtual ring.
func MHash(address string, m int) int {
	h := sha1.New()
	if _, err := h.Write([]byte(address)); err != nil {
		log.Fatal(err)
	}
	b := h.Sum(nil)

	// Truncate down to m bits.
	pid := binary.BigEndian.Uint64(b) >> (64 - m)

	return int(pid)
}
