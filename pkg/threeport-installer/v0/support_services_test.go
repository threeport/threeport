package v0

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/mod/modfile"
	kubeerr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	api "github.com/threeport/threeport/pkg/api/v0"
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

// TestMayUpdateSupportServicesOperator covers which control plane may install the operator.
func TestMayUpdateSupportServicesOperator(t *testing.T) {
	// a missing binding is a claim, and a named one matches only that name
	may, err := MayUpdateSupportServicesOperator("primary", "", false)
	require.NoError(t, err)
	require.True(t, may)
	may, err = MayUpdateSupportServicesOperator("primary", "primary", true)
	require.NoError(t, err)
	require.True(t, may)
	may, err = MayUpdateSupportServicesOperator("primary", "other", true)
	require.NoError(t, err)
	require.False(t, may)
	may, err = MayUpdateSupportServicesOperator("primary", "", true)
	require.NoError(t, err)
	require.False(t, may)
	_, err = MayUpdateSupportServicesOperator("", "", false)
	require.Error(t, err)
}

// TestSupportServicesOperatorOwnerReadsTheBinding covers the name stored on the manager binding.
func TestSupportServicesOperatorOwnerReadsTheBinding(t *testing.T) {
	// a labeled binding, an unlabeled one, and none
	labeled := managerBinding("primary", "kept")
	unlabeled := managerBinding("", "kept")
	for _, tc := range []struct {
		name  string
		have  []runtime.Object
		owner string
		found bool
	}{
		{name: "labeled", have: []runtime.Object{labeled}, owner: "primary", found: true},
		{name: "unlabeled", have: []runtime.Object{unlabeled}, owner: "", found: true},
		{name: "missing", have: nil, owner: "", found: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			owner, found, err := SupportServicesOperatorOwner(testClient(tc.have...), testMapper())
			require.NoError(t, err)
			require.Equal(t, tc.owner, owner)
			require.Equal(t, tc.found, found)
		})
	}
}

// TestInstallSupportServicesOperatorLeavesAForeignBinding covers a binding this control plane does not own.
func TestInstallSupportServicesOperatorLeavesAForeignBinding(t *testing.T) {
	// install against a binding named for another control plane, and one with no name
	for _, owner := range []string{"other", ""} {
		binding := managerBinding(owner, "kept")
		kubeClient := testClient(binding)
		err := InstallThreeportSupportServicesOperator(
			kubeClient,
			testMapper(),
			SupportServicesCloudLocal,
			RuntimeNamespace,
			"primary",
		)
		require.NoError(t, err)
		got, found, err := SupportServicesOperatorOwner(kubeClient, testMapper())
		require.NoError(t, err)
		require.True(t, found)
		require.Equal(t, owner, got)
		require.Equal(t, "kept", bindingSubjectNamespace(t, bindingFrom(t, kubeClient)))
	}
}

// TestInstallSupportServicesOperatorClaimsAMissingBinding covers the first install.
func TestInstallSupportServicesOperatorClaimsAMissingBinding(t *testing.T) {
	// install where no manager binding exists
	objects, err := supportServicesOperatorObjects(RuntimeNamespace)
	require.NoError(t, err)
	mapper := manifestMapper(objects)
	kubeClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	err = InstallThreeportSupportServicesOperator(
		kubeClient,
		mapper,
		SupportServicesCloudLocal,
		RuntimeNamespace,
		"primary",
	)
	require.NoError(t, err)

	// the binding names this control plane and the runtime namespace
	owner, found, err := SupportServicesOperatorOwner(kubeClient, mapper)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "primary", owner)
	require.Equal(t, RuntimeNamespace, bindingSubjectNamespace(t, bindingFrom(t, kubeClient)))
}

// TestInstallSupportServicesOperatorUpdatesItsOwnBinding covers a binding this control plane already owns.
func TestInstallSupportServicesOperatorUpdatesItsOwnBinding(t *testing.T) {
	// install over a binding that already names this control plane
	objects, err := supportServicesOperatorObjects(RuntimeNamespace)
	require.NoError(t, err)
	mapper := manifestMapper(objects)
	kubeClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), managerBinding("primary", "old"))
	err = InstallThreeportSupportServicesOperator(
		kubeClient,
		mapper,
		SupportServicesCloudLocal,
		RuntimeNamespace,
		"primary",
	)
	require.NoError(t, err)

	// the subject moves to the runtime namespace and the name stays
	owner, found, err := SupportServicesOperatorOwner(kubeClient, mapper)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "primary", owner)
	require.Equal(t, RuntimeNamespace, bindingSubjectNamespace(t, bindingFrom(t, kubeClient)))
}

