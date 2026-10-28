package kubernetesworkload

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

func jsonDef(content string) *datatypes.JSON {
	definition := datatypes.JSON(content)
	return &definition
}

func storedWRD(id uint, content string) v0.KubernetesWorkloadResourceDefinition {
	return v0.KubernetesWorkloadResourceDefinition{
		Common:         v0.Common{ID: util.Ptr(id)},
		JSONDefinition: jsonDef(content),
	}
}

func storedWRI(id uint, content string) v0.KubernetesWorkloadResourceInstance {
	return v0.KubernetesWorkloadResourceInstance{
		Common:         v0.Common{ID: util.Ptr(id)},
		JSONDefinition: jsonDef(content),
		Reconciled:     util.Ptr(true),
	}
}

func renderedWRI(content string) v0.KubernetesWorkloadResourceInstance {
	return v0.KubernetesWorkloadResourceInstance{JSONDefinition: jsonDef(content)}
}

const (
	deploymentV1  = `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"web"},"spec":{"replicas":1}}`
	deploymentV2  = `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"web"},"spec":{"replicas":3}}`
	serviceWeb    = `{"apiVersion":"v1","kind":"Service","metadata":{"name":"web"}}`
	configMapWeb  = `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"web"},"data":{"a":"b"}}`
	namespaceTeam = `{"apiVersion":"v1","kind":"Namespace","metadata":{"name":"team"}}`
)

// TestPlanResourceDefinitions covers how a definition's stored resources are
// brought in line with its YAML document: matched by resource, not position.
func TestPlanResourceDefinitions(t *testing.T) {
	existing := []v0.KubernetesWorkloadResourceDefinition{
		storedWRD(10, deploymentV1),
		storedWRD(11, serviceWeb),
	}

	t.Run("the same resources stored with another key order are no change", func(t *testing.T) {
		// jsonb keeps neither key order nor spacing, so comparing bytes would
		// rewrite every resource of every instance on any edit to the definition
		plan, err := planResourceDefinitions(1, existing, [][]byte{
			[]byte(`{"kind":"Deployment","spec":{"replicas":1},"metadata":{"name":"web"},"apiVersion":"apps/v1"}`),
			[]byte(serviceWeb),
		})
		require.NoError(t, err)
		assert.False(t, plan.changed())
		require.Len(t, plan.desired, 2)
		assert.Equal(t, uint(10), *plan.desired[0].ID)
	})

	t.Run("a changed manifest updates its resource definition in place", func(t *testing.T) {
		plan, err := planResourceDefinitions(1, existing, [][]byte{[]byte(deploymentV2), []byte(serviceWeb)})
		require.NoError(t, err)
		require.Len(t, plan.update, 1)
		assert.Equal(t, uint(10), *plan.update[0].ID, "the stored resource keeps its ID")
		assert.JSONEq(t, deploymentV2, string(*plan.update[0].JSONDefinition))
		assert.Empty(t, plan.create)
		assert.Empty(t, plan.remove)
	})

	t.Run("a new manifest is created and a dropped one removed", func(t *testing.T) {
		plan, err := planResourceDefinitions(1, existing, [][]byte{[]byte(deploymentV1), []byte(configMapWeb)})
		require.NoError(t, err)
		require.Len(t, plan.create, 1)
		assert.JSONEq(t, configMapWeb, string(*plan.create[0].JSONDefinition))
		assert.Equal(t, uint(1), *plan.create[0].KubernetesWorkloadDefinitionID)
		require.Len(t, plan.remove, 1)
		assert.Equal(t, uint(11), *plan.remove[0].ID)
	})

	t.Run("a resource moved to another API version is the same resource", func(t *testing.T) {
		moved := `{"apiVersion":"apps/v2","kind":"Deployment","metadata":{"name":"web"},"spec":{"replicas":1}}`
		plan, err := planResourceDefinitions(1, existing, [][]byte{[]byte(moved), []byte(serviceWeb)})
		require.NoError(t, err)
		require.Len(t, plan.update, 1)
		assert.Equal(t, uint(10), *plan.update[0].ID)
		assert.Empty(t, plan.create)
		assert.Empty(t, plan.remove)
	})

	t.Run("an empty YAML document between resources is not a resource", func(t *testing.T) {
		plan, err := planResourceDefinitions(1, existing, [][]byte{[]byte(deploymentV1), []byte("null"), []byte(serviceWeb)})
		require.NoError(t, err)
		assert.False(t, plan.changed())
	})

	t.Run("a resource declared twice is refused", func(t *testing.T) {
		_, err := planResourceDefinitions(1, existing, [][]byte{[]byte(deploymentV1), []byte(deploymentV2)})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "apps/Deployment//web")
	})
}

