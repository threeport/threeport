package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"

	config_v0 "github.com/threeport/threeport/pkg/config/v0"
)

// TestOutputGetv0KubernetesRuntimeInstancesCmdPrintsAPIEndpoint checks the instance table.
// A stored endpoint is printed, and an unset endpoint leaves that cell empty.
func TestOutputGetv0KubernetesRuntimeInstancesCmdPrintsAPIEndpoint(t *testing.T) {
	endpoint := "https://10.0.0.1"
	name := "dev-01"
	unsetName := "unset-01"
	location := "us-central1"
	defaultRuntime := false
	definition := "dev-01"
	instances := []config_v0.KubernetesRuntimeInstanceConfig{
		{
			KubernetesRuntimeInstance: config_v0.KubernetesRuntimeInstanceValues{
				Name:           &name,
				Location:       &location,
				DefaultRuntime: &defaultRuntime,
				APIEndpoint:    &endpoint,
				KubernetesRuntimeDefinition: &config_v0.KubernetesRuntimeDefinitionValues{
					Name: &definition,
				},
			},
		},
		{
			KubernetesRuntimeInstance: config_v0.KubernetesRuntimeInstanceValues{
				Name:           &unsetName,
				Location:       &location,
				DefaultRuntime: &defaultRuntime,
				KubernetesRuntimeDefinition: &config_v0.KubernetesRuntimeDefinitionValues{
					Name: &definition,
				},
			},
		},
	}

	// print the instance table
	output := captureStdout(t, func() error {
		return outputGetv0KubernetesRuntimeInstancesCmd(&instances)
	})

	// check the endpoint column and the stored address
	assert.Contains(t, output, "API ENDPOINT")
	assert.Contains(t, output, endpoint)
	assert.Contains(t, output, unsetName)
}
