package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/iancoleman/strcase"
	"github.com/stretchr/testify/require"
	kubeerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	api "github.com/threeport/threeport/pkg/api/v0"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

const (
	echoServiceName           = "echo"
	echoNamespace             = "gateway-echo"
	echoHost                  = "echo.threeport.test"
	otherHost                 = "other.threeport.test"
	echoBody                  = "gateway-ok"
	echoTcpPort               = 5432
	otherTcpPort              = 5433
	sharedConfigName          = "nginx-ingress"
	ingressComponentName      = "ingress"
	certificatesComponentName = "certificates"
	ingressNamespace          = "nukleros-ingress-system"
	nginxIngressService       = "nginx-ingress-nodeport"
	reconcileAttempts         = 180
	certificateAttempts       = 300
)

const echoWorkload = `
apiVersion: v1
kind: Namespace
metadata:
  name: gateway-echo
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: echo
  namespace: gateway-echo
spec:
  replicas: 1
  selector:
    matchLabels:
      app: echo
  template:
    metadata:
      labels:
        app: echo
    spec:
      containers:
      - name: echo
        image: hashicorp/http-echo:1.0.0
        args: ["-text=gateway-ok"]
        ports:
        - containerPort: 5678
---
apiVersion: v1
kind: Service
metadata:
  name: echo
  namespace: gateway-echo
spec:
  selector:
    app: echo
  ports:
  - name: http
    port: 80
    targetPort: 5678
  - name: https
    port: 443
    targetPort: 5678
  - name: tcp-5432
    port: 5432
    targetPort: 5678
  - name: tcp-5433
    port: 5433
    targetPort: 5678
`

// TestGatewayIngressServesHTTPAndHTTPS checks two hosts on one echo service.
// The shared ingress install stays at one until the last gateway is deleted.
func TestGatewayIngressServesHTTPAndHTTPS(t *testing.T) {
	// open the control plane api and the kind runtime it records
	apiClient, endpoint, encryptionKey := threeportClient(t)
	runtime := controlPlaneRuntime(t, apiClient, endpoint)
	kubeClient, _, err := kube.GetClient(runtime, false, apiClient, endpoint, encryptionKey)
	require.NoError(t, err)

	// create two hosts that share the echo service
	workload := createEchoWorkload(t, apiClient, endpoint, runtime)
	waitForEchoService(t, apiClient, endpoint, workload)
	echoDomain := createEchoDomain(t, apiClient, endpoint, "echo", echoHost)
	otherDomain := createEchoDomain(t, apiClient, endpoint, "other", otherHost)
	echoDefinition := createEchoGatewayDefinition(t, apiClient, endpoint, "echo", echoDomain, echoTcpPort)
	otherDefinition := createEchoGatewayDefinition(t, apiClient, endpoint, "other", otherDomain, otherTcpPort)
	waitForGatewayDefinition(t, apiClient, endpoint, echoDefinition)
	waitForGatewayDefinition(t, apiClient, endpoint, otherDefinition)

	// reconcile the first host and record the shared listeners
	echoGateway := createEchoGatewayInstance(t, apiClient, endpoint, "echo-0", echoDefinition, workload, runtime)
	waitForGatewayInstance(t, apiClient, endpoint, echoGateway)
	sharedID := waitForSharedListeners(
		t,
		apiClient,
		endpoint,
		*runtime.ID,
		[]string{fmt.Sprintf("tcp-%d", echoTcpPort)},
		0,
	)
	waitForHostObjects(t, apiClient, endpoint, kubeClient, *workload.ID, "echo", echoHost, echoTcpPort)
	controllerID := gatewayControllerID(t, apiClient, endpoint, *runtime.ID)
	waitForSharedComponents(t, kubeClient, 1)

	// reconcile the second host onto the same shared listeners
	otherGateway := createEchoGatewayInstance(t, apiClient, endpoint, "other-0", otherDefinition, workload, runtime)
	waitForGatewayInstance(t, apiClient, endpoint, otherGateway)
	sharedID = waitForSharedListeners(
		t,
		apiClient,
		endpoint,
		*runtime.ID,
		[]string{
			fmt.Sprintf("tcp-%d", echoTcpPort),
			fmt.Sprintf("tcp-%d", otherTcpPort),
		},
		sharedID,
	)
	waitForHostObjects(t, apiClient, endpoint, kubeClient, *workload.ID, "other", otherHost, otherTcpPort)
	require.Equal(t, controllerID, gatewayControllerID(t, apiClient, endpoint, *runtime.ID))
	waitForSharedComponents(t, kubeClient, 1)

	// call both hosts over http and https
	ports := waitForIngressPorts(t, kubeClient)
	nodeIP := nodeInternalIP(t, kubeClient)
	callHost(t, kubeClient, nodeIP, ports, echoHost)
	callHost(t, kubeClient, nodeIP, ports, otherHost)

	// delete the second host and keep the first listener
	deleteGateway(t, apiClient, endpoint, otherGateway)
	waitForHostTornDown(t, apiClient, endpoint, kubeClient, *workload.ID, "other", otherHost, otherTcpPort)
	sharedID = waitForSharedListeners(
		t,
		apiClient,
		endpoint,
		*runtime.ID,
		[]string{fmt.Sprintf("tcp-%d", echoTcpPort)},
		sharedID,
	)
	callHost(t, kubeClient, nodeIP, ports, echoHost)
	require.Equal(t, controllerID, gatewayControllerID(t, apiClient, endpoint, *runtime.ID))
	waitForSharedComponents(t, kubeClient, 1)

	// delete the last gateway and remove the shared ingress install
	deleteGateway(t, apiClient, endpoint, echoGateway)
	waitForHostTornDown(t, apiClient, endpoint, kubeClient, *workload.ID, "echo", echoHost, echoTcpPort)
	waitForSharedControllerRemoved(t, apiClient, endpoint, kubeClient, *runtime.ID)
}

