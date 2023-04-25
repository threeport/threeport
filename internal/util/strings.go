package util

import (
	"math/rand"
	"strings"
	"time"
)

const charset = "abcdefghijklmnopqrstuvwxyz0123456789"

// SliceContains returns true if a slice contains a certain string.
func SliceContains(sl []string, name string, caseSensitive bool) bool {
	for _, value := range sl {
		switch caseSensitive {
		case true:
			if value == name {
				return true
			}
		case false:
			if strings.EqualFold(value, name) {
				return true
			}
		}
	}
	return false
}

// RandomString returns a random string with the provided length.
func RandomString(length int) string {
	seededRand := rand.New(rand.NewSource(time.Now().UnixNano()))

	bytes := make([]byte, length)
	for i := range bytes {
		bytes[i] = charset[seededRand.Intn(len(charset))]
	}

	return string(bytes)
}
