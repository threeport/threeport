package v0

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/version"
	fakediscovery "k8s.io/client-go/discovery/fake"
	k8stesting "k8s.io/client-go/testing"
)

// TestServerMinorVersion covers reading a cluster's minor version from the
// versions providers report.
func TestServerMinorVersion(t *testing.T) {
	for name, tc := range map[string]struct {
		info version.Info
		want string
	}{
		"upstream":           {version.Info{Major: "1", Minor: "36", GitVersion: "v1.36.2"}, "1.36"},
		"eks marks it":       {version.Info{Major: "1", Minor: "35+", GitVersion: "v1.35.4-eks-a1b2c3d"}, "1.35"},
		"gke":                {version.Info{Major: "1", Minor: "36", GitVersion: "v1.36.4-gke.2377000"}, "1.36"},
		"only a git version": {version.Info{GitVersion: "v1.34.10"}, "1.34"},
	} {
		t.Run(name, func(t *testing.T) {
			info := tc.info
			client := &fakediscovery.FakeDiscovery{Fake: &k8stesting.Fake{}, FakedServerVersion: &info}
			got, err := ServerMinorVersion(client)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}

	client := &fakediscovery.FakeDiscovery{Fake: &k8stesting.Fake{}, FakedServerVersion: &version.Info{GitVersion: "unknown"}}
	_, err := ServerMinorVersion(client)
	assert.ErrorContains(t, err, "reports no version it can be read from")
}