// createEchoDomain registers the host the ingress matches.
func createEchoDomain(t *testing.T, apiClient *http.Client, endpoint, name, host string) *api.DomainNameDefinition {
	t.Helper()

	// store the domain the gateway serves
	created, err := client.CreateDomainNameDefinition(apiClient, endpoint, &api.DomainNameDefinition{
		Definition: api.Definition{Name: util.Ptr(name)},
		Domain:     util.Ptr(host),
		Zone:       util.Ptr(host),
		AdminEmail: util.Ptr("no-reply@threeport.io"),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		retryDelete(t, name+" domain", func() error {
			_, err := client.DeleteDomainNameDefinition(apiClient, endpoint, *created.ID)
			return err
		})
	})
	return created
}

// createEchoWorkload installs the echo deployment and service.
func createEchoWorkload(
	t *testing.T,
	apiClient *http.Client,
	endpoint string,
	runtime *api.KubernetesRuntimeInstance,
) *api.KubernetesWorkloadInstance {
	t.Helper()

	// create the definition, then an instance on the control plane runtime
	definition, err := client.CreateKubernetesWorkloadDefinition(apiClient, endpoint, &api.KubernetesWorkloadDefinition{
		Definition:   api.Definition{Name: util.Ptr("echo")},
		YAMLDocument: util.Ptr(echoWorkload),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		retryDelete(t, "echo workload definition", func() error {
			_, err := client.DeleteKubernetesWorkloadDefinition(apiClient, endpoint, *definition.ID)
			return err
		})
	})
	created, err := client.CreateKubernetesWorkloadInstance(apiClient, endpoint, &api.KubernetesWorkloadInstance{
		Instance:                       api.Instance{Name: util.Ptr("echo-0")},
		KubernetesWorkloadDefinitionID: definition.ID,
		KubernetesRuntimeInstanceID:    runtime.ID,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		retryDelete(t, "echo workload instance", func() error {
			_, err := client.DeleteKubernetesWorkloadInstance(apiClient, endpoint, *created.ID)
			return err
		})
	})
	return created
}

