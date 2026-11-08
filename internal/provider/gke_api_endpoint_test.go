package provider

import "testing"

// TestGkeAPIEndpoint refuses an empty cluster endpoint and prefixes a host.
func TestGkeAPIEndpoint(t *testing.T) {
	// refuse an empty cluster endpoint
	endpoint, err := gkeAPIEndpoint("", "dev-01")
	if err == nil {
		t.Fatal("empty endpoint returned without an error")
	}
	if endpoint != "" {
		t.Fatalf("empty endpoint = %q", endpoint)
	}

	// prefix a host
	endpoint, err = gkeAPIEndpoint("10.0.0.1", "dev-01")
	if err != nil {
		t.Fatal(err)
	}
	if endpoint != "https://10.0.0.1" {
		t.Fatalf("endpoint = %q", endpoint)
	}
}
