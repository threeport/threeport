package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	api "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

const (
	echoServiceName     = "echo"
	echoHost            = "echo.threeport.test"
	echoBody            = "gateway-ok"
	ingressNamespace    = "nukleros-ingress-system"
	nginxIngressService = "nginx-ingress-nodeport"
	reconcileAttempts   = 180
	certificateAttempts = 300
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
  - port: 80
    targetPort: 5678
`

// TestGatewayIngressServesHTTPAndHTTPS creates an echo workload and a gateway
// through the threeport API, then calls the published ingress over HTTP and HTTPS.
func TestGatewayIngressServesHTTPAndHTTPS(t *testing.T) {
	// open the control plane api and the kind runtime it records
	apiClient, endpoint, encryptionKey := threeportClient(t)
	runtime := controlPlaneRuntime(t, apiClient, endpoint)

	// create the echo workload and the gateway that fronts it
	domain := createEchoDomain(t, apiClient, endpoint)
	workload := createEchoWorkload(t, apiClient, endpoint, runtime)
	waitForEchoService(t, apiClient, endpoint, workload)
	definition := createEchoGatewayDefinition(t, apiClient, endpoint, domain)
	waitForReconciled(t, "gateway definition", func() (*bool, error) {
		stored, err := client.GetGatewayDefinitionByID(apiClient, endpoint, *definition.ID)
		if err != nil {
			return nil, err
		}
		return stored.Reconciled, nil
	})
	gateway := createEchoGatewayInstance(t, apiClient, endpoint, definition, workload, runtime)
	waitForReconciled(t, "gateway instance", func() (*bool, error) {
		stored, err := client.GetGatewayInstanceByID(apiClient, endpoint, *gateway.ID)
		if err != nil {
			return nil, err
		}
		return stored.Reconciled, nil
	})

	// read the service and certificate the operator installed
	kubeClient, _, err := kube.GetClient(runtime, false, apiClient, endpoint, encryptionKey)
	require.NoError(t, err)
	cert := waitForCertificate(t, kubeClient, echoHost)
	ports := waitForIngressPorts(t, kubeClient)
	nodeIP := nodeInternalIP(t, kubeClient)

	// call port 80 and compare the certificate on port 443
	body := httpGet(t, fmt.Sprintf("http://%s:%d/", nodeIP, ports[80]), echoHost)
	require.Contains(t, body, echoBody)
	peer := tlsPeer(t, fmt.Sprintf("%s:%d", nodeIP, ports[443]), echoHost)
	require.Equal(t, issuedCertificate(t, kubeClient, cert), peer)
}

// createEchoDomain registers the host the ingress matches.
func createEchoDomain(t *testing.T, apiClient *http.Client, endpoint string) *api.DomainNameDefinition {
	t.Helper()

	// store the domain the gateway serves
	created, err := client.CreateDomainNameDefinition(apiClient, endpoint, &api.DomainNameDefinition{
		Definition: api.Definition{Name: util.Ptr("echo")},
		Domain:     util.Ptr(echoHost),
		Zone:       util.Ptr(echoHost),
		AdminEmail: util.Ptr("no-reply@threeport.io"),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		retryDelete(t, "echo domain", func() error {
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

// createEchoGatewayDefinition registers http and https ports for the echo service.
func createEchoGatewayDefinition(
	t *testing.T,
	apiClient *http.Client,
	endpoint string,
	domain *api.DomainNameDefinition,
) *api.GatewayDefinition {
	t.Helper()

	// define ports 80 and 443 for the echo service
	created, err := client.CreateGatewayDefinition(apiClient, endpoint, &api.GatewayDefinition{
		Definition:             api.Definition{Name: util.Ptr("echo")},
		DomainNameDefinitionID: domain.ID,
		ServiceName:            util.Ptr(echoServiceName),
		HttpPorts: []*api.GatewayHttpPort{
			{Port: util.Ptr(80), TLSEnabled: util.Ptr(false), Path: util.Ptr("/")},
			{Port: util.Ptr(443), TLSEnabled: util.Ptr(true), Path: util.Ptr("/")},
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		retryDelete(t, "echo gateway definition", func() error {
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
	definition *api.GatewayDefinition,
	workload *api.KubernetesWorkloadInstance,
	runtime *api.KubernetesRuntimeInstance,
) *api.GatewayInstance {
	t.Helper()

	// attach the gateway to the echo workload on the control plane runtime
	created, err := client.CreateGatewayInstance(apiClient, endpoint, &api.GatewayInstance{
		Instance:                     api.Instance{Name: util.Ptr("echo-0")},
		GatewayDefinitionID:          definition.ID,
		KubernetesRuntimeInstanceID:  runtime.ID,
		KubernetesWorkloadInstanceID: workload.ID,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		retryDelete(t, "echo gateway instance", func() error {
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
