package gateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

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

// workloadObject stores one workload resource.
func workloadObject(id uint, kind, name string) v0.KubernetesWorkloadResourceInstance {
	return v0.KubernetesWorkloadResourceInstance{
		Common: v0.Common{ID: util.Ptr(id)},
		JSONDefinition: kubeJSON(map[string]interface{}{
			"apiVersion": "v1",
			"kind":       kind,
			"metadata":   map[string]interface{}{"name": name},
		}),
	}
}

// TestServiceLookupErrUsesTheMissingResourceError reports a missing resource and leaves a kind failure alone.
func TestServiceLookupErrUsesTheMissingResourceError(t *testing.T) {
	// a missing name is the missing-resource error
	stored := []v0.KubernetesWorkloadResourceInstance{workloadObject(1, "Service", "other")}
	_, missing := workload_util.GetKubernetesWorkloadResourceInstance(&stored, "Service", "echo")
	require.ErrorIs(t, missing, workload_util.ErrKubernetesWorkloadResourceInstanceNotFound)
	require.ErrorIs(t, serviceLookupErr(missing), errGatewayServiceMissing)

	// a document with no kind is not a missing resource
	broken := []v0.KubernetesWorkloadResourceInstance{{
		JSONDefinition: kubeJSON(map[string]interface{}{"apiVersion": "v1"}),
	}}
	_, kindErr := workload_util.GetKubernetesWorkloadResourceInstancesByKind(&broken, "Service")
	require.Error(t, kindErr)
	require.False(t, errors.Is(serviceLookupErr(kindErr), errGatewayServiceMissing))
}

