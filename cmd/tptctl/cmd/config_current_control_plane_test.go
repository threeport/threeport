package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	cli "github.com/threeport/threeport/pkg/cli/v0"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
)

func ptrTo[T any](v T) *T { return &v }

// TestResolveControlPlaneRequest covers what the config alone says about a
// requested control plane, including the empty current control plane that
// tptctl down leaves behind.
func TestResolveControlPlaneRequest(t *testing.T) {
	genesis := cli.ControlPlane{Name: "genesis", Genesis: true}
	other := cli.ControlPlane{Name: "other"}

	t.Run("a control plane in the config is found", func(t *testing.T) {
		cfg := &cli.ThreeportConfig{ControlPlanes: []cli.ControlPlane{genesis, other}}
		assert.True(t, resolveControlPlaneRequest(cfg, "other").inConfig)
	})

	t.Run("an empty current control plane is not a control plane", func(t *testing.T) {
		cfg := &cli.ThreeportConfig{
			ControlPlanes: []cli.ControlPlane{{Name: ""}, other},
		}
		resolution := resolveControlPlaneRequest(cfg, "child")
		assert.False(t, resolution.inConfig)
		assert.Nil(t, resolution.current)
		require.NotNil(t, resolution.any)
	})

	t.Run("the current control plane and genesis are reported", func(t *testing.T) {
		cfg := &cli.ThreeportConfig{
			ControlPlanes:       []cli.ControlPlane{genesis, other},
			CurrentControlPlane: "genesis",
		}
		resolution := resolveControlPlaneRequest(cfg, "child")
		assert.False(t, resolution.inConfig)
		require.NotNil(t, resolution.current)
		assert.Equal(t, "genesis", resolution.current.Name)
		require.NotNil(t, resolution.genesis)
		assert.Equal(t, "genesis", resolution.genesis.Name)
	})

	t.Run("a current control plane missing from the config is not found", func(t *testing.T) {
		cfg := &cli.ThreeportConfig{
			ControlPlanes:       []cli.ControlPlane{other},
			CurrentControlPlane: "gone",
		}
		resolution := resolveControlPlaneRequest(cfg, "child")
		assert.Nil(t, resolution.current)
		assert.Nil(t, resolution.genesis)
		require.NotNil(t, resolution.any)
		assert.Equal(t, "other", resolution.any.Name)
	})

	t.Run("an empty config has nothing to search from", func(t *testing.T) {
		resolution := resolveControlPlaneRequest(&cli.ThreeportConfig{}, "child")
		assert.False(t, resolution.inConfig)
		assert.Nil(t, resolution.current)
		assert.Nil(t, resolution.genesis)
		assert.Nil(t, resolution.any)
	})
}

// controlPlaneInstanceServer answers the by-name lookup the way the API does:
// the matching instance in a list, and no Children on any of them.
func controlPlaneInstanceServer(t *testing.T, instances ...v0.ControlPlaneInstance) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		var matches []v0.ControlPlaneInstance
		for _, instance := range instances {
			if instance.Name != nil && *instance.Name == name {
				matches = append(matches, instance)
			}
		}
		if len(matches) == 0 {
			// the API answers a name that matches nothing with an empty list
			matches = []v0.ControlPlaneInstance{}
		}
		require.NoError(t, json.NewEncoder(w).Encode(map[string]interface{}{"Data": matches}))
	}))
	t.Cleanup(server.Close)
	return server
}

func controlPlaneInstance(id uint, name string, parent *uint) v0.ControlPlaneInstance {
	instance := v0.ControlPlaneInstance{Name: ptrTo(name), ParentControlPlaneInstanceID: parent}
	instance.ID = ptrTo(id)
	return instance
}

// TestFindChildControlPlane is the case that panicked: the current control
// plane is the genesis, the requested one is its child and is not in the
// config, and the by-name lookup returns no Children.
func TestFindChildControlPlane(t *testing.T) {
	server := controlPlaneInstanceServer(t,
		controlPlaneInstance(1, "genesis", nil),
		controlPlaneInstance(2, "child", ptrTo(uint(1))),
		controlPlaneInstance(3, "grandchild", ptrTo(uint(2))),
		controlPlaneInstance(4, "unrelated", ptrTo(uint(9))),
	)
	addr := strings.TrimPrefix(server.URL, "http://")

	t.Run("a child of the current control plane is found without its Children", func(t *testing.T) {
		child, err := findChildControlPlane(server.Client(), addr, "genesis", "child")
		require.NoError(t, err)
		assert.Equal(t, "child", *child.Name)
		assert.Nil(t, child.Children)
	})

	t.Run("a grandchild is not a child", func(t *testing.T) {
		_, err := findChildControlPlane(server.Client(), addr, "genesis", "grandchild")
		assert.ErrorIs(t, err, errNotChildControlPlane)
	})

	t.Run("a control plane with another parent is not a child", func(t *testing.T) {
		_, err := findChildControlPlane(server.Client(), addr, "genesis", "unrelated")
		assert.ErrorIs(t, err, errNotChildControlPlane)
	})

	t.Run("the genesis has no parent so it is not a child", func(t *testing.T) {
		_, err := findChildControlPlane(server.Client(), addr, "child", "genesis")
		assert.ErrorIs(t, err, errNotChildControlPlane)
	})

	t.Run("a name that does not exist is reported as not found", func(t *testing.T) {
		_, err := findChildControlPlane(server.Client(), addr, "genesis", "typo")
		assert.ErrorIs(t, err, client_lib.ErrObjectNotFound)
	})

	t.Run("a current control plane that does not exist is an error", func(t *testing.T) {
		_, err := findChildControlPlane(server.Client(), addr, "typo", "child")
		assert.ErrorIs(t, err, client_lib.ErrObjectNotFound)
	})
}

