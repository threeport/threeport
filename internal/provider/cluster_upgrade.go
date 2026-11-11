package provider

import (
	"context"
	"fmt"

	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	tpaws "github.com/threeport/threeport/pkg/aws/v0"
)

// ClusterKubernetesVersion returns the Kubernetes minor version the EKS
// cluster runs: the lower of its control plane's and its node group's.  EKS
// upgrades the control plane on its own when its version leaves standard
// support, and leaves the node group behind.
func (i *KubernetesRuntimeInfraEKS) ClusterKubernetesVersion() (string, error) {
	controlPlane, err := tpaws.EksClusterVersion(i.AwsConfig, i.RuntimeInstanceName)
	if err != nil {
		return "", err
	}
	nodeGroup, err := tpaws.EksNodeGroupVersion(i.AwsConfig, i.RuntimeInstanceName, i.nodeGroupName())
	if err != nil {
		return "", err
	}

	return lowerKubernetesVersion(controlPlane, nodeGroup)
}

// UpgradeInfra upgrades the EKS cluster's control plane, and then its node
// group, to version.  The stack keeps the cluster's upgrade policy, as a
// resumed create does, and the roles the create gave the cluster and its node
// group.
func (i *KubernetesRuntimeInfraEKS) UpgradeInfra(version string) error {
	i.KubernetesVersion = version
	if err := i.lookupClusterIam(); err != nil {
		return err
	}

	stack, err := i.SetupStack(i.pulumiProgram())
	if err != nil {
		return fmt.Errorf("failed to set up Pulumi workspace: %w", err)
	}
	exported, err := stack.Export(context.Background())
	if err != nil {
		return fmt.Errorf("failed to read the stack's EKS cluster: %w", err)
	}
	i.keepStackSupportType(exported.Deployment)

	return i.upgradeStack(stack)
}

// ClusterKubernetesVersion returns the Kubernetes minor version the GKE
// cluster's control plane runs.  GKE upgrades the nodes to it on its own,
// within the maintenance window.
func (i *KubernetesRuntimeInfraGKE) ClusterKubernetesVersion() (string, error) {
	release, err := i.useGCPAuth()
	if err != nil {
		return "", err
	}
	defer release()

	cluster, err := i.getCluster(context.Background())
	if err != nil {
		return "", err
	}
	version := minorVersion(cluster.CurrentMasterVersion)
	if version == "" {
		return "", fmt.Errorf("GKE cluster %s reports no Kubernetes version it can be read from: %q", i.RuntimeInstanceName, cluster.CurrentMasterVersion)
	}

	return version, nil
}

// UpgradeInfra upgrades the GKE cluster's control plane to version, by
// raising the minimum version it asks for.  GKE upgrades it to its release
// channel's release of that version, and the nodes after it within the
// maintenance window.  The stack keeps the cluster's release channel, as a
// resumed create does.
func (i *KubernetesRuntimeInfraGKE) UpgradeInfra(version string) error {
	i.Version = version
	i.stackMinMasterVersion = nil

	stack, err := i.SetupStack(i.pulumiProgram())
	if err != nil {
		return fmt.Errorf("failed to set up Pulumi workspace: %w", err)
	}
	exported, err := stack.Export(context.Background())
	if err != nil {
		return fmt.Errorf("failed to read the stack's GKE cluster: %w", err)
	}
	i.keepStackReleaseChannel(exported.Deployment)

	return i.upgradeStack(stack)
}

// lowerKubernetesVersion returns the older of two minor versions.
func lowerKubernetesVersion(a, b string) (string, error) {
	comparison, err := api_v0.CompareKubernetesVersions(a, b)
	if err != nil {
		return "", err
	}
	if comparison > 0 {
		return b, nil
	}

	return a, nil
}