// waitForEchoService returns when the workload has stored the echo service.
func waitForEchoService(t *testing.T, apiClient *http.Client, endpoint string, workload *api.KubernetesWorkloadInstance) {
	t.Helper()

	// read the workload until its service is stored
	err := util.Retry(reconcileAttempts, 2, func() error {
		stored, err := client.GetKubernetesWorkloadInstanceByID(apiClient, endpoint, *workload.ID)
		if err != nil {
			return err
		}
		if stored.Reconciled == nil || !*stored.Reconciled {
			return fmt.Errorf("echo workload is not reconciled")
		}
		resources, err := client.GetKubernetesWorkloadResourceInstancesByID(apiClient, endpoint, *workload.ID)
		if err != nil {
			return err
		}
		for _, resource := range *resources {
			if resource.JSONDefinition == nil {
				continue
			}
			body := string(*resource.JSONDefinition)
			if strings.Contains(body, "Service") && strings.Contains(body, echoServiceName) {
				return nil
			}
		}
		return fmt.Errorf("echo service is not stored")
	})
	require.NoError(t, err)
}

// createEchoGatewayDefinition registers http, https and one tcp port for the echo service.
func createEchoGatewayDefinition(
	t *testing.T,
	apiClient *http.Client,
	endpoint string,
	name string,
	domain *api.DomainNameDefinition,
	tcpPort int,
) *api.GatewayDefinition {
	t.Helper()

	// define ports 80 and 443 plus one tcp listener
	created, err := client.CreateGatewayDefinition(apiClient, endpoint, &api.GatewayDefinition{
		Definition:             api.Definition{Name: util.Ptr(name)},
		DomainNameDefinitionID: domain.ID,
		ServiceName:            util.Ptr(echoServiceName),
		HttpPorts: []*api.GatewayHttpPort{
			{Port: util.Ptr(80), TLSEnabled: util.Ptr(false), Path: util.Ptr("/")},
			{Port: util.Ptr(443), TLSEnabled: util.Ptr(true), Path: util.Ptr("/")},
		},
		TcpPorts: []*api.GatewayTcpPort{
			{Port: util.Ptr(tcpPort), TLSEnabled: util.Ptr(false)},
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		retryDelete(t, name+" gateway definition", func() error {
			_, err := client.DeleteGatewayDefinition(apiClient, endpoint, *created.ID)
			return err
		})
	})
	return created
}

// createEchoGatewayInstance deploys the gateway on the echo workload.
func createEchoGatewayInstance(
	t *testing.T,
	apiClient *http.Client,
	endpoint string,
	name string,
	definition *api.GatewayDefinition,
	workload *api.KubernetesWorkloadInstance,
	runtime *api.KubernetesRuntimeInstance,
) *api.GatewayInstance {
	t.Helper()

	// attach the gateway to the echo workload on the control plane runtime
	created, err := client.CreateGatewayInstance(apiClient, endpoint, &api.GatewayInstance{
		Instance:                     api.Instance{Name: util.Ptr(name)},
		GatewayDefinitionID:          definition.ID,
		KubernetesRuntimeInstanceID:  runtime.ID,
		KubernetesWorkloadInstanceID: workload.ID,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		retryDelete(t, name+" gateway instance", func() error {
			_, err := client.DeleteGatewayInstance(apiClient, endpoint, *created.ID)
			return err
		})
	})
	return created
}

// waitForCertificate returns the certificate cert-manager issued for the host.
func waitForCertificate(t *testing.T, kubeClient dynamic.Interface, host string) *unstructured.Unstructured {
	t.Helper()

	// read certificates until one for this host is ready
	var found *unstructured.Unstructured
	err := util.Retry(certificateAttempts, 2, func() error {
		list, err := kubeClient.Resource(schema.GroupVersionResource{
			Group: "cert-manager.io", Version: "v1", Resource: "certificates",
		}).Namespace(metav1.NamespaceAll).List(context.Background(), metav1.ListOptions{})
		if err != nil {
			return err
		}
		for _, item := range list.Items {
			dns, _, _ := unstructured.NestedStringSlice(item.Object, "spec", "dnsNames")
			if !containsString(dns, host) {
				continue
			}
			conditions, _, _ := unstructured.NestedSlice(item.Object, "status", "conditions")
			for _, condition := range conditions {
				entry, ok := condition.(map[string]interface{})
				if ok && entry["type"] == "Ready" && entry["status"] == "True" {
					match := item
					found = &match
					return nil
				}
			}
		}
		return fmt.Errorf("certificate for %s is not ready", host)
	})
	require.NoError(t, err)
	return found
}

// waitForIngressPorts returns the node ports nginx publishes for 80 and 443.
func waitForIngressPorts(t *testing.T, kubeClient dynamic.Interface) map[int64]int64 {
	t.Helper()

	// read the nodeport service until both ports are set
	ports := map[int64]int64{}
	err := util.Retry(certificateAttempts, 2, func() error {
		service, err := kubeClient.Resource(schema.GroupVersionResource{
			Version: "v1", Resource: "services",
		}).Namespace(ingressNamespace).Get(context.Background(), nginxIngressService, metav1.GetOptions{})
		if err != nil {
			return err
		}
		servicePorts, _, err := unstructured.NestedSlice(service.Object, "spec", "ports")
		if err != nil {
			return err
		}
		found := map[int64]int64{}
		for _, servicePort := range servicePorts {
			entry, ok := servicePort.(map[string]interface{})
			if !ok {
				continue
			}
			port, portOK := numberValue(entry["port"])
			nodePort, nodeOK := numberValue(entry["nodePort"])
			if portOK && nodeOK && (port == 80 || port == 443) && nodePort > 0 {
				found[port] = nodePort
			}
		}
		if found[80] == 0 || found[443] == 0 {
			return fmt.Errorf("ingress node ports are not ready")
		}
		ready, err := ingressEndpointsReady(kubeClient)
		if err != nil {
			return err
		}
		if !ready {
			return fmt.Errorf("ingress endpoints are not ready")
		}
		ports = found
		return nil
	})
	require.NoError(t, err)
	return ports
}

// nodeInternalIP returns the kind node's address on the docker bridge.
func nodeInternalIP(t *testing.T, kubeClient dynamic.Interface) string {
	t.Helper()

	// read the first node's internal address
	list, err := kubeClient.Resource(schema.GroupVersionResource{
		Version: "v1", Resource: "nodes",
	}).List(context.Background(), metav1.ListOptions{})
	require.NoError(t, err)
	require.NotEmpty(t, list.Items)
	addresses, _, err := unstructured.NestedSlice(list.Items[0].Object, "status", "addresses")
	require.NoError(t, err)
	for _, address := range addresses {
		entry, ok := address.(map[string]interface{})
		if ok && entry["type"] == "InternalIP" {
			ip, _ := entry["address"].(string)
			if ip != "" {
				return ip
			}
		}
	}
	t.Fatal("node has no internal ip")
	return ""
}

// issuedCertificate returns the certificate bytes cert-manager stored.
func issuedCertificate(t *testing.T, kubeClient dynamic.Interface, cert *unstructured.Unstructured) []byte {
	t.Helper()

	// read tls.crt from the certificate secret
	name, _, err := unstructured.NestedString(cert.Object, "spec", "secretName")
	require.NoError(t, err)
	secret, err := kubeClient.Resource(schema.GroupVersionResource{
		Version: "v1", Resource: "secrets",
	}).Namespace(cert.GetNamespace()).Get(context.Background(), name, metav1.GetOptions{})
	require.NoError(t, err)
	encoded, _, err := unstructured.NestedString(secret.Object, "data", "tls.crt")
	require.NoError(t, err)
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	require.NoError(t, err)
	block, _ := pem.Decode(decoded)
	require.NotNil(t, block)
	parsed, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	return parsed.Raw
}

// ingressEndpointsReady reports whether nginx has a ready backend.
func ingressEndpointsReady(kubeClient dynamic.Interface) (bool, error) {
	// read the service endpoints
	endpoints, err := kubeClient.Resource(schema.GroupVersionResource{
		Version: "v1", Resource: "endpoints",
	}).Namespace(ingressNamespace).Get(context.Background(), nginxIngressService, metav1.GetOptions{})
	if err != nil {
		return false, err
	}
	subsets, _, err := unstructured.NestedSlice(endpoints.Object, "subsets")
	if err != nil {
		return false, err
	}
	for _, subset := range subsets {
		entry, ok := subset.(map[string]interface{})
		if !ok {
			continue
		}
		addresses, _, _ := unstructured.NestedSlice(entry, "addresses")
		if len(addresses) > 0 {
			return true, nil
		}
	}
	return false, nil
}

// httpGet reads the body from the ingress, sending host as the Host header.
func httpGet(t *testing.T, rawURL, host string) string {
	t.Helper()

	// retry while kube-proxy programs the node port
	var body string
	err := util.Retry(30, 2, func() error {
		got, err := httpGetOnce(rawURL, host)
		if err != nil {
			return err
		}
		body = got
		return nil
	})
	require.NoError(t, err)
	return body
}

// httpGetOnce calls the node port once.
func httpGetOnce(rawURL, host string) (string, error) {
	// call the node port with the gateway host
	request, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	request.Host = host
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		return "", err
	}
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d: %s", response.StatusCode, payload)
	}
	return string(payload), nil
}

