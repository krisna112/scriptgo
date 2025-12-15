package core

import (
	"crypto/rand"
	"fmt"
)

// GenerateUUID creates a standard RFC 4122 v4 UUID
func GenerateUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	// Version 4
	b[6] = (b[6] & 0x0f) | 0x40
	// Variant RFC 4122
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func FormatBytes(size float64) string {
	power := 1024.0
	n := 0
	labels := []string{"", "K", "M", "G", "T"}
	for size > power {
		size /= power
		n++
	}
	if n >= len(labels) {
		n = len(labels) - 1
	}
	return fmt.Sprintf("%.2f %sB", size, labels[n])
}
