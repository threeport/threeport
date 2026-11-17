package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/pulumi/pulumi/sdk/v3/go/auto/optpreview"
)

// ErrUpgradeDeclined is returned when the operator declines the infrastructure change.
var ErrUpgradeDeclined = errors.New("infrastructure upgrade declined")

// Upgrade previews the infrastructure program against the deployed stack and applies it if accept returns true.
// A declined change leaves the deployed stack as it was.
func (i *KubernetesRuntimeInfraGKE) Upgrade(ctx context.Context, accept func(string) bool) error {
	// authenticate before reading the stack
	cleanup, err := i.useGCPAuth()
	if err != nil {
		return err
	}
	defer cleanup()

	// refuse an upgrade whose backup bucket the stack would grant access to but cannot find
	if i.ProvisionBackups {
		if err := i.requireBackupBucket(); err != nil {
			return err
		}
	}

	// prepare the program
	if i.ProjectName == "" {
		i.InitPulumiProject()
	}
	stack, err := i.SetupStack(i.pulumiProgram())
	if err != nil {
		return err
	}

	// keep what the create path keeps on a stack that holds the cluster: its
	// version and release channel, and the node pool's service account and
	// boot disk
	exported, err := stack.Export(ctx)
	if err != nil {
		return fmt.Errorf("failed to read the stack's GKE cluster: %w", err)
	}
	i.keepStackMinMasterVersion(exported.Deployment)
	i.keepStackReleaseChannel(exported.Deployment)
	i.keepStackNodePool(exported.Deployment)

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
