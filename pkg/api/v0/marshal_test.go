package v0

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	util "github.com/threeport/threeport/pkg/util/v0"
)

// The API types carry no json:",omitempty" tag. Absent values are dropped by
// the OmitZeroStructFields option that util.MarshalObject passes at the
// marshal call site instead. These tests pin that policy down on a real API
// object, since a regression here is silent: the payload still marshals, it
// just starts carrying nulls the api server rejects.

// TestMarshalObject_PartialPatchOmitsUntouchedFields is the regression the
// removed struct tag existed to prevent. A partial PATCH body sets one field
// and leaves the rest nil. If a nil pointer serialized as JSON null, the api
// server's null-on-required guard in PayloadCheck() would reject the request
// even though the caller never meant to touch that field.
func TestMarshalObject_PartialPatchOmitsUntouchedFields(t *testing.T) {
	yamlDoc := "apiVersion: v1"
	payload := KubernetesWorkloadDefinition{
		YAMLDocument: &yamlDoc,
	}

	marshaled, err := util.MarshalObject(payload)
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(marshaled, &got))

	assert.Equal(t, yamlDoc, got["YAMLDocument"], "the field the caller set must survive")

	for key, value := range got {
		assert.NotNil(t, value, "field %q serialized as JSON null; PayloadCheck() would reject this payload", key)
	}
	assert.NotContains(t, got, "Name", "untouched required field must be omitted, not null")
	assert.NotContains(t, got, "ID", "untouched field must be omitted, not null")
}

// TestMarshalObject_EmptyStringIsSent covers a deliberate behavior change from
// the old json:",omitempty" tag. omitempty dropped a *string pointing at "",
// silently discarding a caller's attempt to clear a string field.
// OmitZeroStructFields keeps it, because the pointer itself is non-nil.
func TestMarshalObject_EmptyStringIsSent(t *testing.T) {
	empty := ""
	payload := KubernetesWorkloadDefinition{}
	payload.Name = &empty

	marshaled, err := util.MarshalObject(payload)
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(marshaled, &got))

	require.Contains(t, got, "Name", "an explicitly emptied string must reach the api server")
	assert.Equal(t, "", got["Name"])
}

// TestMarshalObject_EmptyAssociationSliceIsSent covers the other behavior
// change. omitempty dropped an empty non-nil slice; OmitZeroStructFields drops
// only a nil one, so an initialized-but-empty association serializes as [].
func TestMarshalObject_EmptyAssociationSliceIsSent(t *testing.T) {
	payload := KubernetesWorkloadDefinition{
		KubernetesWorkloadInstances: []*KubernetesWorkloadInstance{},
	}

	marshaled, err := util.MarshalObject(payload)
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(marshaled, &got))

	require.Contains(t, got, "KubernetesWorkloadInstances")
	assert.Empty(t, got["KubernetesWorkloadInstances"])

	nilSlice := KubernetesWorkloadDefinition{}
	marshaledNil, err := util.MarshalObject(nilSlice)
	require.NoError(t, err)

	// a fresh map: json.Unmarshal merges into an existing one rather than
	// replacing it, which would leave the key above in place and pass falsely.
	var gotNil map[string]any
	require.NoError(t, json.Unmarshal(marshaledNil, &gotNil))
	assert.NotContains(t, gotNil, "KubernetesWorkloadInstances", "a nil association slice is still omitted")
}
