package gateway

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/threeport/threeport/internal/machinetest"
	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	"github.com/threeport/threeport/pkg/controller/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// portFixture holds the API rows a port reconcile reads.
type portFixture struct {
	gateways  []v0.GatewayInstance
	tcpPorts  map[string][]v0.GatewayTcpPort
	resources []v0.KubernetesWorkloadResourceInstance
	posts     []v0.KubernetesWorkloadResourceInstance
	patches   []v0.KubernetesWorkloadResourceInstance
	woken     bool
}

// kubeJSON returns a stored object definition.
func kubeJSON(object map[string]interface{}) *datatypes.JSON {
	// marshal the object the resource row stores
	body, err := json.Marshal(object)
	if err != nil {
		panic(err)
	}
	value := datatypes.JSON(body)
	return &value
}

// apiObjects copies rows into the response data slice.
func apiObjects[T any](items []T) []apiserver_lib.Object {
	// copy each row so the response data is non-nil
	objects := make([]apiserver_lib.Object, len(items))
	for i := range items {
		objects[i] = items[i]
	}
	return objects
}

// writeOne writes one API object.
func writeOne(t *testing.T, w http.ResponseWriter, status int, data apiserver_lib.Object) {
	// encode the object the client reads back
	machinetest.WriteResponse(t, w, status, []apiserver_lib.Object{data})
}

// serve returns a reconciler bound to the fixture.
func (f *portFixture) serve(t *testing.T) *controller.Reconciler {
	t.Helper()

	// answer the reads and record the writes
	api := machinetest.NewAPIStub(t)
	api.Mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == v0.PathGatewayInstances:
			machinetest.WriteResponse(t, w, http.StatusOK, apiObjects(f.gateways))
		case r.Method == http.MethodGet && r.URL.Path == v0.PathGatewayTcpPorts:
			machinetest.WriteResponse(t, w, http.StatusOK, apiObjects(f.tcpPorts[r.URL.Query().Get("gatewaydefinitionid")]))
		case r.Method == http.MethodGet && r.URL.Path == v0.PathGatewayHttpPorts:
			httpPort := v0.GatewayHttpPort{
				Port:       util.Ptr(80),
				TLSEnabled: util.Ptr(false),
			}
			machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{httpPort})
		case r.Method == http.MethodGet && r.URL.Path == v0.PathKubernetesWorkloadResourceInstances:
			machinetest.WriteResponse(t, w, http.StatusOK, apiObjects(f.resources))
		case r.Method == http.MethodPost && r.URL.Path == v0.PathKubernetesWorkloadResourceInstances:
			body, _ := io.ReadAll(r.Body)
			var object v0.KubernetesWorkloadResourceInstance
			require.NoError(t, json.Unmarshal(body, &object))
			f.posts = append(f.posts, object)
			writeOne(t, w, http.StatusCreated, object)
		case r.Method == http.MethodPatch && len(r.URL.Path) > len(v0.PathKubernetesWorkloadResourceInstances) && r.URL.Path[:len(v0.PathKubernetesWorkloadResourceInstances)] == v0.PathKubernetesWorkloadResourceInstances:
			body, _ := io.ReadAll(r.Body)
			var object v0.KubernetesWorkloadResourceInstance
			require.NoError(t, json.Unmarshal(body, &object))
			f.patches = append(f.patches, object)
			writeOne(t, w, http.StatusOK, object)
		case r.Method == http.MethodPatch && len(r.URL.Path) > len(v0.PathKubernetesWorkloadInstances) && r.URL.Path[:len(v0.PathKubernetesWorkloadInstances)] == v0.PathKubernetesWorkloadInstances:
			f.woken = true
			writeOne(t, w, http.StatusOK, v0.KubernetesWorkloadInstance{Reconciliation: v0.Reconciliation{Reconciled: util.Ptr(false)}})
		default:
			http.Error(w, r.Method+" "+r.URL.RequestURI(), http.StatusNotFound)
		}
	})
	return &controller.Reconciler{APIServer: api.Addr, APIClient: api.Client}
}

// objectKind returns the kind stored on a resource row.
func objectKind(t *testing.T, definition *datatypes.JSON) (string, map[string]interface{}) {
	t.Helper()

	// decode the stored definition
	require.NotNil(t, definition)
	object, err := util.UnmarshalJSON(*definition)
	require.NoError(t, err)
	return object["kind"].(string), object
}

// listenerNames returns the listener names on a global configuration.
func listenerNames(t *testing.T, object map[string]interface{}) []string {
	t.Helper()

	// read each listener name
	listeners, _, err := unstructured.NestedSlice(object, "spec", "listeners")
	require.NoError(t, err)
	names := make([]string, 0, len(listeners))
	for _, listener := range listeners {
		names = append(names, listener.(map[string]interface{})["name"].(string))
	}
	return names
}

