package v0

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// TestValidateInstallIdentity checks each flag combination for a second install.
func TestValidateInstallIdentity(t *testing.T) {
	// each case is one flag combination
	cases := []struct {
		name    string
		opts    Options
		wantErr string
	}{
		{
			name: "defaults",
			opts: Options{Namespace: ControlPlaneNamespace, AuthEnabled: true},
		},
		{
			name: "encryption key file alone",
			opts: Options{Namespace: "new", AuthEnabled: false, EncryptionKeyFile: "key"},
		},
		{
			name: "ca files without an encryption key",
			opts: Options{Namespace: "new", AuthEnabled: true, CACertFile: "cert", CAKeyFile: "key"},
		},
		{
			name:    "from-namespace is the install",
			opts:    Options{Namespace: "same", FromNamespace: "same", AuthEnabled: true},
			wantErr: "from-namespace is the namespace being installed",
		},
		{
			name: "from-namespace with a file",
			opts: Options{
				Namespace:         "new",
				FromNamespace:     "old",
				EncryptionKeyFile: "key",
				AuthEnabled:       true,
			},
			wantErr: "from-namespace cannot be combined with ca or encryption-key files",
		},
		{
			name:    "ca cert without a key",
			opts:    Options{Namespace: "new", AuthEnabled: true, CACertFile: "cert"},
			wantErr: "ca cert and ca key must both be set or both be empty",
		},
		{
			name:    "ca key without a cert",
			opts:    Options{Namespace: "new", AuthEnabled: true, CAKeyFile: "key"},
			wantErr: "ca cert and ca key must both be set or both be empty",
		},
		{
			name:    "ca files with auth disabled",
			opts:    Options{Namespace: "new", AuthEnabled: false, CACertFile: "cert", CAKeyFile: "key"},
			wantErr: "ca files are unused when auth is disabled",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateInstallIdentity(tc.opts)
			if tc.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.EqualError(t, err, tc.wantErr)
		})
	}
}

// TestRestoreReplicaCount checks which deployments a stopped restore brings back.
func TestRestoreReplicaCount(t *testing.T) {
	// a stopped restore keeps the recorded count only for the API and the nats CLI
	cases := []struct {
		name        string
		deployment  string
		recorded    int64
		stopped     bool
		want        int64
		clearRecord bool
	}{
		{name: "flag off", deployment: "workload-controller", recorded: 2, want: 2, clearRecord: true},
		{name: "api comes back", deployment: ThreeportAPIServiceResourceName, recorded: 3, stopped: true, want: 3, clearRecord: true},
		{name: "nats cli comes back", deployment: natsBoxDeployName, recorded: 1, stopped: true, want: 1, clearRecord: true},
		{name: "controller stays stopped", deployment: "workload-controller", recorded: 2, stopped: true, want: 0, clearRecord: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, clear := restoreReplicaCount(tc.deployment, tc.recorded, tc.stopped)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.clearRecord, clear)
		})
	}
}

// TestApplyInstallPlacement checks that the install label is set and affinity is left alone.
func TestApplyInstallPlacement(t *testing.T) {
	// an existing term stays, and no term for the source namespace is added
	resource := &unstructured.Unstructured{Object: map[string]interface{}{
		"kind": "Deployment",
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{"app": "crdb"},
				},
				"spec": map[string]interface{}{
					"affinity": map[string]interface{}{
						"podAntiAffinity": map[string]interface{}{
							"preferredDuringSchedulingIgnoredDuringExecution": []interface{}{
								map[string]interface{}{"weight": int64(100)},
							},
							"requiredDuringSchedulingIgnoredDuringExecution": []interface{}{
								map[string]interface{}{"topologyKey": "kubernetes.io/hostname"},
							},
						},
					},
				},
			},
		},
	}}

	err := applyInstallPlacement(resource, "new-ns")
	require.NoError(t, err)

	labels, _, err := unstructured.NestedStringMap(resource.Object, "spec", "template", "metadata", "labels")
	require.NoError(t, err)
	assert.Equal(t, "new-ns", labels[LabelInstall])
	assert.Equal(t, "crdb", labels["app"])

	preferred, found, err := unstructured.NestedSlice(
		resource.Object,
		"spec", "template", "spec", "affinity", "podAntiAffinity",
		"preferredDuringSchedulingIgnoredDuringExecution",
	)
	require.NoError(t, err)
	require.True(t, found)
	require.Len(t, preferred, 1)

	required, found, err := unstructured.NestedSlice(
		resource.Object,
		"spec", "template", "spec", "affinity", "podAntiAffinity",
		"requiredDuringSchedulingIgnoredDuringExecution",
	)
	require.NoError(t, err)
	require.True(t, found)
	assert.Len(t, required, 1)
}

// TestApplyInstallPlacementKeepsAGoInt checks that a plain int in the pod spec does not panic the copy.
func TestApplyInstallPlacementKeepsAGoInt(t *testing.T) {
	// a container port is a Go int, the type a hand-built manifest literal produces
	resource := &unstructured.Unstructured{Object: map[string]interface{}{
		"kind": "StatefulSet",
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{
							"name": "nats",
							"ports": []interface{}{
								map[string]interface{}{"containerPort": 4222},
							},
						},
					},
				},
			},
		},
	}}

	err := applyInstallPlacement(resource, "genesis-02")
	require.NoError(t, err)

	spec, found, err := unstructured.NestedFieldNoCopy(resource.Object, "spec", "template", "spec")
	require.NoError(t, err)
	require.True(t, found)
	containers := spec.(map[string]interface{})["containers"].([]interface{})
	ports := containers[0].(map[string]interface{})["ports"].([]interface{})
	assert.Equal(t, 4222, ports[0].(map[string]interface{})["containerPort"])

	_, found, err = unstructured.NestedFieldNoCopy(
		resource.Object,
		"spec", "template", "spec", "affinity",
	)
	require.NoError(t, err)
	require.False(t, found)
}

// TestApplyInstallPlacementLabelOnly checks that an empty source namespace sets only the label.
func TestApplyInstallPlacementLabelOnly(t *testing.T) {
	// an empty source namespace sets the label and leaves affinity unset
	resource := &unstructured.Unstructured{Object: map[string]interface{}{
		"kind": "StatefulSet",
		"spec": map[string]interface{}{
			"template": map[string]interface{}{},
		},
	}}

	err := applyInstallPlacement(resource, "new-ns")
	require.NoError(t, err)

	labels, _, err := unstructured.NestedStringMap(resource.Object, "spec", "template", "metadata", "labels")
	require.NoError(t, err)
	assert.Equal(t, "new-ns", labels[LabelInstall])

	_, found, err := unstructured.NestedMap(resource.Object, "spec", "template", "spec", "affinity")
	require.NoError(t, err)
	assert.False(t, found)
}

// TestApplyInstallPlacementIgnoresOtherKinds checks that a non-workload is unchanged.
func TestApplyInstallPlacementIgnoresOtherKinds(t *testing.T) {
	// a secret is not a workload and is left unchanged
	resource := &unstructured.Unstructured{Object: map[string]interface{}{
		"kind": "Secret",
		"metadata": map[string]interface{}{
			"name": "encryption-key",
		},
	}}
	before := resource.DeepCopy()

	err := applyInstallPlacement(resource, "new-ns")
	require.NoError(t, err)
	assert.Equal(t, before.Object, resource.Object)
}
