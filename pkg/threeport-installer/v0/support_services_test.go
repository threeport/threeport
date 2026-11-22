package v0

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/mod/modfile"
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

// TestSupportServicesOperatorManifestIncludesStorage covers the storage definition.
func TestSupportServicesOperatorManifestIncludesStorage(t *testing.T) {
	// the operator manifest registers the storage component
	objects, err := supportServicesOperatorObjects(ControlPlaneNamespace)
	require.NoError(t, err)
	found := false
	for _, item := range objects {
		if item.GetKind() == "CustomResourceDefinition" && item.GetName() == "storagecomponents.platform.addons.nukleros.io" {
			found = true
		}
	}
	require.True(t, found)
}

// TestSupportServicesOperatorModuleMatchesImage checks go.mod against the image tag.
func TestSupportServicesOperatorModuleMatchesImage(t *testing.T) {
	// read the module requirement
	raw, err := os.ReadFile("../../../go.mod")
	require.NoError(t, err)
	file, err := modfile.Parse("go.mod", raw, nil)
	require.NoError(t, err)
	const modulePath = "github.com/nukleros/support-services-operator"
	var version string
	for _, req := range file.Require {
		if req.Mod.Path == modulePath {
			version = req.Mod.Version
		}
	}

	// the image tag is that same version
	tag := SupportServicesOperatorImage[strings.LastIndex(SupportServicesOperatorImage, ":")+1:]
	require.Equal(t, tag, version)
}

// TestSupportServicesOperatorUsesRuntimeNamespace covers one operator in the runtime namespace.
func TestSupportServicesOperatorUsesRuntimeNamespace(t *testing.T) {
	for _, namespace := range []string{RuntimeNamespace} {
		objects, err := supportServicesOperatorObjects(namespace)
		require.NoError(t, err)

		// one deployment, and it lives in the runtime namespace
		deployments := 0
		for _, item := range objects {
			if item.GetNamespace() == supportServicesOperatorNamespace {
				t.Errorf("object %s %s stayed in %s", item.GetKind(), item.GetName(), supportServicesOperatorNamespace)
			}
			if item.GetKind() != "Deployment" {
				continue
			}
			deployments++
			require.Equal(t, namespace, item.GetNamespace())
			require.Equal(t, SupportServicesOperatorDeployName, item.GetName())
		}
		require.Equal(t, 1, deployments)
	}
}
