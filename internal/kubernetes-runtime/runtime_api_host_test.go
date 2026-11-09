package kubernetesruntime

import (
	"testing"

	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestRuntimeAPIHost leaves a nil endpoint unset and rejects an endpoint with no host.
func TestRuntimeAPIHost(t *testing.T) {
	// leave a nil endpoint unset
	host, err := runtimeAPIHost(nil)
	if err != nil {
		t.Fatal(err)
	}
	if host != "" {
		t.Fatalf("nil endpoint host = %q", host)
	}

	// reject an endpoint with no host
	host, err = runtimeAPIHost(util.Ptr("https://"))
	if err == nil {
		t.Fatal("empty host returned without an error")
	}
	if host != "" {
		t.Fatalf("empty host = %q", host)
	}

	// return the host
	host, err = runtimeAPIHost(util.Ptr("https://10.0.0.1"))
	if err != nil {
		t.Fatal(err)
	}
	if host != "10.0.0.1" {
		t.Fatalf("host = %q", host)
	}
}
