package v0

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/datatypes"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/discovery"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
)

// ValidateK8sApiAllowedCidrsFlag checks --k8s-api-allowed-cidrs: the blocks
// must be valid and within the provider's limit, and the flag applies only
// to a cluster tptctl creates on a cloud provider.
func ValidateK8sApiAllowedCidrsFlag(cidrs []string, infraProvider string, controlPlaneOnly bool) error {
	if len(cidrs) == 0 {
		return nil
	}

	if infraProvider == v0.KubernetesRuntimeInfraProviderKind {
		return errors.New("--k8s-api-allowed-cidrs is not supported with infrastructure provider 'kind'")
	}
	if controlPlaneOnly {
		return errors.New("--k8s-api-allowed-cidrs is not supported with --control-plane-only, since tptctl does not create the cluster")
	}

	if _, err := v0.ParseCidrs("--k8s-api-allowed-cidrs", allowedCidrsJSON(cidrs)); err != nil {
		return err
	}
	limit, _ := v0.AllowedCidrsLimit(infraProvider, false)
	if len(cidrs) > limit {
		return fmt.Errorf(
			"--k8s-api-allowed-cidrs has %d entries and %s admits at most %d",
			len(cidrs), infraProvider, limit,
		)
	}

	return nil
}

// allowedCidrsJSON returns cidrs as the JSON array the API stores, or nil
// when there are none.
func allowedCidrsJSON(cidrs []string) *datatypes.JSON {
	if len(cidrs) == 0 {
		return nil
	}

	// a slice of strings always marshals
	raw, _ := json.Marshal(cidrs)
	cidrsJSON := datatypes.JSON(raw)
	return &cidrsJSON
}

// How long tptctl up tries to reach a new cluster's Kubernetes API before
// it installs anything, the time each attempt may take, and the pause
// between attempts.  Endpoint changes can take a minute or two to apply.
var (
	kubeAPIReachTimeout        = 2 * time.Minute
	kubeAPIReachAttemptTimeout = 10 * time.Second
	kubeAPIReachRetryWait      = 5 * time.Second
)

// waitForKubernetesAPI asks the Kubernetes API of runtimeInstance for its
// version until it answers or kubeAPIReachTimeout passes.  An answer that
// refuses the request still shows that the API is reachable, which is all
// this checks.  tptctl up runs it when --k8s-api-allowed-cidrs is set, so
// that a list missing this machine's address fails before anything is
// installed, with an error that says so.
func waitForKubernetesAPI(runtimeInstance *v0.KubernetesRuntimeInstance, allowedCidrs []string) error {
	restConfig, err := kube.GetRestConfig(runtimeInstance, false, nil, "", "")
	if err != nil {
		return fmt.Errorf("failed to get a REST config for the Kubernetes API: %w", err)
	}
	restConfig.Timeout = kubeAPIReachAttemptTimeout

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(restConfig)
	if err != nil {
		return fmt.Errorf("failed to create a discovery client for the Kubernetes API: %w", err)
	}

	deadline := time.Now().Add(kubeAPIReachTimeout)
	for {
		_, err = discoveryClient.ServerVersion()
		var statusErr *apierrors.StatusError
		if err == nil || errors.As(err, &statusErr) {
			return nil
		}
		if time.Now().Add(kubeAPIReachRetryWait).After(deadline) {
			break
		}
		time.Sleep(kubeAPIReachRetryWait)
	}

	return fmt.Errorf(
		"could not reach the Kubernetes API at %s within %s - the most likely cause is that this machine's public address is not in --k8s-api-allowed-cidrs %v: %w",
		restConfig.Host, kubeAPIReachTimeout, allowedCidrs, err,
	)
}

// withUnreachableClusterCleanup adds to err how to delete a cluster whose
// Kubernetes API tptctl up could not reach, which is left in place unless
// --teardown-on-failure is set.
func withUnreachableClusterCleanup(err error, controlPlaneName string, teardownOnFailure bool) error {
	if teardownOnFailure {
		return err
	}

	return fmt.Errorf(
		"%w\nthe cluster has been left in place - delete it with 'tptctl down --name %s --infra-only', or run tptctl up with --teardown-on-failure to delete it on failure",
		err, controlPlaneName,
	)
}
