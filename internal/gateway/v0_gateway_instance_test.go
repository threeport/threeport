package gateway

import (
	"fmt"
	"net/http"
	"testing"

	_ "embed"

	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/threeport/threeport/internal/gateway/assets"
	workload_util "github.com/threeport/threeport/internal/kubernetes-workload/util"
	"github.com/threeport/threeport/internal/machinetest"
	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

//go:embed assets/service-test.yaml
var serviceTest []byte

// TestConfigureIssuerKindUsesSelfSigned covers a kind runtime.
func TestConfigureIssuerKindUsesSelfSigned(t *testing.T) {
	// serve the issuer document and a kind runtime definition
	api := machinetest.NewAPIStub(t)
	r := &controller.Reconciler{APIClient: api.Client, APIServer: api.Addr}
	issuer := resourceDefinition(t, assets.Issuer)
	api.Mux.HandleFunc(v0.PathKubernetesWorkloadResourceDefinitions, func(w http.ResponseWriter, req *http.Request) {
		machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{issuer})
	})
	kind := v0.KubernetesRuntimeInfraProviderKind
	runtimeDef := v0.KubernetesRuntimeDefinition{InfraProvider: &kind}
	api.Mux.HandleFunc(fmt.Sprintf("%s/9", v0.PathKubernetesRuntimeDefinitions), func(w http.ResponseWriter, req *http.Request) {
		machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{runtimeDef})
	})
	gatewayDef := &v0.GatewayDefinition{KubernetesWorkloadDefinitionID: util.Ptr(uint(2))}
	runtime := &v0.KubernetesRuntimeInstance{KubernetesRuntimeDefinitionID: util.Ptr(uint(9))}
	domain := &v0.DomainNameDefinition{Definition: v0.Definition{Name: util.Ptr("example")}, Domain: util.Ptr("example.com")}

	// the kind path returns before the region lookup
	got, err := configureIssuer(r, gatewayDef, nil, runtime, domain)
	require.NoError(t, err)
	require.Contains(t, string(*got), "selfSigned")
	require.NotContains(t, string(*got), "acme")
}

// TestConfigureCertificateSetsDnsName covers the certificate rewrite.
func TestConfigureCertificateSetsDnsName(t *testing.T) {
	// serve one certificate document
	api := machinetest.NewAPIStub(t)
	r := &controller.Reconciler{APIClient: api.Client, APIServer: api.Addr}
	certificate := resourceDefinition(t, assets.Certificate)
	api.Mux.HandleFunc(v0.PathKubernetesWorkloadResourceDefinitions, func(w http.ResponseWriter, req *http.Request) {
		machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{certificate})
	})
	gatewayDef := &v0.GatewayDefinition{KubernetesWorkloadDefinitionID: util.Ptr(uint(2))}
	domain := &v0.DomainNameDefinition{
		Definition: v0.Definition{Name: util.Ptr("example")},
		Domain:     util.Ptr("example.com"),
	}

	// the certificate names the domain and the issuer
	got, err := configureCertificate(r, gatewayDef, nil, nil, domain)
	require.NoError(t, err)
	require.Contains(t, string(*got), "example.com")
	require.Contains(t, string(*got), `"name":"example"`)
}

