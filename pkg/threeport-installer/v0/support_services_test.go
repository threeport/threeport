package v0

import (
	"testing"

	"github.com/stretchr/testify/require"
	kubeerr "k8s.io/apimachinery/pkg/api/errors"
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
	require.Equal(t, "aws", SupportServicesCloud("eks"))
	require.Equal(t, "google", SupportServicesCloud("gke"))
	require.Equal(t, "local", SupportServicesCloud("kind"))
	require.Equal(t, "aws", supportServicesObject("aws").Object["spec"].(map[string]interface{})["cloud"])
}

// TestAddCainjectorPatchVerb adds patch to a webhook rule that only allows update.
func TestAddCainjectorPatchVerb(t *testing.T) {
	// start from the verbs the operator installs
	role := map[string]interface{}{
		"rules": []interface{}{
			map[string]interface{}{
				"apiGroups": []interface{}{"admissionregistration.k8s.io"},
				"resources": []interface{}{"validatingwebhookconfigurations", "mutatingwebhookconfigurations"},
				"verbs":     []interface{}{"get", "list", "update"},
			},
		},
	}

	// add patch once, then leave the role unchanged
	changed, err := addCainjectorPatchVerb(role)
	require.NoError(t, err)
	require.True(t, changed)
	rule := role["rules"].([]interface{})[0].(map[string]interface{})
	require.Contains(t, stringList(rule["verbs"]), "patch")
	changed, err = addCainjectorPatchVerb(role)
	require.NoError(t, err)
	require.False(t, changed)
}
