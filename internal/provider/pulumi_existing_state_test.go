package provider

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// TestAFreshStackIsNotTakenForExistingState covers #570 against a real Pulumi
// backend: the stack UpsertStack has just created already has a state file
// that is not empty, which is what made every new install report that it had
// found existing state and refresh a stack holding nothing. A stack that
// does hold a resource is still reconciled.
func TestAFreshStackIsNotTakenForExistingState(t *testing.T) {
	requirePulumiCLI(t)
	noop := func(*pulumi.Context) error { return nil }

	fresh := NewPulumiWorkspace("fresh-instance", "freshproj", WithStateDirRoot(t.TempDir()))
	_, refreshed, err := fresh.setupStack(noop)
	require.NoError(t, err)
	assert.False(t, refreshed, "a stack just created holds nothing to reconcile")

	stateFile, err := fresh.GetStateFilePath()
	require.NoError(t, err)
	info, err := os.Stat(stateFile)
	require.NoError(t, err)
	assert.Positive(t, info.Size(), "the file a new stack is given is not empty, so its size says nothing")

	provisioned := NewPulumiWorkspace("held-instance", "heldproj", WithStateDirRoot(t.TempDir()))
	require.NoError(t, provisioned.SetStackState(jsonPtr(checkpointState("heldproj", "held-instance", "held"))))
	_, refreshed, err = provisioned.setupStack(noop)
	require.NoError(t, err)
	assert.True(t, refreshed, "a stack holding a resource is reconciled before it is deployed")
}