// TestRecordSupportServicesOperatorReplacementNamesTheReplacement covers a blue-green handoff.
func TestRecordSupportServicesOperatorReplacementNamesTheReplacement(t *testing.T) {
	// hand a binding named for the source to the replacement
	binding := managerBinding("source", "kept")
	kubeClient := testClient(binding)
	err := RecordSupportServicesOperatorReplacement(kubeClient, testMapper(), "source", "replacement")
	require.NoError(t, err)
	owner, found, err := SupportServicesOperatorOwner(kubeClient, testMapper())
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "replacement", owner)
	require.Equal(t, "kept", bindingSubjectNamespace(t, bindingFrom(t, kubeClient)))

	// leave a binding named for someone else, and one with no name
	for _, owner := range []string{"other", ""} {
		kubeClient = testClient(managerBinding(owner, "kept"))
		err = RecordSupportServicesOperatorReplacement(kubeClient, testMapper(), "source", "replacement")
		require.NoError(t, err)
		got, found, err := SupportServicesOperatorOwner(kubeClient, testMapper())
		require.NoError(t, err)
		require.True(t, found)
		require.Equal(t, owner, got)
	}
}

// managerBinding returns the operator manager binding with one subject namespace.
func managerBinding(owner, subjectNamespace string) *unstructured.Unstructured {
	// set the owner when the binding has one
	metadata := map[string]interface{}{"name": SupportServicesOperatorManagerBindingName}
	if owner != "" {
		metadata["labels"] = map[string]interface{}{LabelControlPlane: owner}
	}
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "rbac.authorization.k8s.io/v1",
		"kind":       "ClusterRoleBinding",
		"metadata":   metadata,
		"subjects": []interface{}{
			map[string]interface{}{
				"kind":      "ServiceAccount",
				"name":      "support-services-operator-controller-manager",
				"namespace": subjectNamespace,
			},
		},
	}}
}

// bindingFrom reads the operator manager binding back from the client.
func bindingFrom(t *testing.T, kubeClient dynamic.Interface) *unstructured.Unstructured {
	t.Helper()
	binding, found, err := resourceInstalled(
		kubeClient,
		testMapper(),
		"rbac.authorization.k8s.io",
		"v1",
		"ClusterRoleBinding",
		"",
		SupportServicesOperatorManagerBindingName,
	)
	require.NoError(t, err)
	require.True(t, found)
	return binding
}

// bindingSubjectNamespace returns the namespace of the binding's first subject.
func bindingSubjectNamespace(t *testing.T, binding *unstructured.Unstructured) string {
	t.Helper()
	subjects, _, err := unstructured.NestedSlice(binding.Object, "subjects")
	require.NoError(t, err)
	require.NotEmpty(t, subjects)
	namespace, _, err := unstructured.NestedString(subjects[0].(map[string]interface{}), "namespace")
	require.NoError(t, err)
	return namespace
}

// manifestMapper maps every kind in the operator manifest.
func manifestMapper(objects []*unstructured.Unstructured) *meta.RESTMapper {
	// register each object so the install can create it
	seen := map[schema.GroupVersion]struct{}{}
	var groups []schema.GroupVersion
	for _, object := range objects {
		gv := object.GroupVersionKind().GroupVersion()
		if _, ok := seen[gv]; ok {
			continue
		}
		seen[gv] = struct{}{}
		groups = append(groups, gv)
	}
	groups = append(groups, schema.GroupVersion{Group: "setup.addons.nukleros.io", Version: "v1alpha1"})
	mapper := meta.NewDefaultRESTMapper(groups)
	for _, object := range objects {
		scope := meta.RESTScopeNamespace
		if object.GetNamespace() == "" {
			scope = meta.RESTScopeRoot
		}
		mapper.Add(object.GroupVersionKind(), scope)
	}
	mapper.Add(schema.GroupVersionKind{
		Group:   "setup.addons.nukleros.io",
		Version: "v1alpha1",
		Kind:    "SupportServices",
	}, meta.RESTScopeRoot)
	var rest meta.RESTMapper = mapper
	return &rest
}
