package core

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

func GenerateRandomID(length int) string {
	b := make([]byte, length/2)
	rand.Read(b)
	return hex.EncodeToString(b)
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