// TestConfirmGatewayPortsExposedRebuildsRuntimePorts covers a shared listener update.
func TestConfirmGatewayPortsExposedRebuildsRuntimePorts(t *testing.T) {
	// serve two live gateways and one that is deleting
	deleting := time.Now().UTC()
	fixture := &portFixture{
		gateways: []v0.GatewayInstance{
			{Common: v0.Common{ID: util.Ptr(uint(1))}, GatewayDefinitionID: util.Ptr(uint(1))},
			{Common: v0.Common{ID: util.Ptr(uint(2))}, GatewayDefinitionID: util.Ptr(uint(2))},
			{
				Common:              v0.Common{ID: util.Ptr(uint(3))},
				Reconciliation:      v0.Reconciliation{DeletionScheduled: &deleting},
				GatewayDefinitionID: util.Ptr(uint(3)),
			},
		},
		tcpPorts: map[string][]v0.GatewayTcpPort{
			"1": {{Port: util.Ptr(5432)}},
			"2": {{Port: util.Ptr(443)}},
			"3": {{Port: util.Ptr(9000)}},
		},
		resources: []v0.KubernetesWorkloadResourceInstance{{
			Common: v0.Common{ID: util.Ptr(uint(11))},
			JSONDefinition: kubeJSON(map[string]interface{}{
				"apiVersion": "k8s.nginx.org/v1",
				"kind":       "GlobalConfiguration",
				"metadata":   map[string]interface{}{"name": "nginx-ingress", "namespace": "nukleros-ingress-system"},
				"spec": map[string]interface{}{
					"listeners": []interface{}{
						map[string]interface{}{"name": "tcp-9000", "port": 9000, "protocol": "TCP"},
						map[string]interface{}{"name": "tcp-1111", "port": 1111, "protocol": "TCP"},
					},
				},
			}),
		}},
	}
	reconciler := fixture.serve(t)
	runtime := &v0.KubernetesRuntimeInstance{
		Common:                      v0.Common{ID: util.Ptr(uint(5))},
		GatewayControllerInstanceID: util.Ptr(uint(9)),
	}
	gateway := &v0.GatewayInstance{
		Common:              v0.Common{ID: util.Ptr(uint(1))},
		GatewayDefinitionID: util.Ptr(uint(1)),
	}

	// replace the listeners with the live gateways' ports
	err := confirmGatewayPortsExposed(reconciler, gateway, runtime, nil, nil)
	require.NoError(t, err)
	require.True(t, fixture.woken)
	var listeners []string
	for _, patch := range fixture.patches {
		kind, object := objectKind(t, patch.JSONDefinition)
		if kind == "GlobalConfiguration" {
			listeners = listenerNames(t, object)
		}
	}
	require.Equal(t, []string{"tcp-443", "tcp-5432"}, listeners)
	require.Len(t, fixture.posts, 2)
	var sawService, sawController bool
	for _, post := range fixture.posts {
		kind, object := objectKind(t, post.JSONDefinition)
		switch kind {
		case "Service":
			sawService = true
			selector, _, err := unstructured.NestedStringMap(object, "spec", "selector")
			require.NoError(t, err)
			require.Equal(t, "nginx-tcp", selector["app"])
		case "Deployment":
			sawController = true
			containers, _, err := unstructured.NestedSlice(object, "spec", "template", "spec", "containers")
			require.NoError(t, err)
			container := containers[0].(map[string]interface{})
			var args []string
			for _, arg := range container["args"].([]interface{}) {
				args = append(args, arg.(string))
			}
			require.Contains(t, args, "-enable-custom-resources")
			require.Contains(t, args, "-ingress-class=nginx-tcp")
			require.Len(t, container["ports"].([]interface{}), 2)
		}
	}
	require.True(t, sawService)
	require.True(t, sawController)
}

