package v0

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"gorm.io/datatypes"
)

// operatorNamespaces are created by the support-services operator.
// A workload update must not replace them with a managed namespace.
// The operator does not export these names.
var operatorNamespaces = map[string]struct{}{
	"nukleros-ingress-system": {},
	"nukleros-certs-system":   {},
	"nukleros-secrets-system": {},
}

// KeepsOperatorNamespace reports whether the object already names a namespace
// the support-services operator creates.
func KeepsOperatorNamespace(jsonDef datatypes.JSON) bool {
	// decode metadata and match its namespace
	var mapDef map[string]interface{}
	if err := json.Unmarshal(jsonDef, &mapDef); err != nil {
		return false
	}
	metadata, ok := mapDef["metadata"].(map[string]interface{})
	if !ok {
		return false
	}

	return keepsOperatorNamespace(metadata)
}

// CountsTowardManagedNamespace reports whether a namespaced object should
// cause a managed namespace to be created for the workload.
func CountsTowardManagedNamespace(jsonDef datatypes.JSON) bool {
	// a gateway and an operator namespace stay where they are
	var mapDef map[string]interface{}
	if err := json.Unmarshal(jsonDef, &mapDef); err != nil {
		return true
	}
	if mapDef["kind"] == "Gateway" {
		return false
	}
	metadata, ok := mapDef["metadata"].(map[string]interface{})
	if ok && keepsOperatorNamespace(metadata) {
		return false
	}

	return true
}

// keepsOperatorNamespace reports whether metadata already names an operator namespace.
func keepsOperatorNamespace(metadata map[string]interface{}) bool {
	// match the namespace the object already carries
	namespace, _ := metadata["namespace"].(string)
	_, keep := operatorNamespaces[namespace]
	return keep
}

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
		switch {
		case mapDef["kind"] == "Gateway":
			metadata["namespace"] = GatewaySystemNamespace
		case keepsOperatorNamespace(metadata):
			// leave a support-services namespace in place
		default:
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
	return JSONDefinitionsEqualIgnoring(a, b)
}

// JSONDefinitionsEqualIgnoring reports whether two JSON definitions describe the
// same thing once the given fields are set aside.
//
// A caller comparing what it configures against what is stored often does not
// own every field of the stored copy - another reconciler may have filled some
// in and persisted them. Those fields differ on every comparison and say nothing
// about whether the caller's own configuration changed, so it names them here
// rather than seeing a difference forever.
//
// Each ignored field is a path of object keys, e.g. {"metadata", "namespace"}.
func JSONDefinitionsEqualIgnoring(a, b *datatypes.JSON, ignore ...[]string) (bool, error) {
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

	for _, path := range ignore {
		deleteJSONPath(decodedA, path)
		deleteJSONPath(decodedB, path)
	}

	return reflect.DeepEqual(decodedA, decodedB), nil
}

// deleteJSONPath removes a field from a decoded JSON value. A path that is not
// there is not an error: absence is the state being asked for.
func deleteJSONPath(value interface{}, path []string) {
	if len(path) == 0 {
		return
	}

	object, ok := value.(map[string]interface{})
	if !ok {
		return
	}
	if len(path) == 1 {
		delete(object, path[0])
		return
	}

	deleteJSONPath(object[path[0]], path[1:])
}

// JSONValuesEqual reports whether two decoded values are the same JSON.
// A Go int and the float64 produced by unmarshalling that int compare equal.
func JSONValuesEqual(left interface{}, right interface{}) (bool, error) {
	// round-trip both values, then compare
	canonicalLeft, err := canonicalJSON(left)
	if err != nil {
		return false, err
	}
	canonicalRight, err := canonicalJSON(right)
	if err != nil {
		return false, err
	}

	return reflect.DeepEqual(canonicalLeft, canonicalRight), nil
}

// canonicalJSON round-trips a value so numbers share one encoding.
func canonicalJSON(value interface{}) (interface{}, error) {
	// marshal and unmarshal so numbers share one encoding
	body, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON for comparison: %w", err)
	}
	var decoded interface{}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON for comparison: %w", err)
	}

	return decoded, nil
}