// TestConfigureGatewayManifestsSetsBackends covers both port kinds.
func TestConfigureGatewayManifestsSetsBackends(t *testing.T) {
	// serve the service, the gateway documents, and one port of each kind
	api := machinetest.NewAPIStub(t)
	r := &controller.Reconciler{APIClient: api.Client, APIServer: api.Addr}
	service := resourceInstance(t, serviceTest)
	api.Mux.HandleFunc(v0.PathKubernetesWorkloadResourceInstances, func(w http.ResponseWriter, req *http.Request) {
		machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{service})
	})
	gatewayDef := &v0.GatewayDefinition{
		Common:                         v0.Common{ID: util.Ptr(uint(1))},
		Definition:                     v0.Definition{Name: util.Ptr("example-com")},
		DomainNameDefinitionID:         util.Ptr(uint(4)),
		KubernetesWorkloadDefinitionID: util.Ptr(uint(2)),
	}
	// name the documents the install step writes
	ingress := namedDefinition(t, assets.Ingress, getIngressName(gatewayDef, "example.com"))
	transport := namedDefinition(t, assets.TransportServer, fmt.Sprintf("%s-%d", *gatewayDef.Name, 443))
	api.Mux.HandleFunc(v0.PathKubernetesWorkloadResourceDefinitions, func(w http.ResponseWriter, req *http.Request) {
		machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{ingress, transport})
	})
	httpPort := v0.GatewayHttpPort{Port: util.Ptr(80)}
	tcpPort := v0.GatewayTcpPort{Port: util.Ptr(443)}
	api.Mux.HandleFunc(v0.PathGatewayHttpPorts, func(w http.ResponseWriter, req *http.Request) {
		machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{httpPort})
	})
	api.Mux.HandleFunc(v0.PathGatewayTcpPorts, func(w http.ResponseWriter, req *http.Request) {
		machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{tcpPort})
	})
	domain := v0.DomainNameDefinition{Definition: v0.Definition{Name: util.Ptr("example")}, Domain: util.Ptr("example.com"), AdminEmail: util.Ptr("admin@example.com")}
	api.Mux.HandleFunc(fmt.Sprintf("%s/4", v0.PathDomainNameDefinitions), func(w http.ResponseWriter, req *http.Request) {
		machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{domain})
	})
	workload := &v0.KubernetesWorkloadInstance{Common: v0.Common{ID: util.Ptr(uint(3))}}

	// each document points at the service and its port
	got, err := configureGatewayManifests(r, gatewayDef, workload, nil)
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Contains(t, string(*got[0]), `"name":"app"`)
	require.Contains(t, string(*got[1]), `"service":"app"`)
}

// namedDefinition stores an embedded manifest under the name the install step writes.
func namedDefinition(t *testing.T, asset []byte, name string) v0.KubernetesWorkloadResourceDefinition {
	// set the name, then store the document the same way as a plain asset
	decoded, err := util.UnstructuredFromYaml(asset)
	require.NoError(t, err)
	require.NoError(t, unstructured.SetNestedField(decoded.Object, name, "metadata", "name"))
	raw, err := decoded.MarshalJSON()
	require.NoError(t, err)
	body := datatypes.JSON(raw)
	return v0.KubernetesWorkloadResourceDefinition{JSONDefinition: &body}
}

// resourceDefinition stores an embedded manifest as a definition.
func resourceDefinition(t *testing.T, asset []byte) v0.KubernetesWorkloadResourceDefinition {
	// convert the yaml into the stored json document
	body := manifestJSON(t, asset)
	return v0.KubernetesWorkloadResourceDefinition{JSONDefinition: &body}
}

// resourceInstance stores an embedded manifest as an instance.
func resourceInstance(t *testing.T, asset []byte) v0.KubernetesWorkloadResourceInstance {
	// convert the yaml into the stored json document
	body := manifestJSON(t, asset)
	return v0.KubernetesWorkloadResourceInstance{JSONDefinition: &body}
}

// manifestJSON converts an embedded yaml manifest to json.
func manifestJSON(t *testing.T, asset []byte) datatypes.JSON {
	// decode the same way the reconciler does
	decoded, err := util.UnstructuredFromYaml(asset)
	require.NoError(t, err)
	raw, err := decoded.MarshalJSON()
	require.NoError(t, err)
	return datatypes.JSON(raw)
}