// tlsPeer returns the certificate nginx presents for host.
func tlsPeer(t *testing.T, address, host string) []byte {
	t.Helper()

	// retry while the https listener comes up
	var peer []byte
	err := util.Retry(30, 2, func() error {
		got, err := tlsPeerOnce(address, host)
		if err != nil {
			return err
		}
		peer = got
		return nil
	})
	require.NoError(t, err)
	return peer
}

// tlsPeerOnce handshakes once and returns the leaf.
func tlsPeerOnce(address, host string) ([]byte, error) {
	// handshake without trusting the system roots
	connection, err := tls.Dial("tcp", address, &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         host,
	})
	if err != nil {
		return nil, err
	}
	defer connection.Close()
	if len(connection.ConnectionState().PeerCertificates) == 0 {
		return nil, fmt.Errorf("nginx presented no certificate")
	}
	return connection.ConnectionState().PeerCertificates[0].Raw, nil
}

// gatewayObject is one per-host manifest the gateway stores and applies.
type gatewayObject struct {
	// The Kubernetes kind stored by threeport
	kind string

	// The object name
	name string

	// The cluster resource used to read the object
	gvr schema.GroupVersionResource

	// Whether the test reads the object from the cluster
	cluster bool
}

// waitForGatewayDefinition waits until the gateway definition is reconciled.
func waitForGatewayDefinition(t *testing.T, apiClient *http.Client, endpoint string, definition *api.GatewayDefinition) {
	t.Helper()

	// read the definition until reconcile finishes
	waitForReconciled(t, *definition.Name+" gateway definition", func() (*bool, error) {
		stored, err := client.GetGatewayDefinitionByID(apiClient, endpoint, *definition.ID)
		if err != nil {
			return nil, err
		}
		return stored.Reconciled, nil
	})
}