// TestV0GatewayInstanceDeletedSchedulesOwnedResourcesWithoutService schedules the ingress, certificate, and issuer when the service read fails.
func TestV0GatewayInstanceDeletedSchedulesOwnedResourcesWithoutService(t *testing.T) {
	cases := []struct {
		name    string
		extra   []v0.KubernetesWorkloadResourceInstance
		wantErr bool
	}{
		{name: "missing service"},
		{
			name: "renamed service",
			extra: []v0.KubernetesWorkloadResourceInstance{
				workloadObject(24, "Service", "echo-renamed"),
			},
		},
		{
			name: "duplicate service",
			extra: []v0.KubernetesWorkloadResourceInstance{
				workloadObject(25, "Service", "echo"),
				workloadObject(26, "Service", "echo"),
			},
			wantErr: true,
		},
		{
			name: "duplicate certificate",
			extra: []v0.KubernetesWorkloadResourceInstance{
				workloadObject(28, "Certificate", "other-cert"),
			},
			wantErr: true,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			// keep the workload and the gateway definitions, and leave the named service unread
			stored := []v0.KubernetesWorkloadResourceInstance{
				workloadObject(21, "Ingress", "echo-threeport-test"),
				workloadObject(22, "Certificate", "example-com"),
				workloadObject(23, "Deployment", "echo"),
				workloadObject(27, "Issuer", "example-com"),
			}
			stored = append(stored, testCase.extra...)
			definitions := []v0.KubernetesWorkloadResourceDefinition{
				{
					JSONDefinition: kubeJSON(map[string]interface{}{
						"apiVersion": "networking.k8s.io/v1",
						"kind":       "Ingress",
						"metadata":   map[string]interface{}{"name": "echo-threeport-test"},
					}),
				},
				{
					JSONDefinition: kubeJSON(map[string]interface{}{
						"apiVersion": "cert-manager.io/v1",
						"kind":       "Certificate",
						"metadata":   map[string]interface{}{"name": "placeholder"},
					}),
				},
				{
					JSONDefinition: kubeJSON(map[string]interface{}{
						"apiVersion": "cert-manager.io/v1",
						"kind":       "Issuer",
						"metadata":   map[string]interface{}{"name": "placeholder"},
					}),
				},
			}
			var patches []v0.KubernetesWorkloadResourceInstance
			var woken bool
			server := machinetest.NewAPIStub(t)
			server.Mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodGet && r.URL.Path == v0.PathKubernetesRuntimeInstances+"/4":
					writeOne(t, w, http.StatusOK, v0.KubernetesRuntimeInstance{Common: v0.Common{ID: util.Ptr(uint(4))}})
				case r.Method == http.MethodGet && r.URL.Path == v0.PathKubernetesWorkloadResourceInstances:
					if r.URL.Query().Get("kubernetesworkloadinstanceid") != "7" {
						t.Errorf("resource query %s", r.URL.RawQuery)
						http.Error(w, "query", http.StatusInternalServerError)
						return
					}
					machinetest.WriteResponse(t, w, http.StatusOK, apiObjects(stored))
				case r.Method == http.MethodGet && r.URL.Path == v0.PathGatewayDefinitions+"/3":
					writeOne(t, w, http.StatusOK, v0.GatewayDefinition{
						Common:                         v0.Common{ID: util.Ptr(uint(3))},
						ServiceName:                    util.Ptr("echo"),
						KubernetesWorkloadDefinitionID: util.Ptr(uint(8)),
					})
				case r.Method == http.MethodGet && r.URL.Path == v0.PathKubernetesWorkloadInstances+"/7":
					writeOne(t, w, http.StatusOK, v0.KubernetesWorkloadInstance{Common: v0.Common{ID: util.Ptr(uint(7))}})
				case r.Method == http.MethodGet && r.URL.Path == v0.PathKubernetesWorkloadResourceDefinitions:
					if r.URL.Query().Get("kubernetesworkloaddefinitionid") != "8" {
						t.Errorf("definition query %s", r.URL.RawQuery)
						http.Error(w, "query", http.StatusInternalServerError)
						return
					}
					machinetest.WriteResponse(t, w, http.StatusOK, apiObjects(definitions))
				case r.Method == http.MethodPatch && strings.HasPrefix(r.URL.Path, v0.PathKubernetesWorkloadResourceInstances+"/"):
					body, readErr := io.ReadAll(r.Body)
					if readErr != nil {
						t.Errorf("read patch: %v", readErr)
						http.Error(w, "read", http.StatusInternalServerError)
						return
					}
					var object v0.KubernetesWorkloadResourceInstance
					if unmarshalErr := json.Unmarshal(body, &object); unmarshalErr != nil {
						t.Errorf("decode patch: %v", unmarshalErr)
						http.Error(w, "decode", http.StatusInternalServerError)
						return
					}
					idText := strings.TrimPrefix(r.URL.Path, v0.PathKubernetesWorkloadResourceInstances+"/")
					id, convErr := strconv.Atoi(idText)
					if convErr != nil {
						t.Errorf("patch id %s", idText)
						http.Error(w, "id", http.StatusInternalServerError)
						return
					}
					object.ID = util.Ptr(uint(id))
					patches = append(patches, object)
					writeOne(t, w, http.StatusOK, object)
				case r.Method == http.MethodPatch && r.URL.Path == v0.PathKubernetesWorkloadInstances+"/7":
					woken = true
					writeOne(t, w, http.StatusOK, v0.KubernetesWorkloadInstance{Common: v0.Common{ID: util.Ptr(uint(7))}})
				default:
					t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
					http.Error(w, "unexpected", http.StatusInternalServerError)
				}
			})
			reconciler := &controller.Reconciler{
				APIServer: server.Addr,
				APIClient: server.Client,
			}
			now := time.Now()
			gateway := &v0.GatewayInstance{
				Common:                       v0.Common{ID: util.Ptr(uint(1))},
				Reconciliation:               v0.Reconciliation{DeletionScheduled: &now},
				GatewayDefinitionID:          util.Ptr(uint(3)),
				KubernetesRuntimeInstanceID:  util.Ptr(uint(4)),
				KubernetesWorkloadInstanceID: util.Ptr(uint(7)),
			}

			// delete the gateway while its service cannot be read
			_, err := v0GatewayInstanceDeleted(reconciler, gateway, nil)
			// a lookup failure schedules nothing
			if testCase.wantErr {
				require.Error(t, err)
				require.Empty(t, patches)
				require.False(t, woken)
				return
			}
			require.NoError(t, err)

			// the delete schedules the ingress, the certificate, and the issuer
			require.True(t, woken)
			require.Len(t, patches, 3)
			require.ElementsMatch(t, []uint{21, 22, 27}, []uint{*patches[0].ID, *patches[1].ID, *patches[2].ID})
			for _, patch := range patches {
				require.NotNil(t, patch.ScheduledForDeletion)
				require.NotNil(t, patch.Reconciled)
				require.False(t, *patch.Reconciled)
			}
		})
	}
}
