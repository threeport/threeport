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

// configuredInstance builds a resource instance as the gateway configures it.
// Nothing here sets metadata.namespace, matching what this package generates.
func configuredInstance(t *testing.T, id uint, kind, host string) v0.KubernetesWorkloadResourceInstance {
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

// storedInstance builds a resource instance as it comes back from the API,
// carrying the namespace the kubernetes workload reconciler wrote into it and
// persisted. Comparing against anything else is comparing against a state that
// does not occur.
func storedInstance(t *testing.T, id uint, kind, host string) v0.KubernetesWorkloadResourceInstance {
	t.Helper()
	definition := datatypes.JSON(fmt.Sprintf(
		`{"kind":%q,"metadata":{"name":"a-gateway","namespace":"a-workload-xk39dl2p01"},"spec":{"hosts":[%q]}}`,
		kind, host,
	))

	return v0.KubernetesWorkloadResourceInstance{
		Common:         v0.Common{ID: util.Ptr(id)},
		JSONDefinition: &definition,
		Reconciled:     util.Ptr(true),
	}
}

func TestChangedGatewayResourceInstances_NothingChanged(t *testing.T) {
	existing := []v0.KubernetesWorkloadResourceInstance{
		storedInstance(t, 1, "VirtualService", "example.com"),
		storedInstance(t, 2, "Gateway", "example.com"),
	}
	updated := []v0.KubernetesWorkloadResourceInstance{
		configuredInstance(t, 1, "VirtualService", "example.com"),
		configuredInstance(t, 2, "Gateway", "example.com"),
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
		storedInstance(t, 1, "VirtualService", "example.com"),
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
		storedInstance(t, 1, "VirtualService", "example.com"),
		storedInstance(t, 2, "Gateway", "example.com"),
	}
	updated := []v0.KubernetesWorkloadResourceInstance{
		configuredInstance(t, 1, "VirtualService", "moved.example.com"),
		configuredInstance(t, 2, "Gateway", "example.com"),
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
		storedInstance(t, 1, "VirtualService", "example.com"),
	}
	updated := []v0.KubernetesWorkloadResourceInstance{
		configuredInstance(t, 1, "VirtualService", "example.com"),
	}

	_, err := changedGatewayResourceInstances(
		[]string{"VirtualService", "Gateway"}, &existing, &updated,
	)
	require.Error(t, err)
}

// TestChangedGatewayResourceInstances_StoredNamespaceIsNotAChange is the case
// that decides whether any of this works.
//
// Nothing in the gateway configuration sets metadata.namespace. The kubernetes
// workload reconciler writes one into every namespaced resource and persists
// it, so the stored copy always carries a namespace the configured copy never
// has. Treating that as a difference marks every resource unreconciled on every
// invocation, and the cascade this function is meant to stop fires exactly as
// before.
func TestChangedGatewayResourceInstances_StoredNamespaceIsNotAChange(t *testing.T) {
	existing := []v0.KubernetesWorkloadResourceInstance{
		storedInstance(t, 1, "VirtualService", "example.com"),
		storedInstance(t, 2, "Issuer", "example.com"),
		storedInstance(t, 3, "Certificate", "example.com"),
	}
	updated := []v0.KubernetesWorkloadResourceInstance{
		configuredInstance(t, 1, "VirtualService", "example.com"),
		configuredInstance(t, 2, "Issuer", "example.com"),
		configuredInstance(t, 3, "Certificate", "example.com"),
	}

	changed, err := changedGatewayResourceInstances(
		[]string{"VirtualService", "Issuer", "Certificate"}, &existing, &updated,
	)
	require.NoError(t, err)
	assert.Empty(t, changed, "a namespace only the stored copy carries is not a gateway change")
}

// the namespace being set aside must not hide a real difference elsewhere in
// the same resource
func TestChangedGatewayResourceInstances_NamespaceAsideStillSeesRealChanges(t *testing.T) {
	existing := []v0.KubernetesWorkloadResourceInstance{
		storedInstance(t, 1, "VirtualService", "example.com"),
	}
	updated := []v0.KubernetesWorkloadResourceInstance{
		configuredInstance(t, 1, "VirtualService", "moved.example.com"),
	}

	changed, err := changedGatewayResourceInstances(
		[]string{"VirtualService"}, &existing, &updated,
	)
	require.NoError(t, err)
	require.Len(t, changed, 1)
	assert.Equal(t, uint(1), *changed[0].ID)
}
