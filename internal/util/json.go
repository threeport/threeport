package util

import (
	"encoding/json"
	"fmt"
)

// MarshalObject takes an object interface and returns its json byte array.
func MarshalObject(object interface{}) ([]byte, error) {
	objectJSON, err := json.Marshal(object)
	if err != nil {
		return []byte{}, fmt.Errorf("failed to marshal object to JSON: %w", err)
	}

	return objectJSON, nil
}