// TestConfirmGatewayPortsExposedDeletesLastPorts covers an empty tcp port list.
func TestConfirmGatewayPortsExposedDeletesLastPorts(t *testing.T) {
	// serve a deleting gateway as the only tcp user
	deleting := time.Now().UTC()
	fixture := &portFixture{
		gateways: []v0.GatewayInstance{{
			Common:              v0.Common{ID: util.Ptr(uint(1))},
			Reconciliation:      v0.Reconciliation{DeletionScheduled: &deleting},
			GatewayDefinitionID: util.Ptr(uint(1)),
		}},
		tcpPorts: map[string][]v0.GatewayTcpPort{
			"1": {{Port: util.Ptr(9000)}},
		},
		resources: []v0.KubernetesWorkloadResourceInstance{
			{
				Common: v0.Common{ID: util.Ptr(uint(12))},
				JSONDefinition: kubeJSON(map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Service",
					"metadata":   map[string]interface{}{"name": "nginx-tcp", "namespace": "nukleros-ingress-system"},
					"spec": map[string]interface{}{
						"selector": map[string]interface{}{"app": "nginx-tcp"},
						"ports":    []interface{}{map[string]interface{}{"name": "tcp-9000", "port": 9000, "protocol": "TCP"}},
					},
				}),
			},
			{
				Common: v0.Common{ID: util.Ptr(uint(13))},
				JSONDefinition: kubeJSON(map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata":   map[string]interface{}{"name": "nginx-tcp", "namespace": "nukleros-ingress-system"},
					"spec":       map[string]interface{}{"replicas": 1},
				}),
			},
		},
	}
	reconciler := fixture.serve(t)
	runtime := &v0.KubernetesRuntimeInstance{
		Common:                      v0.Common{ID: util.Ptr(uint(5))},
		GatewayControllerInstanceID: util.Ptr(uint(9)),
	}
	gateway := &v0.GatewayInstance{
		Common:              v0.Common{ID: util.Ptr(uint(1))},
		GatewayDefinitionID: util.Ptr(uint(1)),
	}

	// schedule the service and controller for deletion
	err := confirmGatewayPortsExposed(reconciler, gateway, runtime, nil, nil)
	require.NoError(t, err)
	require.Empty(t, fixture.posts)
	require.True(t, fixture.woken)
	require.Len(t, fixture.patches, 2)
	for _, patch := range fixture.patches {
		require.NotNil(t, patch.ScheduledForDeletion)
	}
}

// storedController returns the tcp controller the reconcile would render.
func storedController(t *testing.T, ports []int) *datatypes.JSON {
	t.Helper()

	// render the same manifest the reconcile stores
	rendered, err := tcpControllerYaml(ports)
	require.NoError(t, err)
	object, err := util.UnstructuredFromYaml(rendered)
	require.NoError(t, err)
	return kubeJSON(object.Object)
}

// TestConfirmGatewayPortsExposedSkipsMatchingObjects covers an unchanged port set.
func TestConfirmGatewayPortsExposedSkipsMatchingObjects(t *testing.T) {
	// store the listeners, service ports, and controller those ports already produce
	ports := []int{443, 5432}
	fixture := &portFixture{
		gateways: []v0.GatewayInstance{
			{Common: v0.Common{ID: util.Ptr(uint(1))}, GatewayDefinitionID: util.Ptr(uint(1))},
			{Common: v0.Common{ID: util.Ptr(uint(2))}, GatewayDefinitionID: util.Ptr(uint(2))},
		},
		tcpPorts: map[string][]v0.GatewayTcpPort{
			"1": {{Port: util.Ptr(5432)}},
			"2": {{Port: util.Ptr(443)}},
		},
		resources: []v0.KubernetesWorkloadResourceInstance{
			{
				Common: v0.Common{ID: util.Ptr(uint(11))},
				JSONDefinition: kubeJSON(map[string]interface{}{
					"apiVersion": "k8s.nginx.org/v1",
					"kind":       "GlobalConfiguration",
					"metadata":   map[string]interface{}{"name": "nginx-ingress", "namespace": "nukleros-ingress-system"},
					"spec":       map[string]interface{}{"listeners": tcpListeners(ports)},
				}),
			},
			{
				Common: v0.Common{ID: util.Ptr(uint(12))},
				JSONDefinition: kubeJSON(map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Service",
					"metadata":   map[string]interface{}{"name": "nginx-tcp", "namespace": "nukleros-ingress-system"},
					"spec":       map[string]interface{}{"ports": tcpServicePorts(ports)},
				}),
			},
			{
				Common:         v0.Common{ID: util.Ptr(uint(13))},
				JSONDefinition: storedController(t, ports),
			},
		},
	}
	reconciler := fixture.serve(t)
	runtime := &v0.KubernetesRuntimeInstance{
		Common:                      v0.Common{ID: util.Ptr(uint(5))},
		GatewayControllerInstanceID: util.Ptr(uint(9)),
	}
	gateway := &v0.GatewayInstance{
		Common:              v0.Common{ID: util.Ptr(uint(1))},
		GatewayDefinitionID: util.Ptr(uint(1)),
	}

	// leave the stored objects and the controller workload alone
	err := confirmGatewayPortsExposed(reconciler, gateway, runtime, nil, nil)
	require.NoError(t, err)
	require.Empty(t, fixture.posts)
	require.Empty(t, fixture.patches)
	require.False(t, fixture.woken)
}
