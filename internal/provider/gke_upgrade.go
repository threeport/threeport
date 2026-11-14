package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/pulumi/pulumi/sdk/v3/go/auto/optpreview"
)

const (
	// e2-medium cannot schedule a 2Gi request, and changing the first pool replaces it.
	standardNodeMachineType      = "e2-standard-2"
	developmentStandardNodeCount = 1
	productionStandardNodeCount  = 3
	productionWorkerMinCount     = 0
	standardNodeLocationPolicy   = "BALANCED"
	standardNodeDiskSizeGb       = 100
	standardNodeDiskType         = "pd-balanced"

	gkeNodePoolTypeToken = "gcp:container/nodePool:NodePool"
	gkeClusterTypeToken  = "gcp:container/cluster:Cluster"
)

// ErrUpgradeDeclined is returned when the operator declines the infrastructure change.
var ErrUpgradeDeclined = errors.New("infrastructure upgrade declined")

// standardNodePoolName is the second pool, sized for the control plane.
func (i *KubernetesRuntimeInfraGKE) standardNodePoolName() string {
	return fmt.Sprintf("%s-standard", i.RuntimeInstanceName)
}

// Upgrade previews the infrastructure program against the deployed stack and applies it if accept returns true.
// A declined change leaves the deployed stack as it was.
func (i *KubernetesRuntimeInfraGKE) Upgrade(ctx context.Context, accept func(string) bool) error {
	// an empty recorded type would recreate the pool at the zero default
	if i.MachineType == "" {
		return fmt.Errorf("recorded machine type is empty")
	}

	// authenticate before reading the stack
	cleanup, err := i.useGCPAuth()
	if err != nil {
		return err
	}
	defer cleanup()

	// prepare the program
	if i.ProjectName == "" {
		i.InitPulumiProject()
	}
	stack, err := i.SetupStack(i.pulumiProgram())
	if err != nil {
		return err
	}

	// preview the change
	preview, err := stack.Preview(ctx, optpreview.Diff())
	if err != nil {
		return fmt.Errorf("failed to preview the stack: %w", err)
	}

	// apply the change if the operator accepts
	if accept == nil || !accept(preview.StdOut) {
		return ErrUpgradeDeclined
	}
	if _, err = i.RunUp(ctx, stack); err != nil {
		return fmt.Errorf("failed to deploy stack: %w", err)
	}
	return nil
}
