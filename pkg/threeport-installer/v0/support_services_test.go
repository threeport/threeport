package v0

import (
	"testing"

	"github.com/stretchr/testify/require"
	kubeerr "k8s.io/apimachinery/pkg/api/errors"

	api "github.com/threeport/threeport/pkg/api/v0"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stesting "k8s.io/client-go/testing"
)

// selector returns a deployment selector for the app label.
func selector(app string) map[string]interface{} {
	// build the matchLabels map
	return map[string]interface{}{
		"matchLabels": map[string]interface{}{"app": app},
	}
}

// deploymentWithSelector returns a deployment with the app selector.
func deploymentWithSelector(namespace, name, app string) *unstructured.Unstructured {
	// set the selector and a resource version
	object := deployment(namespace, name, "nginx:1")
	spec := object.Object["spec"].(map[string]interface{})
	spec["selector"] = selector(app)
	object.SetResourceVersion("1")
	return object
}

// TestInstallDeploymentWaitsForSelectorReplacement covers a selector change.
func TestInstallDeploymentWaitsForSelectorReplacement(t *testing.T) {
	// keep the old deployment readable after delete
	existing := deploymentWithSelector("ingress", "nginx", "old")
	desired := deploymentWithSelector("ingress", "nginx", "new")
	kubeClient := testClient(existing)
	deleted := false
	getsAfterDelete := 0
	kubeClient.PrependReactor("delete", "deployments", func(action k8stesting.Action) (bool, runtime.Object, error) {
		deleted = true
		return true, nil, nil
	})
	kubeClient.PrependReactor("get", "deployments", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if !deleted {
			return false, nil, nil
		}
		getsAfterDelete++
		if getsAfterDelete < 3 {
			return false, nil, nil
		}
		return true, nil, kubeerr.NewNotFound(schema.GroupResource{Group: "apps", Resource: "deployments"}, "nginx")
	})
	kubeClient.PrependReactor("update", "deployments", func(action k8stesting.Action) (bool, runtime.Object, error) {
		t.Errorf("updated deployment while the old one was still present")
		return true, nil, kubeerr.NewBadRequest("immutable selector")
	})
	created := false
	kubeClient.PrependReactor("create", "deployments", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if getsAfterDelete < 3 {
			t.Errorf("created deployment while the old one was still present")
		}
		created = true
		return true, desired, nil
	})
	deploymentGoneWaitSeconds = 0
	t.Cleanup(func() {
		deploymentGoneWaitSeconds = 1
	})

	// create the replacement only after the old deployment is gone
	err := installDeployment(desired, kubeClient, testMapper())
	require.NoError(t, err)
	require.True(t, created)
}

// TestInstallDeploymentUpdatesMatchingSelector covers an unchanged selector.
func TestInstallDeploymentUpdatesMatchingSelector(t *testing.T) {
	// install a deployment whose selector already matches
	existing := deploymentWithSelector("ingress", "nginx", "nginx")
	desired := deploymentWithSelector("ingress", "nginx", "nginx")
	kubeClient := testClient(existing)
	deleted := false
	kubeClient.PrependReactor("delete", "deployments", func(action k8stesting.Action) (bool, runtime.Object, error) {
		deleted = true
		return false, nil, nil
	})

	// update it without deleting
	err := installDeployment(desired, kubeClient, testMapper())
	require.NoError(t, err)
	require.False(t, deleted)
}

func TestSupportServicesCloud(t *testing.T) {
	// eks and gke map onto the custom resource enum
	require.Equal(t, SupportServicesCloudAWS, SupportServicesCloud(api.KubernetesRuntimeInfraProviderEKS))
	require.Equal(t, SupportServicesCloudGoogle, SupportServicesCloud(api.KubernetesRuntimeInfraProviderGKE))
	require.Equal(t, SupportServicesCloudLocal, SupportServicesCloud(api.KubernetesRuntimeInfraProviderKind))
	require.Equal(t, SupportServicesCloudAWS, supportServicesObject(SupportServicesCloudAWS).Object["spec"].(map[string]interface{})["cloud"])
}

// TestSetManagerMemoryLimitCapsTheManager checks the manager memory limit and keeps its other resources.
func TestSetManagerMemoryLimitCapsTheManager(t *testing.T) {
	object := map[string]interface{}{
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{
							"name": "manager",
							"resources": map[string]interface{}{
								"requests": map[string]interface{}{
									"cpu":    "10m",
									"memory": "64Mi",
								},
								"limits": map[string]interface{}{
									"cpu":    "500m",
									"memory": "128Mi",
								},
							},
						},
						map[string]interface{}{"name": "kube-rbac-proxy"},
					},
				},
			},
		},
	}

	// cap the manager
	setManagerMemoryLimit(object)

	// read the containers back
	containers, found, err := unstructured.NestedSlice(object, "spec", "template", "spec", "containers")
	require.NoError(t, err)
	require.True(t, found)
	resources := containers[0].(map[string]interface{})["resources"].(map[string]interface{})
	limits := resources["limits"].(map[string]interface{})
	requests := resources["requests"].(map[string]interface{})

	// keep the memory limit and the resources that were already set
	require.Equal(t, supportServicesOperatorMemoryLimit, limits["memory"])
	require.Equal(t, "500m", limits["cpu"])
	require.Equal(t, "10m", requests["cpu"])
	require.Equal(t, "64Mi", requests["memory"])
	_, otherHasResources := containers[1].(map[string]interface{})["resources"]
	require.False(t, otherHasResources)
}
