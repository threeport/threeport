package v0

import (
	"strings"
	"testing"
)

// TestGatewayHttpPortBeforeCreate rejects a missing port and tls with a redirect, and defaults an empty path.
func TestGatewayHttpPortBeforeCreate(t *testing.T) {
	// a missing port fails
	port := &GatewayHttpPort{}
	err := port.beforeCreate(nil)
	if err == nil || !strings.Contains(err.Error(), "port must be set") {
		t.Fatalf("missing port error = %v", err)
	}

	// tls and a redirect on one port fail
	portNum := 443
	enabled := true
	port.Port = &portNum
	port.TLSEnabled = &enabled
	port.HTTPSRedirect = &enabled
	err = port.beforeCreate(nil)
	if err == nil || !strings.Contains(err.Error(), "cannot both") {
		t.Fatalf("tls and redirect error = %v", err)
	}

	// an empty path becomes /
	redirect := false
	empty := ""
	port.HTTPSRedirect = &redirect
	port.Path = &empty
	if err := port.beforeCreate(nil); err != nil {
		t.Fatal(err)
	}
	if port.Path == nil || *port.Path != "/" {
		t.Fatalf("path = %v", *port.Path)
	}
}

// TestGatewayHttpPortBeforeUpdateLeavesOmittedPath does not set a path the update omitted.
func TestGatewayHttpPortBeforeUpdateLeavesOmittedPath(t *testing.T) {
	// an update that omits path does not set one
	port := &GatewayHttpPort{}
	if err := port.beforeUpdate(nil); err != nil {
		t.Fatal(err)
	}
	if port.Path != nil {
		t.Fatalf("path = %v", *port.Path)
	}
}
