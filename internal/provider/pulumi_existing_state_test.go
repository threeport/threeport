package provider

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDeploymentHoldsState(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		deployment string
		holds      bool
	}{
		{"nothing exported", ``, false},
		{"a freshly created stack", `{"manifest":{"time":"0001-01-01T00:00:00Z","magic":"","version":""}}`, false},
		{"a destroyed stack", `{"manifest":{},"resources":[]}`, false},
		{"a provisioned stack", `{"manifest":{},"resources":[{"urn":"urn:pulumi:s::p::pulumi:pulumi:Stack::p-s","type":"pulumi:pulumi:Stack"}]}`, true},
		{
			// what an interrupted run leaves: an operation in flight, nothing
			// recorded as created yet
			"an interrupted run", `{"manifest":{},"pending_operations":[{"resource":{"urn":"urn:pulumi:s::p::gcp:container/cluster:Cluster::c"},"type":"creating"}]}`, true,
		},
		{"unreadable", `not json`, true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			assert.Equal(t, testCase.holds, deploymentHoldsState(json.RawMessage(testCase.deployment)))
		})
	}
}