// waitForGatewayInstance waits until the gateway instance is reconciled.
func waitForGatewayInstance(t *testing.T, apiClient *http.Client, endpoint string, gateway *api.GatewayInstance) {
	t.Helper()

	// read the instance until reconcile finishes
	waitForReconciled(t, *gateway.Name+" gateway instance", func() (*bool, error) {
		stored, err := client.GetGatewayInstanceByID(apiClient, endpoint, *gateway.ID)
		if err != nil {
			return nil, err
		}
		return stored.Reconciled, nil
	})
}

// deleteGateway deletes the gateway and waits until threeport drops it.
func deleteGateway(t *testing.T, apiClient *http.Client, endpoint string, gateway *api.GatewayInstance) {
	t.Helper()

	// schedule deletion and wait until the row is gone
	_, err := client.DeleteGatewayInstance(apiClient, endpoint, *gateway.ID)
	require.NoError(t, err)
	err = util.Retry(reconcileAttempts, 2, func() error {
		_, err := client.GetGatewayInstanceByID(apiClient, endpoint, *gateway.ID)
		if errors.Is(err, client_lib.ErrObjectNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		return fmt.Errorf("gateway %s is still stored", *gateway.Name)
	})
	require.NoError(t, err)
}

// callHost checks the host's http body and certificate.
func callHost(t *testing.T, kubeClient dynamic.Interface, nodeIP string, ports map[int64]int64, host string) {
	t.Helper()

	// call port 80 and compare the certificate on port 443
	body := httpGet(t, fmt.Sprintf("http://%s:%d/", nodeIP, ports[80]), host)
	require.Contains(t, body, echoBody)
	cert := waitForCertificate(t, kubeClient, host)
	peer := tlsPeer(t, fmt.Sprintf("%s:%d", nodeIP, ports[443]), host)
	require.Equal(t, issuedCertificate(t, kubeClient, cert), peer)
}

// waitForSharedListeners returns when the stored global configuration lists these listeners.
// previousID is zero on the first read and the stored id after that.
func waitForSharedListeners(
	t *testing.T,
	apiClient *http.Client,
	endpoint string,
	runtimeID uint,
	want []string,
	previousID uint,
) uint {
	t.Helper()

	// read the stored listeners because the operator does not pass -global-configuration
	var id uint
	err := util.Retry(reconcileAttempts, 2, func() error {
		gotID, names, err := sharedListeners(apiClient, endpoint, runtimeID)
		if err != nil {
			return err
		}
		if !sameNames(names, want) {
			return fmt.Errorf("listeners %v, want %v", names, want)
		}
		if previousID != 0 && gotID != previousID {
			return fmt.Errorf("global configuration id %d, want %d", gotID, previousID)
		}
		id = gotID
		return nil
	})
	require.NoError(t, err)
	return id
}

// waitForHostObjects returns when the host's gateway objects are stored.
func waitForHostObjects(
	t *testing.T,
	apiClient *http.Client,
	endpoint string,
	kubeClient dynamic.Interface,
	workloadID uint,
	name string,
	host string,
	port int,
) {
	t.Helper()

	// wait until each object is stored and present in the cluster
	err := util.Retry(reconcileAttempts, 2, func() error {
		return hostObjectsMatch(apiClient, endpoint, kubeClient, workloadID, name, host, port, true)
	})
	require.NoError(t, err)
}

// waitForHostTornDown returns when the host's gateway objects are gone.
func waitForHostTornDown(
	t *testing.T,
	apiClient *http.Client,
	endpoint string,
	kubeClient dynamic.Interface,
	workloadID uint,
	name string,
	host string,
	port int,
) {
	t.Helper()

	// wait until each object is gone and the echo service remains
	err := util.Retry(reconcileAttempts, 2, func() error {
		return hostObjectsMatch(apiClient, endpoint, kubeClient, workloadID, name, host, port, false)
	})
	require.NoError(t, err)
}

// hostObjectsMatch checks the host's stored objects and the echo service.
func hostObjectsMatch(
	apiClient *http.Client,
	endpoint string,
	kubeClient dynamic.Interface,
	workloadID uint,
	name string,
	host string,
	port int,
	present bool,
) error {
	// index the workload's stored manifests
	resources, err := workloadResources(apiClient, endpoint, workloadID)
	if err != nil {
		return err
	}
	if err := echoServiceKept(resources, kubeClient); err != nil {
		return err
	}

	// compare each gateway object with the stored row and the cluster
	for _, object := range hostGatewayObjects(name, host, port) {
		resource, stored := resources[object.kind+"/"+object.name]
		if present && !stored {
			return fmt.Errorf("%s %s is not stored", object.kind, object.name)
		}
		if present && resource.ScheduledForDeletion != nil {
			return fmt.Errorf("%s %s is scheduled for deletion", object.kind, object.name)
		}
		if !present && stored {
			return fmt.Errorf("%s %s is still stored", object.kind, object.name)
		}
		if !object.cluster {
			continue
		}
		_, getErr := kubeClient.Resource(object.gvr).Namespace(echoNamespace).Get(
			context.Background(),
			object.name,
			metav1.GetOptions{},
		)
		if present && getErr != nil {
			return getErr
		}
		if !present && getErr != nil && !kubeerrors.IsNotFound(getErr) {
			return getErr
		}
		if !present && getErr == nil {
			return fmt.Errorf("%s %s is still in the cluster", object.kind, object.name)
		}
	}
	return nil
}

// gatewayControllerID returns the live shared controller workload.
func gatewayControllerID(t *testing.T, apiClient *http.Client, endpoint string, runtimeID uint) uint {
	t.Helper()

	// load the controller the runtime records
	runtime, err := client.GetKubernetesRuntimeInstanceByID(apiClient, endpoint, runtimeID)
	require.NoError(t, err)
	require.NotNil(t, runtime.GatewayControllerInstanceID)
	workload, err := client.GetKubernetesWorkloadInstanceByID(
		apiClient,
		endpoint,
		*runtime.GatewayControllerInstanceID,
	)
	require.NoError(t, err)
	require.Nil(t, workload.DeletionScheduled)
	return *runtime.GatewayControllerInstanceID
}

// waitForSharedComponents polls until the named install count matches.
func waitForSharedComponents(t *testing.T, kubeClient dynamic.Interface, want int) {
	t.Helper()

	// count the cluster objects the shared install applies
	err := util.Retry(reconcileAttempts, 2, func() error {
		ingressCount, err := namedComponentCount(kubeClient, "ingresscomponents", ingressComponentName)
		if err != nil {
			return err
		}
		certificatesCount, err := namedComponentCount(
			kubeClient,
			"certificatescomponents",
			certificatesComponentName,
		)
		if err != nil {
			return err
		}
		if ingressCount != want || certificatesCount != want {
			return fmt.Errorf("ingress %d certificates %d", ingressCount, certificatesCount)
		}
		return nil
	})
	require.NoError(t, err)
}

// waitForSharedControllerRemoved polls until the shared install is gone.
func waitForSharedControllerRemoved(
	t *testing.T,
	apiClient *http.Client,
	endpoint string,
	kubeClient dynamic.Interface,
	runtimeID uint,
) {
	t.Helper()

	// require the controller workload to be deleting and the components gone
	err := util.Retry(reconcileAttempts, 2, func() error {
		runtime, err := client.GetKubernetesRuntimeInstanceByID(apiClient, endpoint, runtimeID)
		if err != nil {
			return err
		}
		if runtime.GatewayControllerInstanceID == nil {
			return errors.New("runtime has no gateway controller")
		}
		workload, err := client.GetKubernetesWorkloadInstanceByID(
			apiClient,
			endpoint,
			*runtime.GatewayControllerInstanceID,
		)
		if err != nil && !errors.Is(err, client_lib.ErrObjectNotFound) {
			return err
		}
		if err == nil && workload.DeletionScheduled == nil {
			return errors.New("gateway controller workload is still live")
		}
		ingressCount, err := namedComponentCount(kubeClient, "ingresscomponents", ingressComponentName)
		if err != nil {
			return err
		}
		certificatesCount, err := namedComponentCount(
			kubeClient,
			"certificatescomponents",
			certificatesComponentName,
		)
		if err != nil {
			return err
		}
		if ingressCount != 0 || certificatesCount != 0 {
			return fmt.Errorf("ingress %d certificates %d", ingressCount, certificatesCount)
		}
		return nil
	})
	require.NoError(t, err)
}

// namedComponentCount counts cluster objects of one kind and name.
func namedComponentCount(kubeClient dynamic.Interface, resource, name string) (int, error) {
	// list the cluster objects
	list, err := kubeClient.Resource(schema.GroupVersionResource{
		Group:    "platform.addons.nukleros.io",
		Version:  "v1alpha1",
		Resource: resource,
	}).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return 0, err
	}

	// count the shared name
	count := 0
	for _, item := range list.Items {
		if item.GetName() == name {
			count++
		}
	}
	return count, nil
}

