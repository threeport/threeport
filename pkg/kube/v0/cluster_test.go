package v0

import (
	"strings"
	"testing"

	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// TestGetRestConfigControlPlaneHostUsesServiceAddress keeps the stored credential.
func TestGetRestConfigControlPlaneHostUsesServiceAddress(t *testing.T) {
	// rewrite the host and keep the client certificate
	host := true
	endpoint := "https://127.0.0.1:39733"
	cert := "cert"
	key := "key"
	ca := "ca"
	runtime := &v0.KubernetesRuntimeInstance{
		ThreeportControlPlaneHost: &host,
		APIEndpoint:               &endpoint,
		Certificate:               &cert,
		CertificateKey:            &key,
		CACertificate:             &ca,
	}
	config, err := GetRestConfig(runtime, true, nil, "", "")
	if err != nil {
		t.Fatalf("got %v", err)
	}
	if config.Host != "https://kubernetes.default.svc.cluster.local" {
		t.Fatalf("got host %s", config.Host)
	}
	if len(config.CertData) == 0 {
		t.Fatal("stored client certificate was dropped")
	}
}

// TestGetRestConfigRequiresEndpointOffControlPlane rejects a missing API endpoint.
func TestGetRestConfigRequiresEndpointOffControlPlane(t *testing.T) {
	// reject a runtime that is not the control plane host
	_, err := GetRestConfig(&v0.KubernetesRuntimeInstance{}, false, nil, "", "")
	if err == nil || !strings.Contains(err.Error(), "without API endpoint") {
		t.Fatalf("got %v", err)
	}
}
