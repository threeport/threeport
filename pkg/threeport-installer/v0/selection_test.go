package v0

import (
	"strings"
	"testing"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	"k8s.io/apimachinery/pkg/runtime"
)

// testControllers returns controllers covering the group-to-controller name mapping.
func testControllers() []*v0.ControlPlaneComponent {
	return []*v0.ControlPlaneComponent{
		{Name: "kubernetes-workload-controller"},
		{Name: "gateway-controller"},
		{Name: "kubernetes-runtime-controller"},
		{Name: "aws-controller"},
	}
}

// TestSelectControllersByGroup covers matching API object groups to
// controllers, including empty input and a group with no match.
func TestSelectControllersByGroup(t *testing.T) {
	// seed the controller list
	allControllers := testControllers()

	tests := []struct {
		name        string
		groupNames  []string
		controllers []*v0.ControlPlaneComponent
		wantNames   []string
		wantErrSub  string
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

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// select controllers by group
			got, err := SelectControllersByGroup(tc.groupNames, tc.controllers)
			if tc.wantErrSub != "" {
				// check the unknown-group error
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErrSub)
				}
				if !strings.Contains(err.Error(), tc.wantErrSub) {
					t.Fatalf("error %q does not contain %q", err.Error(), tc.wantErrSub)
				}
				return
			}
			// check selected controller names in order
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tc.wantNames) {
				t.Fatalf("got %d controllers, want %d", len(got), len(tc.wantNames))
			}
			for i, controller := range got {
				if controller.Name != tc.wantNames[i] {
					t.Errorf("position %d: got %q, want %q", i, controller.Name, tc.wantNames[i])
				}
			}
		})
	}
}

// TestSelectControllersByGroupErrorListsValidNames covers an unknown group
// error listing the valid API object group names.
func TestSelectControllersByGroupErrorListsValidNames(t *testing.T) {
	// select with an unknown group
	_, err := SelectControllersByGroup([]string{"bogus"}, testControllers())

	// check the unknown group returns an error
	if err == nil {
		t.Fatal("expected error for unknown group, got nil")
	}
	// check the error lists valid group names
	for _, expected := range []string{"aws", "gateway", "kubernetes_runtime", "kubernetes_workload"} {
		if !strings.Contains(err.Error(), expected) {
			t.Errorf("error %q missing expected group %q", err.Error(), expected)
		}
	}
}

// TestSelectControllersForReinstallFallsBackWhenNoneLabeled covers no labeled
// deployments, an API-server-only cluster, and one labeled controller.
func TestSelectControllersForReinstallFallsBackWhenNoneLabeled(t *testing.T) {
	// seed the controller list and namespace
	allControllers := testControllers()
	namespace := "threeport-control-plane"

	t.Run("no labeled deployments keeps the full controller set", func(t *testing.T) {
		// seed an empty cluster
		kubeClient := testKubeClient()

		// select controllers for reinstall
		selected, names, autoDetected, err := SelectControllersForReinstall(
			kubeClient, namespace, nil, allControllers,
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// check auto-detected is true
		if !autoDetected {
			t.Error("expected auto-detected to be true")
		}
		// check selected controller count
		if len(selected) != len(allControllers) {
			t.Fatalf("got %d controllers, want %d", len(selected), len(allControllers))
		}
		// check selected name count
		if len(names) != len(allControllers) {
			t.Fatalf("got %d names, want %d", len(names), len(allControllers))
		}
	})

	t.Run("labeled api server only keeps zero optional controllers", func(t *testing.T) {
		// seed an installer-managed API server only
		kubeClient := testKubeClient(testManagedDeployment(ThreeportAPIServiceResourceName, namespace))

		// select controllers for reinstall
		selected, names, autoDetected, err := SelectControllersForReinstall(
			kubeClient, namespace, nil, allControllers,
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// check auto-detected is true
		if !autoDetected {
			t.Error("expected auto-detected to be true")
		}
		// check selected controller count
		if len(selected) != 0 {
			t.Fatalf("got %d controllers, want 0", len(selected))
		}
		// check selected name count
		if len(names) != 0 {
			t.Fatalf("got %d names, want 0", len(names))
		}
	})

	t.Run("labeled controller is selected", func(t *testing.T) {
		// seed an installer-managed API server and gateway controller
		objects := []runtime.Object{
			testManagedDeployment(ThreeportAPIServiceResourceName, namespace),
			testManagedDeployment("threeport-gateway-controller", namespace),
		}
		kubeClient := testKubeClient(objects...)

		// select controllers for reinstall
		selected, names, autoDetected, err := SelectControllersForReinstall(
			kubeClient, namespace, nil, allControllers,
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// check auto-detected is true
		if !autoDetected {
			t.Error("expected auto-detected to be true")
		}
		// check the labeled controller is selected
		if len(selected) != 1 || selected[0].Name != "gateway-controller" {
			t.Fatalf("got %#v, want gateway-controller", names)
		}
	})
}
