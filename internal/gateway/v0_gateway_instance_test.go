package gateway

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// v0GatewayInstanceUpdated marks its dependent objects unreconciled, which
// makes the kubernetes workload controller re-apply their manifests to a live
// cluster. These cover the decision of which resources that is worth doing for,
// because doing it for a resource that has not changed re-applies an identical
// manifest for nothing.

// resourceInstance builds a stored resource instance of the given kind.
func resourceInstance(t *testing.T, id uint, kind, host string) v0.KubernetesWorkloadResourceInstance {
	t.Helper()
	definition := datatypes.JSON(fmt.Sprintf(
		`{"kind":%q,"metadata":{"name":"a-gateway"},"spec":{"hosts":[%q]}}`, kind, host,
	))

	return v0.KubernetesWorkloadResourceInstance{
		Common:         v0.Common{ID: util.Ptr(id)},
		JSONDefinition: &definition,
		Reconciled:     util.Ptr(true),
	}
}

func TestChangedGatewayResourceInstances_NothingChanged(t *testing.T) {
	existing := []v0.KubernetesWorkloadResourceInstance{
		resourceInstance(t, 1, "VirtualService", "example.com"),
		resourceInstance(t, 2, "Gateway", "example.com"),
	}
	updated := []v0.KubernetesWorkloadResourceInstance{
		resourceInstance(t, 1, "VirtualService", "example.com"),
		resourceInstance(t, 2, "Gateway", "example.com"),
	}

	changed, err := changedGatewayResourceInstances(
		[]string{"VirtualService", "Gateway"}, &existing, &updated,
	)
	require.NoError(t, err)
	assert.Empty(t, changed, "an unchanged gateway should leave its dependents alone")
}

// a definition read back from the database is not guaranteed to return
// byte-identical to what went in, so a resource that differs only in
// serialization must still count as unchanged - otherwise the cascade fires on
// every invocation and this check does nothing
func TestChangedGatewayResourceInstances_ReserializedIsUnchanged(t *testing.T) {
	existing := []v0.KubernetesWorkloadResourceInstance{
		resourceInstance(t, 1, "VirtualService", "example.com"),
	}
	reordered := datatypes.JSON(
		"{\n  \"spec\": {\"hosts\": [\"example.com\"]},\n  \"metadata\": {\"name\": \"a-gateway\"},\n  \"kind\": \"VirtualService\"\n}",
	)
	updated := []v0.KubernetesWorkloadResourceInstance{
		{Common: v0.Common{ID: util.Ptr(uint(1))}, JSONDefinition: &reordered},
	}

	changed, err := changedGatewayResourceInstances(
		[]string{"VirtualService"}, &existing, &updated,
	)
	require.NoError(t, err)
	assert.Empty(t, changed)
}

func TestChangedGatewayResourceInstances_OnlyTheChangedOne(t *testing.T) {
	existing := []v0.KubernetesWorkloadResourceInstance{
		resourceInstance(t, 1, "VirtualService", "example.com"),
		resourceInstance(t, 2, "Gateway", "example.com"),
	}
	updated := []v0.KubernetesWorkloadResourceInstance{
		resourceInstance(t, 1, "VirtualService", "moved.example.com"),
		resourceInstance(t, 2, "Gateway", "example.com"),
	}

	changed, err := changedGatewayResourceInstances(
		[]string{"VirtualService", "Gateway"}, &existing, &updated,
	)
	require.NoError(t, err)
	require.Len(t, changed, 1, "only the virtual service changed")

	assert.Equal(t, uint(1), *changed[0].ID)

	// it carries the new definition and is marked unreconciled, which is what
	// makes the workload controller apply it
	require.NotNil(t, changed[0].Reconciled)
	assert.False(t, *changed[0].Reconciled)
	equal, err := util.JSONDefinitionsEqual(changed[0].JSONDefinition, updated[0].JSONDefinition)
	require.NoError(t, err)
	assert.True(t, equal)
}

func TestChangedGatewayResourceInstances_MissingResourceIsAnError(t *testing.T) {
	existing := []v0.KubernetesWorkloadResourceInstance{
		resourceInstance(t, 1, "VirtualService", "example.com"),
	}
	updated := []v0.KubernetesWorkloadResourceInstance{
		resourceInstance(t, 1, "VirtualService", "example.com"),
	}

	_, err := changedGatewayResourceInstances(
		[]string{"VirtualService", "Gateway"}, &existing, &updated,
	)
	require.Error(t, err)
}