// TestControlPlaneConfigFromInstance covers the optional fields the API can
// leave out, which used to be dereferenced after the config had been written.
func TestControlPlaneConfigFromInstance(t *testing.T) {
	instance := func() *v0.ControlPlaneInstance {
		return &v0.ControlPlaneInstance{
			Name:              ptrTo("child"),
			ApiServerEndpoint: ptrTo("child.example.com:443"),
			CACert:            ptrTo("ca"),
			ClientCert:        ptrTo("cert"),
			ClientKey:         ptrTo("key"),
			Genesis:           ptrTo(false),
		}
	}
	definition := func(auth bool) *v0.ControlPlaneDefinition {
		return &v0.ControlPlaneDefinition{AuthEnabled: ptrTo(auth)}
	}
	runtime := func() *v0.KubernetesRuntimeDefinition {
		return &v0.KubernetesRuntimeDefinition{InfraProvider: ptrTo("gke")}
	}

	t.Run("an auth enabled control plane gets its credentials", func(t *testing.T) {
		got, err := controlPlaneConfigFromInstance(instance(), definition(true), runtime())
		require.NoError(t, err)
		assert.Equal(t, "child", got.Name)
		assert.Equal(t, "child.example.com:443", got.APIServer)
		assert.Equal(t, "gke", got.Provider)
		assert.True(t, got.AuthEnabled)
		assert.Equal(t, "ca", got.CACert)
		require.Len(t, got.Credentials, 1)
		assert.Equal(t, "cert", got.Credentials[0].ClientCert)
		assert.Equal(t, "key", got.Credentials[0].ClientKey)
	})

	t.Run("a control plane without auth needs no certs", func(t *testing.T) {
		withoutCerts := instance()
		withoutCerts.CACert, withoutCerts.ClientCert, withoutCerts.ClientKey = nil, nil, nil
		got, err := controlPlaneConfigFromInstance(withoutCerts, definition(false), runtime())
		require.NoError(t, err)
		assert.False(t, got.AuthEnabled)
		assert.Empty(t, got.Credentials)
	})

	t.Run("a genesis control plane is marked as one", func(t *testing.T) {
		genesis := instance()
		genesis.Genesis = ptrTo(true)
		got, err := controlPlaneConfigFromInstance(genesis, definition(true), runtime())
		require.NoError(t, err)
		assert.True(t, got.Genesis)
	})

	for name, mutate := range map[string]func(*v0.ControlPlaneInstance, *v0.ControlPlaneDefinition, *v0.KubernetesRuntimeDefinition){
		"name": func(i *v0.ControlPlaneInstance, _ *v0.ControlPlaneDefinition, _ *v0.KubernetesRuntimeDefinition) {
			i.Name = nil
		},
		"API server": func(i *v0.ControlPlaneInstance, _ *v0.ControlPlaneDefinition, _ *v0.KubernetesRuntimeDefinition) {
			i.ApiServerEndpoint = nil
		},
		"CA cert": func(i *v0.ControlPlaneInstance, _ *v0.ControlPlaneDefinition, _ *v0.KubernetesRuntimeDefinition) {
			i.CACert = nil
		},
		"client cert": func(i *v0.ControlPlaneInstance, _ *v0.ControlPlaneDefinition, _ *v0.KubernetesRuntimeDefinition) {
			i.ClientCert = nil
		},
		"client key": func(i *v0.ControlPlaneInstance, _ *v0.ControlPlaneDefinition, _ *v0.KubernetesRuntimeDefinition) {
			i.ClientKey = nil
		},
		"auth enabled": func(_ *v0.ControlPlaneInstance, d *v0.ControlPlaneDefinition, _ *v0.KubernetesRuntimeDefinition) {
			d.AuthEnabled = nil
		},
		"infra provider": func(_ *v0.ControlPlaneInstance, _ *v0.ControlPlaneDefinition, r *v0.KubernetesRuntimeDefinition) {
			r.InfraProvider = nil
		},
	} {
		t.Run("a missing "+name+" is an error, not a panic", func(t *testing.T) {
			i, d, r := instance(), definition(true), runtime()
			mutate(i, d, r)
			assert.NotPanics(t, func() {
				got, err := controlPlaneConfigFromInstance(i, d, r)
				assert.Error(t, err)
				assert.Nil(t, got)
			})
		})
	}
}