// sharedListeners reads the one global configuration stored for the runtime.
func sharedListeners(apiClient *http.Client, endpoint string, runtimeID uint) (uint, []string, error) {
	// load the gateway controller workload
	runtime, err := client.GetKubernetesRuntimeInstanceByID(apiClient, endpoint, runtimeID)
	if err != nil {
		return 0, nil, err
	}
	if runtime.GatewayControllerInstanceID == nil {
		return 0, nil, errors.New("runtime has no gateway controller")
	}
	resources, err := workloadResources(apiClient, endpoint, *runtime.GatewayControllerInstanceID)
	if err != nil {
		return 0, nil, err
	}

	// require exactly one live global configuration
	resource, ok := resources["GlobalConfiguration/"+sharedConfigName]
	if !ok {
		return 0, nil, errors.New("shared global configuration is not stored")
	}
	if resource.ScheduledForDeletion != nil {
		return 0, nil, errors.New("shared global configuration is scheduled for deletion")
	}
	if resource.JSONDefinition == nil || resource.ID == nil {
		return 0, nil, errors.New("shared global configuration has no definition")
	}
	var object map[string]interface{}
	if err := json.Unmarshal(*resource.JSONDefinition, &object); err != nil {
		return 0, nil, err
	}
	namespace, _, err := unstructured.NestedString(object, "metadata", "namespace")
	if err != nil {
		return 0, nil, err
	}
	if namespace != ingressNamespace {
		return 0, nil, fmt.Errorf("global configuration namespace %s", namespace)
	}
	names, err := listenerNames(object)
	if err != nil {
		return 0, nil, err
	}
	return *resource.ID, names, nil
}