// TestChangedGatewayResourceInstancesMatchesEachIngress covers two ingresses of the same kind.
func TestChangedGatewayResourceInstancesMatchesEachIngress(t *testing.T) {
	// store one unchanged ingress and one whose body differs
	httpID := uint(10)
	tlsID := uint(11)
	stored := []v0.KubernetesWorkloadResourceInstance{
		{
			Common: v0.Common{ID: &httpID},
			JSONDefinition: kubeJSON(map[string]interface{}{
				"kind": "Ingress",
				"metadata": map[string]interface{}{
					"name":      "echo-80",
					"namespace": "gateway-echo",
				},
				"spec": map[string]interface{}{"host": "echo"},
			}),
		},
		{
			Common: v0.Common{ID: &tlsID},
			JSONDefinition: kubeJSON(map[string]interface{}{
				"kind": "Ingress",
				"metadata": map[string]interface{}{
					"name":      "echo-443",
					"namespace": "gateway-echo",
				},
				"spec": map[string]interface{}{"host": "echo", "tls": false},
			}),
		},
	}
	updated := []v0.KubernetesWorkloadResourceInstance{
		{
			JSONDefinition: kubeJSON(map[string]interface{}{
				"kind":     "Ingress",
				"metadata": map[string]interface{}{"name": "echo-80"},
				"spec":     map[string]interface{}{"host": "echo"},
			}),
		},
		{
			JSONDefinition: kubeJSON(map[string]interface{}{
				"kind":     "Ingress",
				"metadata": map[string]interface{}{"name": "echo-443"},
				"spec":     map[string]interface{}{"host": "echo", "tls": true},
			}),
		},
		{
			JSONDefinition: kubeJSON(map[string]interface{}{
				"kind":     "ConfigMap",
				"metadata": map[string]interface{}{"name": "ignored"},
			}),
		},
	}

	// compare the configured objects to the stored ones
	changed, err := changedGatewayResourceInstances([]string{"Ingress"}, &stored, &updated)
	require.NoError(t, err)

	// expect only the tls ingress, marked for reconcile
	require.Len(t, changed, 1)
	require.Equal(t, tlsID, *changed[0].ID)
	require.NotNil(t, changed[0].Reconciled)
	require.False(t, *changed[0].Reconciled)
	require.JSONEq(t, string(*updated[1].JSONDefinition), string(*changed[0].JSONDefinition))
}

// TestResourcesByKindReturnsEveryIngress covers a kind that has two objects.
func TestResourcesByKindReturnsEveryIngress(t *testing.T) {
	// store two ingresses and one service
	stored := []v0.KubernetesWorkloadResourceInstance{
		{JSONDefinition: kubeJSON(map[string]interface{}{
			"kind":     "Ingress",
			"metadata": map[string]interface{}{"name": "echo-80"},
		})},
		{JSONDefinition: kubeJSON(map[string]interface{}{
			"kind":     "Service",
			"metadata": map[string]interface{}{"name": "echo"},
		})},
		{JSONDefinition: kubeJSON(map[string]interface{}{
			"kind":     "Ingress",
			"metadata": map[string]interface{}{"name": "echo-443"},
		})},
	}

	// select the ingresses
	matches, err := workload_util.GetKubernetesWorkloadResourceInstancesByKind(&stored, "Ingress")
	require.NoError(t, err)

	// expect both ingresses and neither service
	require.Len(t, matches, 2)
	_, firstName, err := resourceKindAndName(matches[0].JSONDefinition)
	require.NoError(t, err)
	_, secondName, err := resourceKindAndName(matches[1].JSONDefinition)
	require.NoError(t, err)
	require.Equal(t, "echo-80", firstName)
	require.Equal(t, "echo-443", secondName)
}

func TestGatewayResourcesToDeleteSkipsUserIngress(t *testing.T) {
	// a user ingress of the same kind is left alone
	user := v0.KubernetesWorkloadResourceInstance{
		JSONDefinition: kubeJSON(map[string]interface{}{
			"kind":     "Ingress",
			"metadata": map[string]interface{}{"name": "user"},
		}),
	}
	owned := v0.KubernetesWorkloadResourceInstance{
		JSONDefinition: kubeJSON(map[string]interface{}{
			"kind":     "Ingress",
			"metadata": map[string]interface{}{"name": "echo-threeport-test"},
		}),
	}
	stored := []v0.KubernetesWorkloadResourceInstance{user, owned}
	rendered := []v0.KubernetesWorkloadResourceInstance{owned}

	got, err := gatewayResourcesToDelete(&stored, &rendered)
	require.NoError(t, err)
	require.Len(t, got, 1)
	_, name, err := resourceKindAndName(got[0].JSONDefinition)
	require.NoError(t, err)
	require.Equal(t, "echo-threeport-test", name)
}
