package kubernetesruntime

import (
	"errors"
	"fmt"
	"net/url"
	"testing"

	"github.com/go-logr/logr"

	controller "github.com/threeport/threeport/pkg/controller/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// timeoutNetErr is a dial timeout the way net.OpError reports one.
type timeoutNetErr struct{}

// Error returns the timeout text.
func (timeoutNetErr) Error() string { return "i/o timeout" }

// Timeout reports that the dial timed out.
func (timeoutNetErr) Timeout() bool { return true }

// Temporary reports that the dial can be retried.
func (timeoutNetErr) Temporary() bool { return true }

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

// TestWorkloadAPIErr keeps retrying a dial and returns any other error.
func TestWorkloadAPIErr(t *testing.T) {
	log := logr.Discard()
	dial := fmt.Errorf(
		"failed to get kube API group resources: %w",
		&url.Error{Op: "Get", URL: "https://10.0.0.1/api", Err: timeoutNetErr{}},
	)

	// a dial timeout retries and does not count as a failure
	delay, err := workloadAPIErr(&log, dial, "failed to get a Kubernetes client and mapper")
	if err != nil {
		t.Fatal(err)
	}
	if delay != controller.Requeue30s {
		t.Fatalf("delay = %d", delay)
	}

	// an API response stays a failure for the one-hour limit
	delay, err = workloadAPIErr(&log, errors.New("forbidden"), "failed to check support services operator")
	if err == nil {
		t.Fatal("an API response was retried")
	}
	if delay != 0 {
		t.Fatalf("delay = %d", delay)
	}
}
