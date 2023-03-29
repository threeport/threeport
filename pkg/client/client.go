package client

import (
	"encoding/json"
	"fmt"
)

func MarshalObject(object interface{}) ([]byte, error) {
	objectJSON, err := json.Marshal(object)
	if err != nil {
		return []byte{}, fmt.Errorf("failed to marshal object to JSON: %w", err)
	}

	return objectJSON, nil
}