// TestPlanResourceInstances covers which of an instance's resources a
// definition change rewrites.
func TestPlanResourceInstances(t *testing.T) {
	t.Run("a resource the change touched is replaced and left unreconciled", func(t *testing.T) {
		plan, err := planResourceInstances(
			5,
			[]v0.KubernetesWorkloadResourceInstance{storedWRI(20, deploymentV1), storedWRI(21, serviceWeb)},
			[]v0.KubernetesWorkloadResourceInstance{renderedWRI(deploymentV1), renderedWRI(serviceWeb)},
			[]v0.KubernetesWorkloadResourceInstance{renderedWRI(deploymentV2), renderedWRI(serviceWeb)},
		)
		require.NoError(t, err)
		require.Len(t, plan.update, 1)
		assert.Equal(t, uint(20), *plan.update[0].ID)
		assert.JSONEq(t, deploymentV2, string(*plan.update[0].JSONDefinition))
		assert.False(t, *plan.update[0].Reconciled)
		assert.Empty(t, plan.create)
		assert.Empty(t, plan.remove)
	})

	t.Run("a resource the change did not touch keeps another controller's edit", func(t *testing.T) {
		// the gateway controller edits resources of workloads it serves;
		// rewriting every resource from the definition would undo that
		editedService := `{"apiVersion":"v1","kind":"Service","metadata":{"name":"web"},"spec":{"ports":[{"port":8443}]}}`
		plan, err := planResourceInstances(
			5,
			[]v0.KubernetesWorkloadResourceInstance{storedWRI(20, deploymentV1), storedWRI(21, editedService)},
			[]v0.KubernetesWorkloadResourceInstance{renderedWRI(deploymentV1), renderedWRI(serviceWeb)},
			[]v0.KubernetesWorkloadResourceInstance{renderedWRI(deploymentV2), renderedWRI(serviceWeb)},
		)
		require.NoError(t, err)
		require.Len(t, plan.update, 1)
		assert.Equal(t, uint(20), *plan.update[0].ID)
	})

	t.Run("an added resource is created for the instance", func(t *testing.T) {
		plan, err := planResourceInstances(
			5,
			[]v0.KubernetesWorkloadResourceInstance{storedWRI(20, deploymentV1)},
			[]v0.KubernetesWorkloadResourceInstance{renderedWRI(deploymentV1)},
			[]v0.KubernetesWorkloadResourceInstance{renderedWRI(deploymentV1), renderedWRI(configMapWeb)},
		)
		require.NoError(t, err)
		require.Len(t, plan.create, 1)
		assert.Equal(t, uint(5), *plan.create[0].KubernetesWorkloadInstanceID)
		assert.JSONEq(t, configMapWeb, string(*plan.create[0].JSONDefinition))
		assert.False(t, *plan.create[0].Reconciled)
	})

	t.Run("a dropped resource is scheduled for deletion", func(t *testing.T) {
		plan, err := planResourceInstances(
			5,
			[]v0.KubernetesWorkloadResourceInstance{storedWRI(20, deploymentV1), storedWRI(21, serviceWeb)},
			[]v0.KubernetesWorkloadResourceInstance{renderedWRI(deploymentV1), renderedWRI(serviceWeb)},
			[]v0.KubernetesWorkloadResourceInstance{renderedWRI(deploymentV1)},
		)
		require.NoError(t, err)
		require.Len(t, plan.remove, 1)
		assert.Equal(t, uint(21), *plan.remove[0].ID)
		assert.NotNil(t, plan.remove[0].ScheduledForDeletion)
		assert.False(t, *plan.remove[0].Reconciled)
	})

	t.Run("resource instances from elsewhere are never touched", func(t *testing.T) {
		// a secret's or a gateway's resources match nothing the definition renders
		secretResource := `{"apiVersion":"external-secrets.io/v1beta1","kind":"ExternalSecret","metadata":{"name":"db"}}`
		plan, err := planResourceInstances(
			5,
			[]v0.KubernetesWorkloadResourceInstance{storedWRI(20, deploymentV1), storedWRI(30, secretResource)},
			[]v0.KubernetesWorkloadResourceInstance{renderedWRI(deploymentV1)},
			[]v0.KubernetesWorkloadResourceInstance{renderedWRI(deploymentV2)},
		)
		require.NoError(t, err)
		require.Len(t, plan.update, 1)
		assert.Equal(t, uint(20), *plan.update[0].ID)
		assert.Empty(t, plan.remove)
	})

	t.Run("a namespace is never deleted by a definition change", func(t *testing.T) {
		// deleting it would delete everything in it, resources from elsewhere included
		plan, err := planResourceInstances(
			5,
			[]v0.KubernetesWorkloadResourceInstance{storedWRI(19, namespaceTeam), storedWRI(20, deploymentV1)},
			[]v0.KubernetesWorkloadResourceInstance{renderedWRI(namespaceTeam), renderedWRI(deploymentV1)},
			[]v0.KubernetesWorkloadResourceInstance{renderedWRI(`{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"ClusterRole","metadata":{"name":"web"}}`)},
		)
		require.NoError(t, err)
		require.Len(t, plan.remove, 1)
		assert.Equal(t, uint(20), *plan.remove[0].ID)
	})

	t.Run("a retry after the writes went through has nothing left to do", func(t *testing.T) {
		plan, err := planResourceInstances(
			5,
			[]v0.KubernetesWorkloadResourceInstance{storedWRI(20, deploymentV2), storedWRI(22, configMapWeb)},
			[]v0.KubernetesWorkloadResourceInstance{renderedWRI(deploymentV1), renderedWRI(serviceWeb)},
			[]v0.KubernetesWorkloadResourceInstance{renderedWRI(deploymentV2), renderedWRI(configMapWeb)},
		)
		require.NoError(t, err)
		assert.False(t, plan.changed())
	})

	t.Run("a resource already on its way out is neither updated nor scheduled again", func(t *testing.T) {
		leaving := storedWRI(21, serviceWeb)
		leaving.ScheduledForDeletion = util.Ptr(time.Now())

		plan, err := planResourceInstances(
			5,
			[]v0.KubernetesWorkloadResourceInstance{storedWRI(20, deploymentV1), leaving},
			[]v0.KubernetesWorkloadResourceInstance{renderedWRI(deploymentV1), renderedWRI(serviceWeb)},
			[]v0.KubernetesWorkloadResourceInstance{renderedWRI(deploymentV1)},
		)
		require.NoError(t, err)
		assert.False(t, plan.changed())

		// and declared once more, it is created afresh beside the one leaving
		plan, err = planResourceInstances(
			5,
			[]v0.KubernetesWorkloadResourceInstance{storedWRI(20, deploymentV1), leaving},
			[]v0.KubernetesWorkloadResourceInstance{renderedWRI(deploymentV1)},
			[]v0.KubernetesWorkloadResourceInstance{renderedWRI(deploymentV1), renderedWRI(serviceWeb)},
		)
		require.NoError(t, err)
		require.Len(t, plan.create, 1)
		assert.Empty(t, plan.update)
	})
}

// TestManagedNamespace covers which namespace a set of resources runs in.
func TestManagedNamespace(t *testing.T) {
	namespace, err := managedNamespace([][]byte{[]byte(deploymentV1), []byte(namespaceTeam)})
	require.NoError(t, err)
	assert.Equal(t, "team", namespace)

	namespace, err = managedNamespace([][]byte{[]byte("null"), []byte(deploymentV1)})
	require.NoError(t, err)
	assert.Equal(t, "", namespace, "with no Namespace declared threeport manages one")
}
