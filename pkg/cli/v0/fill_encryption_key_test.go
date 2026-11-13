package v0

import (
	"testing"

	tpapi "github.com/threeport/threeport/pkg/api/v0"
)

// TestFillEncryptionKeyLeavesTheConfigWhenTheClusterCannotOpen calls the
// function a config update uses when the provider has no cluster client.
func TestFillEncryptionKeyLeavesTheConfigWhenTheClusterCannotOpen(t *testing.T) {
	// save the certificates when the key cannot be read
	controlPlane := &ControlPlane{
		Name:      "child",
		Provider:  tpapi.KubernetesRuntimeInfraProviderKind,
		CACert:    "ca",
		APIServer: "https://child",
	}
	err := FillEncryptionKey(&ThreeportConfig{}, "https://genesis", controlPlane, "child-ns")
	if err != nil {
		t.Fatal(err)
	}
	if controlPlane.EncryptionKey != "" {
		t.Fatalf("EncryptionKey %q", controlPlane.EncryptionKey)
	}
	if controlPlane.CACert != "ca" {
		t.Fatalf("CACert %q", controlPlane.CACert)
	}
}
