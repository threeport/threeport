package cmd

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/fake"

	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestUpgradeRollTargetMatrix checks which control plane an upgrade rolls.
func TestUpgradeRollTargetMatrix(t *testing.T) {
	genesisID := uint(1)
	primaryID := uint(2)
	childID := uint(3)
	genesis := &api_v0.ControlPlaneInstance{
		Common:    api_v0.Common{ID: &genesisID},
		Instance:  api_v0.Instance{Name: util.Ptr("genesis")},
		Namespace: util.Ptr("genesis-ns"),
		IsSelf:    util.Ptr(true),
	}
	primary := &api_v0.ControlPlaneInstance{
		Common:                       api_v0.Common{ID: &primaryID},
		Instance:                     api_v0.Instance{Name: util.Ptr("primary")},
		Namespace:                    util.Ptr("primary-ns"),
		IsSelf:                       util.Ptr(false),
		ParentControlPlaneInstanceID: &genesisID,
	}
	child := &api_v0.ControlPlaneInstance{
		Common:                       api_v0.Common{ID: &childID},
		Instance:                     api_v0.Instance{Name: util.Ptr("child")},
		Namespace:                    util.Ptr("child-ns"),
		IsSelf:                       util.Ptr(false),
		ParentControlPlaneInstanceID: &primaryID,
	}
	byName := func(name string) (*api_v0.ControlPlaneInstance, error) {
		switch name {
		case "primary":
			return primary, nil
		case "child":
			return child, nil
		default:
			return nil, errors.New("not found")
		}
	}
	byID := func(id uint) (*api_v0.ControlPlaneInstance, error) {
		switch id {
		case genesisID:
			return genesis, nil
		case primaryID:
			return primary, nil
		default:
			return nil, errors.New("not found")
		}
	}

	// each row is the connection the command already resolved, plus --name
	rows := []struct {
		name    string
		self    *api_v0.ControlPlaneInstance
		roll    string
		want    string
		refused bool
	}{
		{name: "row 1 current genesis", self: genesis, want: "genesis"},
		{name: "row 2 flag genesis", self: genesis, want: "genesis"},
		{name: "row 3 current primary", self: primarySelf(primaryID, genesisID), refused: true},
		{name: "row 4 flag primary", self: primarySelf(primaryID, genesisID), refused: true},
		{name: "row 5 flag child", self: childSelf(childID, primaryID), refused: true},
		{name: "row 6 flag genesis no name", self: genesis, want: "genesis"},
		{name: "row 7 genesis name primary", self: genesis, roll: "primary", want: "primary"},
		{name: "row 7 genesis name child", self: genesis, roll: "child", want: "child"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			kube := deploymentClient(t)
			err := rollNamespaceImages(row.self, row.roll, byName, byID, kube, "new-tag")
			if row.refused {
				require.ErrorIs(t, err, api_v0.ErrRequestMustBeMadeToParent)
				require.Equal(t, "repo:old", deploymentImage(t, kube, "genesis-ns"))
				require.Equal(t, "repo:old", deploymentImage(t, kube, "primary-ns"))
				t.Logf("%s refused parent connection, images unchanged", row.name)
				return
			}
			require.NoError(t, err)
			require.Equal(t, "repo:new-tag", deploymentImage(t, kube, row.want+"-ns"))
			if row.want != "genesis" {
				require.Equal(t, "repo:old", deploymentImage(t, kube, "genesis-ns"))
			}
			t.Logf("%s rolls %s", row.name, row.want)
		})
	}

	// a name that is not a descendant is refused
	_, err := upgradeRollTarget(genesis, "other", byName, byID)
	require.Error(t, err)
}

// childSelf is a child API's own row, so a direct connection is refused.
func childSelf(id, parent uint) *api_v0.ControlPlaneInstance {
	return &api_v0.ControlPlaneInstance{
		Common:                       api_v0.Common{ID: &id},
		Instance:                     api_v0.Instance{Name: util.Ptr("child")},
		IsSelf:                       util.Ptr(true),
		ParentControlPlaneInstanceID: &parent,
	}
}

// deploymentClient holds one deployment in each control plane namespace.
func deploymentClient(t *testing.T) dynamic.Interface {
	// seed genesis, primary, and child with the old tag
	scheme := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))
	var objects []runtime.Object
	for _, ns := range []string{"genesis-ns", "primary-ns", "child-ns"} {
		objects = append(objects, &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata":   map[string]interface{}{"name": "api", "namespace": ns},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{"name": "api", "image": "repo:old"},
						},
					},
				},
			},
		}})
	}
	return fake.NewSimpleDynamicClient(scheme, objects...)
}

// deploymentImage reads the api container image in one namespace.
func deploymentImage(t *testing.T, kube dynamic.Interface, namespace string) string {
	// read the image the roll wrote
	gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	got, err := kube.Resource(gvr).Namespace(namespace).Get(t.Context(), "api", metav1.GetOptions{})
	require.NoError(t, err)
	containers, _, _ := unstructured.NestedSlice(got.Object, "spec", "template", "spec", "containers")
	container := containers[0].(map[string]interface{})
	return container["image"].(string)
}

// primarySelf is a primary API's own row, so a direct connection is refused.
func primarySelf(id, parent uint) *api_v0.ControlPlaneInstance {
	return &api_v0.ControlPlaneInstance{
		Common:                       api_v0.Common{ID: &id},
		Instance:                     api_v0.Instance{Name: util.Ptr("primary")},
		IsSelf:                       util.Ptr(true),
		ParentControlPlaneInstanceID: &parent,
	}
}