// workloadResources indexes a workload's manifests by kind and name.
func workloadResources(
	apiClient *http.Client,
	endpoint string,
	workloadID uint,
) (map[string]*api.KubernetesWorkloadResourceInstance, error) {
	// load every resource stored for the workload
	stored, err := client.GetKubernetesWorkloadResourceInstancesByID(apiClient, endpoint, workloadID)
	if err != nil {
		return nil, err
	}
	indexed := map[string]*api.KubernetesWorkloadResourceInstance{}
	for i := range *stored {
		resource := &(*stored)[i]
		if resource.JSONDefinition == nil {
			continue
		}
		var object map[string]interface{}
		if err := json.Unmarshal(*resource.JSONDefinition, &object); err != nil {
			return nil, err
		}
		kind, _, err := unstructured.NestedString(object, "kind")
		if err != nil {
			return nil, err
		}
		name, _, err := unstructured.NestedString(object, "metadata", "name")
		if err != nil {
			return nil, err
		}
		if kind == "" || name == "" {
			return nil, errors.New("stored resource has no kind or name")
		}
		key := kind + "/" + name
		if _, exists := indexed[key]; exists {
			return nil, fmt.Errorf("duplicate %s", key)
		}
		indexed[key] = resource
	}
	return indexed, nil
}

