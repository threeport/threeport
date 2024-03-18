package v0

import (
	"encoding/json"
	"errors"
	"fmt"

	"gorm.io/datatypes"
)

// updateNamespace takes the JSON definition for a Kubernetes resource and sets
// the namespace.
func UpdateNamespace(jsonDef datatypes.JSON, namespace string) ([]byte, error) {
	// unmarshal the JSON into a map
	var mapDef map[string]interface{}
	err := json.Unmarshal(jsonDef, &mapDef)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON definition to map: %w", err)
	}

	// set the namespace field in the metadata
	if metadata, ok := mapDef["metadata"].(map[string]interface{}); ok {
		if mapDef["kind"] == "Gateway" {
			metadata["namespace"] = GatewaySystemNamespace
		} else {
			metadata["namespace"] = namespace
		}
	} else {
		return nil, errors.New("failed to find \"metadata\" field in JSON definition")
	}

	// marshal the modified map back to JSON
	modifiedJson, err := json.Marshal(mapDef)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON from modified map: %w", err)
	}

	return modifiedJson, nil
}
