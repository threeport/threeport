// Package crdbha verifies the production-tier CockroachDB cluster #602
// built against a real, multi-node Kubernetes scheduler - not the fake
// dynamic client the unit tests in pkg/threeport-installer/v0 use, which
// cannot schedule a pod, run a container, or lose a node.
//
// CI's existing integration test only ever installs onto kind, which can
// only run the development tier - the one reason #602 confirmed production
// manifests work was a manual, one-off verification against a hand-built
// multi-node kind cluster. This package makes that verification a standing,
// automated one: every PR that touches the production CockroachDB path runs
// it again, rather than relying on it having been checked once.
package crdbha

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"

	"k8s.io/apimachinery/pkg/api/meta"

	"github.com/threeport/threeport/internal/provider"
)

// clusterName is the kind cluster this package creates and deletes. A fixed
// name rather than a generated one: a leftover from a crashed previous run
// is a collision this package can clean up itself, by deleting it under the
// same name, rather than an orphan with a name nothing refers to again.
const clusterName = "crdb-ha-test"

// schedulableNodes is how many nodes a production CockroachDB cluster needs
// able to run a pod. kind's control-plane node is tainted and does not
// count, so this is worker nodes alone.
const schedulableNodes = 3

var (
	dynamicClient dynamic.Interface
	restMapper    meta.RESTMapper
	typedClient   *kubernetes.Clientset
	restConfig    *rest.Config
)

// TestMain provisions one multi-node kind cluster for the package and tears
// it down afterward.
//
// os.Exit skips deferred functions, so every exit path below deletes the
// cluster itself rather than relying on a defer - the same shape
// test/cockroach/main_test.go uses for its container.
func TestMain(m *testing.M) {
	if _, err := exec.LookPath("docker"); err != nil {
		fmt.Println("docker not available; skipping the production-tier CockroachDB tests")
		os.Exit(0)
	}

	kubeconfigPath := filepath.Join(os.TempDir(), "crdb-ha-test-kubeconfig")

	infra := &provider.KubernetesRuntimeInfraKind{
		RuntimeInstanceName: clusterName,
		KubeconfigPath:      kubeconfigPath,
		NumWorkerNodes:      schedulableNodes,
	}

	// a cluster left behind by a previous run that crashed before its own
	// teardown is a name collision kind refuses to create over, not
	// something a fresh run can clean up after itself without first
	// knowing it is there
	_ = infra.Delete()

	if _, err := infra.Create(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create kind cluster: %v\n", err)
		os.Exit(1)
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to build rest config: %v\n", err)
		_ = infra.Delete()
		os.Exit(1)
	}

	dynamicClient, err = dynamic.NewForConfig(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to build dynamic client: %v\n", err)
		_ = infra.Delete()
		os.Exit(1)
	}

	typedClient, err = kubernetes.NewForConfig(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to build typed client: %v\n", err)
		_ = infra.Delete()
		os.Exit(1)
	}

	disco, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to build discovery client: %v\n", err)
		_ = infra.Delete()
		os.Exit(1)
	}
	groupResources, err := restmapper.GetAPIGroupResources(disco)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get API group resources: %v\n", err)
		_ = infra.Delete()
		os.Exit(1)
	}
	restMapper = restmapper.NewDiscoveryRESTMapper(groupResources)
	restConfig = config

	code := m.Run()

	if err := infra.Delete(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to delete kind cluster: %v\n", err)
		if code == 0 {
			code = 1
		}
	}

	os.Exit(code)
}
