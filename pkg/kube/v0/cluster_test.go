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

// TestARuntimeWithNoRecordedCAIsVerifiedInClusterWithTheServiceAccountCA
// covers a private GKE cluster, which records its DNS endpoint and no CA,
// since a public CA signs that endpoint's certificate.  A controller on that
// cluster reaches it at the in-cluster address instead, which serves the
// cluster's own certificate, so it verifies it with the pod's service
// account CA.  Off the cluster no CA is set and the system's roots verify the
// DNS endpoint, and a recorded CA is used as before.
func TestARuntimeWithNoRecordedCAIsVerifiedInClusterWithTheServiceAccountCA(t *testing.T) {
	host := true
	endpoint := "https://gke-0123456789abcdef-123456789012.us-central1.gke.goog"
	token := "token"
	cert := "cert"
	key := "key"
	noCA := ""
	recordedCA := "ca"

	cases := []struct {
		name     string
		runtime  *v0.KubernetesRuntimeInstance
		inPod    bool
		wantFile string
		wantData string
	}{
		{
			name:     "in the cluster, with a token",
			runtime:  &v0.KubernetesRuntimeInstance{ThreeportControlPlaneHost: &host, APIEndpoint: &endpoint, ConnectionToken: &token, CACertificate: &noCA},
			inPod:    true,
			wantFile: serviceAccountCAFile,
		},
		{
			name:     "in the cluster, with a client certificate",
			runtime:  &v0.KubernetesRuntimeInstance{ThreeportControlPlaneHost: &host, APIEndpoint: &endpoint, Certificate: &cert, CertificateKey: &key, CACertificate: &noCA},
			inPod:    true,
			wantFile: serviceAccountCAFile,
		},
		{
			name:    "off the cluster",
			runtime: &v0.KubernetesRuntimeInstance{ThreeportControlPlaneHost: &host, APIEndpoint: &endpoint, ConnectionToken: &token, CACertificate: &noCA},
		},
		{
			name:     "in the cluster, with a recorded CA",
			runtime:  &v0.KubernetesRuntimeInstance{ThreeportControlPlaneHost: &host, APIEndpoint: &endpoint, ConnectionToken: &token, CACertificate: &recordedCA},
			inPod:    true,
			wantData: recordedCA,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			config, err := GetRestConfig(c.runtime, c.inPod, nil, "", "")
			if err != nil {
				t.Fatalf("got %v", err)
			}
			if config.CAFile != c.wantFile {
				t.Errorf("got CA file %q, want %q", config.CAFile, c.wantFile)
			}
			if string(config.CAData) != c.wantData {
				t.Errorf("got CA data %q, want %q", config.CAData, c.wantData)
			}
		})
	}
}
