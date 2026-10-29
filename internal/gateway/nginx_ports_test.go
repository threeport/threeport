package gateway

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/threeport/threeport/internal/machinetest"
	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestSupportedHTTPPort accepts the ports nginx publishes.
func TestSupportedHTTPPort(t *testing.T) {
	// accept 80, and 443 only when tls is on
	require.NoError(t, supportedHTTPPort(80, false))
	require.NoError(t, supportedHTTPPort(80, true))
	require.NoError(t, supportedHTTPPort(443, true))
	require.Error(t, supportedHTTPPort(443, false))
	require.Error(t, supportedHTTPPort(8080, true))
}

// TestGetIngressesYamlOneIngressPerHost covers ports 80 and 443 on one host.
func TestGetIngressesYamlOneIngressPerHost(t *testing.T) {
	// return an http port and a tls port for the same host
	api := machinetest.NewAPIStub(t)
	api.Mux.HandleFunc(v0.PathGatewayHttpPorts, func(w http.ResponseWriter, r *http.Request) {
		httpPort := v0.GatewayHttpPort{Port: util.Ptr(80), Path: util.Ptr("/"), TLSEnabled: util.Ptr(false)}
		tlsPort := v0.GatewayHttpPort{Port: util.Ptr(443), Path: util.Ptr("/"), TLSEnabled: util.Ptr(true)}
		machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{httpPort, tlsPort})
	})
	reconciler := &controller.Reconciler{APIServer: api.Addr, APIClient: api.Client}
	definition := &v0.GatewayDefinition{
		Common:     v0.Common{ID: util.Ptr(uint(1))},
		Definition: v0.Definition{Name: util.Ptr("echo")},
	}

	// expect one ingress that terminates tls for the host
	got, err := getIngressesYaml(reconciler, definition, "echo.threeport.test")
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Contains(t, got[0], "name: echo-threeport-test")
	require.NotContains(t, got[0], "name: echo-threeport-test-443")
	require.Contains(t, got[0], "secretName: echo-threeport-test-tls")
	require.Contains(t, got[0], "host: echo.threeport.test")
	require.Contains(t, got[0], "nginx.org/ssl-redirect: \"false\"")
}

// TestGetIngressesYamlRejectsUnpublishedPort covers the ingress call site.
func TestGetIngressesYamlRejectsUnpublishedPort(t *testing.T) {
	// return an http port nginx does not publish
	api := machinetest.NewAPIStub(t)
	api.Mux.HandleFunc(v0.PathGatewayHttpPorts, func(w http.ResponseWriter, r *http.Request) {
		httpPort := v0.GatewayHttpPort{
			Port:       util.Ptr(8080),
			TLSEnabled: util.Ptr(true),
		}
		machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{httpPort})
	})
	reconciler := &controller.Reconciler{APIServer: api.Addr, APIClient: api.Client}

	// reject the port before an ingress is rendered
	_, err := getIngressesYaml(reconciler, &v0.GatewayDefinition{Common: v0.Common{ID: util.Ptr(uint(1))}}, "app.example.com")
	require.Error(t, err)
	require.Contains(t, err.Error(), "8080")
}

// TestTcpListenersUseTheRuntimeSet drops a duplicate port.
func TestTcpListenersUseTheRuntimeSet(t *testing.T) {
	// keep one listener per port
	listeners := tcpListeners([]int{5432, 443, 5432})
	require.Len(t, listeners, 2)
	require.Equal(t, "tcp-443", listeners[0].(map[string]interface{})["name"])
	require.Equal(t, "tcp-5432", listeners[1].(map[string]interface{})["name"])
}

// TestGetIngressesYamlDistinctPaths covers two paths on one host.
func TestGetIngressesYamlDistinctPaths(t *testing.T) {
	// return two paths for one host
	api := machinetest.NewAPIStub(t)
	api.Mux.HandleFunc(v0.PathGatewayHttpPorts, func(w http.ResponseWriter, r *http.Request) {
		health := v0.GatewayHttpPort{Port: util.Ptr(80), Path: util.Ptr("/health"), TLSEnabled: util.Ptr(false)}
		root := v0.GatewayHttpPort{Port: util.Ptr(443), Path: util.Ptr("/"), TLSEnabled: util.Ptr(true)}
		machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{health, root})
	})
	reconciler := &controller.Reconciler{APIServer: api.Addr, APIClient: api.Client}
	definition := &v0.GatewayDefinition{
		Common:     v0.Common{ID: util.Ptr(uint(1))},
		Definition: v0.Definition{Name: util.Ptr("echo")},
	}

	// expect both paths on the one ingress
	got, err := getIngressesYaml(reconciler, definition, "echo.threeport.test")
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Contains(t, got[0], "path: /health")
	require.Contains(t, got[0], "path: /")
}