// echoServiceKept checks the echo service is still stored and applied.
func echoServiceKept(
	resources map[string]*api.KubernetesWorkloadResourceInstance,
	kubeClient dynamic.Interface,
) error {
	// reject a missing or deleting service row
	resource, ok := resources["Service/"+echoServiceName]
	if !ok {
		return errors.New("echo service is not stored")
	}
	if resource.ScheduledForDeletion != nil {
		return errors.New("echo service is scheduled for deletion")
	}
	_, err := kubeClient.Resource(schema.GroupVersionResource{
		Version: "v1", Resource: "services",
	}).Namespace(echoNamespace).Get(context.Background(), echoServiceName, metav1.GetOptions{})
	return err
}

// hostGatewayObjects names the manifests one host adds to the echo workload.
func hostGatewayObjects(name, host string, port int) []gatewayObject {
	// issuer and certificate use the domain name, and the ingress uses the host
	issuer := strcase.ToKebab(name)
	return []gatewayObject{
		{
			kind:    "Ingress",
			name:    strcase.ToKebab(host),
			gvr:     schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "ingresses"},
			cluster: true,
		},
		{
			kind:    "Issuer",
			name:    issuer,
			gvr:     schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "issuers"},
			cluster: true,
		},
		{
			kind:    "Certificate",
			name:    issuer,
			gvr:     schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "certificates"},
			cluster: true,
		},
		{
			kind: "TransportServer",
			name: fmt.Sprintf("%s-%d", name, port),
		},
	}
}

// listenerNames reads spec.listeners names in stored order.
func listenerNames(object map[string]interface{}) ([]string, error) {
	// an absent list is an empty listener set
	listeners, found, err := unstructured.NestedSlice(object, "spec", "listeners")
	if err != nil || !found {
		return nil, err
	}
	names := make([]string, 0, len(listeners))
	for _, listener := range listeners {
		entry, ok := listener.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("listener is %T", listener)
		}
		name, _, err := unstructured.NestedString(entry, "name")
		if err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, nil
}

// sameNames reports whether both lists hold the same names in order.
func sameNames(got, want []string) bool {
	// compare length and then each position
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// numberValue reads a JSON number.
func numberValue(value interface{}) (int64, bool) {
	// accept the number types the dynamic client returns
	switch number := value.(type) {
	case int64:
		return number, true
	case float64:
		return int64(number), true
	case int:
		return int64(number), true
	default:
		return 0, false
	}
}

// containsString reports whether values includes want.
func containsString(values []string, want string) bool {
	// compare each stored name
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
