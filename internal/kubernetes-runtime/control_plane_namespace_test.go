package kubernetesruntime

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setControlPlaneNamespace stands in for the pod's namespace file and returns a
// function restoring it. Read for real, these tests would depend on whether they
// happen to run inside a pod.
func setControlPlaneNamespace(t *testing.T, read func() (string, error)) {
	t.Helper()
	previous := readControlPlaneNamespace
	readControlPlaneNamespace = read
	t.Cleanup(func() { readControlPlaneNamespace = previous })
}

// The identity a managed cluster is told to authorize is the one these
// controllers present, which is decided by where they are actually running.
// A child control plane's controllers run in the child's namespace, while its
// API still holds a record of the genesis control plane - so the record cannot
// answer this and the pod has to.

func TestControlPlaneNamespace_ReadsWhereThisControllerRuns(t *testing.T) {
	setControlPlaneNamespace(t, func() (string, error) { return "a-child-namespace", nil })

	namespace, err := controlPlaneNamespace()
	require.NoError(t, err)
	assert.Equal(t, "a-child-namespace", namespace)
}

// guessing here authorizes a principal that never connects, so it says so
func TestControlPlaneNamespace_UnreadableIsAnError(t *testing.T) {
	setControlPlaneNamespace(t, func() (string, error) {
		return "", errors.New("no namespace file")
	})

	_, err := controlPlaneNamespace()
	require.Error(t, err)
}

// TestReadControlPlaneNamespace_OutsideAPod covers the real read where there is
// no pod to read from, which is where this repository's tests and a developer
// machine both are.
func TestReadControlPlaneNamespace_OutsideAPod(t *testing.T) {
	if _, err := readControlPlaneNamespace(); err == nil {
		t.Skip("running inside a pod, where the namespace file exists")
	}
}
