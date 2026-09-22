package v0

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

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

// JSONDefinitionsEqual reports whether two JSON definitions describe the same
// thing.
//
// The comparison is on the decoded values, not the bytes.  One side of a
// comparison like this has usually been round-tripped through the database and
// back, and nothing guarantees it returns byte-identical to what went in - key
// order and whitespace are free to change.  Comparing bytes would report a
// difference on every call and quietly undo whatever the comparison was meant
// to avoid.
//
// Two absent definitions are equal; one absent and one present are not.
func JSONDefinitionsEqual(a, b *datatypes.JSON) (bool, error) {
	if a == nil || b == nil {
		return a == nil && b == nil, nil
	}

	var decodedA, decodedB interface{}
	if err := json.Unmarshal(*a, &decodedA); err != nil {
		return false, fmt.Errorf("failed to unmarshal the first JSON definition: %w", err)
	}
	if err := json.Unmarshal(*b, &decodedB); err != nil {
		return false, fmt.Errorf("failed to unmarshal the second JSON definition: %w", err)
	}

	return reflect.DeepEqual(decodedA, decodedB), nil
}
