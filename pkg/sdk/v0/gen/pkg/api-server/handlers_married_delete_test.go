package apiserver

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/threeport/threeport/pkg/sdk/v0/gen"
)

// TestADeleteHandlerDeletesTheBaseAnObjectMarries covers #768: the delete
// a generated handler makes for an object with a foreign key tagged
// relationship:"marries" deletes the base that key names in the same
// transaction.  Any other relationship, or none, deletes the object alone.
func TestADeleteHandlerDeletesTheBaseAnObjectMarries(t *testing.T) {
	generator := &gen.Generator{ApiObjectGroups: []gen.ApiObjectGroup{{
		StructTags: map[string]map[string]map[string]string{
			"GkeDefinition": {
				"Name":                          {"validate": "required"},
				"KubernetesRuntimeDefinitionID": {"relationship": "marries"},
			},
			"GkeInstance": {
				"GkeDefinitionID":      {"relationship": "requires"},
				"GcpProviderID":        {"relationship": "requires"},
				"ObservabilityStackID": {"relationship": "describes"},
			},
			"WorkloadInstance": {
				"OwnerID": {"relationship": "owns,type=WorkloadDefinition"},
			},
			"KubernetesRuntimeDefinition": {
				"Name": {"validate": "required"},
			},
		},
	}}}

	for typeName, want := range map[string]string{
		"GkeDefinition":               "v0.DeleteWithMarriedBases(db, &gkeDefinition)",
		"GkeInstance":                 "db.Delete(&gkeInstance)",
		"WorkloadInstance":            "db.Delete(&workloadInstance)",
		"KubernetesRuntimeDefinition": "db.Delete(&kubernetesRuntimeDefinition)",
		"NotAnApiObject":              "db.Delete(&notAnApiObject)",
	} {
		t.Run(typeName, func(t *testing.T) {
			assert.Equal(t, want, fmt.Sprintf("%#v", deleteObject(generator, typeName)))
		})
	}
}
