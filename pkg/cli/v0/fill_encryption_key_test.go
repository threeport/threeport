package v0

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"

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

	// save the child in a config file after the key read is skipped
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	viper.SetConfigFile(path)
	viper.SetConfigType("yaml")
	if err := viper.ReadInConfig(); err != nil {
		t.Fatal(err)
	}
	cfg := &ThreeportConfig{}
	if err := UpdateThreeportConfig(cfg, controlPlane); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(saved)
	if !strings.Contains(text, "child") || !strings.Contains(text, "kind") {
		t.Fatalf("config %s", text)
	}
}
