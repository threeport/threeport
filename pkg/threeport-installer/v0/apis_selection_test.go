package v0

import (
	"strings"
	"testing"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	"k8s.io/apimachinery/pkg/runtime"
)

// testControllers returns controllers that cover group-to-controller name mapping.
func testControllers() []*v0.ControlPlaneComponent {
	return []*v0.ControlPlaneComponent{
		{Name: "kubernetes-workload-controller"},
		{Name: "gateway-controller"},
		{Name: "kubernetes-runtime-controller"},
		{Name: "aws-controller"},
	}
}

// TestSelectControllersByGroup covers matching groups to controllers, including
// empty input and a group with no match.
func TestSelectControllersByGroup(t *testing.T) {
	// build the shared controller list
	allControllers := testControllers()

	// define the selection cases
	tests := []struct {
		// The subtest name
		name string
		// The API object group names passed in
		groupNames []string
		// The controllers the selection searches
		controllers []*v0.ControlPlaneComponent
		// The controller names expected in order
		wantNames []string
		// The error text that must appear, or empty for success
		wantErrSub string
	}{
		{
			name:        "empty group names returns all controllers unchanged",
			groupNames:  nil,
			controllers: allControllers,
			wantNames: []string{
				"kubernetes-workload-controller",
				"gateway-controller",
				"kubernetes-runtime-controller",
				"aws-controller",
			},
		},
		{
			name:        "none selects zero controllers",
			groupNames:  []string{"none"},
			controllers: allControllers,
			wantNames:   []string{},
		},
		{
			name:        "single group selects matching controller",
			groupNames:  []string{"gateway"},
			controllers: allControllers,
			wantNames:   []string{"gateway-controller"},
		},
		{
			name:        "underscored group maps to dashed controller name",
			groupNames:  []string{"kubernetes_workload"},
			controllers: allControllers,
			wantNames:   []string{"kubernetes-workload-controller"},
		},
		{
			name:        "multiple groups select matching controllers in order",
			groupNames:  []string{"kubernetes_workload", "gateway"},
			controllers: allControllers,
			wantNames: []string{
				"kubernetes-workload-controller",
				"gateway-controller",
			},
		},
		{
			name:        "unknown group returns error listing valid choices",
			groupNames:  []string{"bogus_group"},
			controllers: allControllers,
			wantErrSub:  "unknown api object group",
		},
		{
			name:       "group with no controller in trimmed list returns error",
			groupNames: []string{"aws"},
			controllers: []*v0.ControlPlaneComponent{
				{Name: "gateway-controller"},
				{Name: "kubernetes-workload-controller"},
			},
			wantErrSub: "unknown api object group \"aws\"",
		},
	}

	// run each case
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// select controllers for the case
			got, err := SelectControllersByGroup(tc.groupNames, tc.controllers)

			// check the expected error text
			if tc.wantErrSub != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErrSub)
				}
				if !strings.Contains(err.Error(), tc.wantErrSub) {
					t.Fatalf("error %q does not contain %q", err.Error(), tc.wantErrSub)
				}
				return
			}

			// check the call returned no error
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// check the controller count
			if len(got) != len(tc.wantNames) {
				t.Fatalf("got %d controllers, want %d", len(got), len(tc.wantNames))
			}

			// check each controller name in order
			for i, controller := range got {
				if controller.Name != tc.wantNames[i] {
					t.Errorf("position %d: got %q, want %q", i, controller.Name, tc.wantNames[i])
				}
			}
		})
	}
}

// TestSelectControllersByGroupErrorListsValidNames covers an unknown group error
// that includes the four fixture group names.
func TestSelectControllersByGroupErrorListsValidNames(t *testing.T) {
	// select an unknown group
	_, err := SelectControllersByGroup([]string{"bogus"}, testControllers())

	// check the call returned an error
	if err == nil {
		t.Fatal("expected error for unknown group, got nil")
	}

	// check these group names appear in the error
	for _, expected := range []string{"aws", "gateway", "kubernetes_runtime", "kubernetes_workload"} {
		if !strings.Contains(err.Error(), expected) {
			t.Errorf("error %q missing expected group %q", err.Error(), expected)
		}
	}
}

// TestSelectControllersForReinstallFallsBackWhenNoneLabeled covers no labeled
// deployments, an API-server-only cluster, and one labeled controller.
func TestSelectControllersForReinstallFallsBackWhenNoneLabeled(t *testing.T) {
	// build the shared controller list and namespace
	allControllers := testControllers()
	namespace := "threeport-control-plane"

	// run the empty-cluster case
	t.Run("no labeled deployments keeps the full controller set", func(t *testing.T) {
		// build a client with no deployments
		kubeClient := testKubeClient()

		// select with no explicit groups
		selected, names, autoDetected, err := SelectControllersForReinstall(
			kubeClient, namespace, nil, allControllers,
		)

		// check the call returned no error
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// check auto-detected is true
		if !autoDetected {
			t.Error("expected auto-detected to be true")
		}

		// check the selected count
		if len(selected) != len(allControllers) {
			t.Fatalf("got %d controllers, want %d", len(selected), len(allControllers))
		}

		// check the name count
		if len(names) != len(allControllers) {
			t.Fatalf("got %d names, want %d", len(names), len(allControllers))
		}
	})

	// run the API-server-only case
	t.Run("labeled api server only keeps zero optional controllers", func(t *testing.T) {
		// seed an installer-managed API server only
		kubeClient := testKubeClient(testManagedDeployment(ThreeportAPIServiceResourceName, namespace))

		// select with no explicit groups
		selected, names, autoDetected, err := SelectControllersForReinstall(
			kubeClient, namespace, nil, allControllers,
		)

		// check the call returned no error
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// check auto-detected is true
		if !autoDetected {
			t.Error("expected auto-detected to be true")
		}

		// check the selected count
		if len(selected) != 0 {
			t.Fatalf("got %d controllers, want 0", len(selected))
		}

		// check the name count
		if len(names) != 0 {
			t.Fatalf("got %d names, want 0", len(names))
		}
	})

	// run the labeled-controller case
	t.Run("labeled controller is selected", func(t *testing.T) {
		// seed an installer-managed API server and gateway controller
		objects := []runtime.Object{
			testManagedDeployment(ThreeportAPIServiceResourceName, namespace),
			testManagedDeployment("threeport-gateway-controller", namespace),
		}
		kubeClient := testKubeClient(objects...)

		// select with no explicit groups
		selected, names, autoDetected, err := SelectControllersForReinstall(
			kubeClient, namespace, nil, allControllers,
		)

		// check the call returned no error
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// check auto-detected is true
		if !autoDetected {
			t.Error("expected auto-detected to be true")
		}

		// check only the gateway controller is selected
		if len(selected) != 1 || selected[0].Name != "gateway-controller" {
			t.Fatalf("got %#v, want gateway-controller", names)
		}
	})
}
