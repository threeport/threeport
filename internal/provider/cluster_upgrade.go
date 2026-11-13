package provider

import (
	"context"
	"fmt"

	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	tpaws "github.com/threeport/threeport/pkg/aws/v0"
)

// ClusterKubernetesVersion returns the Kubernetes minor version the EKS
// cluster runs: the lowest of its control plane's and its node groups'.  EKS
// upgrades the control plane on its own when its version leaves standard
// support, and leaves the node groups behind.
func (i *KubernetesRuntimeInfraEKS) ClusterKubernetesVersion() (string, error) {
	version, err := eksControlPlaneVersion(i.AwsConfig, i.RuntimeInstanceName)
	if err != nil {
		return "", err
	}
	for _, nodeGroupName := range i.nodeGroupNames() {
		nodeGroup, err := eksNodeGroupVersion(i.AwsConfig, i.RuntimeInstanceName, nodeGroupName)
		if err != nil {
			return "", err
		}
		if version, err = lowerKubernetesVersion(version, nodeGroup); err != nil {
			return "", err
		}
	}

	return version, nil
}

// eksNodeGroupVersion reads an EKS node group's minor version.  A test stands
// in for it.
var eksNodeGroupVersion = tpaws.EksNodeGroupVersion

// UpgradeInfra upgrades the EKS cluster's control plane, and then its node
// group, to version.  A control plane already past version, which EKS
// upgraded on its own while the node group stayed behind, is kept where it
// is, and only the node group catches up.  The stack keeps the cluster's
// upgrade policy, as a resumed create does, and the roles the create gave
// the cluster and its node group.
func (i *KubernetesRuntimeInfraEKS) UpgradeInfra(version string) error {
	i.KubernetesVersion = version
	controlPlane, err := eksControlPlaneVersion(i.AwsConfig, i.RuntimeInstanceName)
	if err != nil {
		return err
	}
	if i.controlPlaneVersion, err = controlPlaneVersionForUpgrade(controlPlane, version); err != nil {
		return err
	}
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

// eksControlPlaneVersion reads the EKS control plane's minor version.  A test
// stands in for it.
var eksControlPlaneVersion = tpaws.EksClusterVersion

// controlPlaneVersionForUpgrade returns the version to keep the control plane
// on while the cluster is upgraded to version: its own when it is already
// past version, as a downgrade would be refused, and "" to upgrade it to
// version with the node group otherwise.
func controlPlaneVersionForUpgrade(controlPlane, version string) (string, error) {
	comparison, err := api_v0.CompareKubernetesVersions(controlPlane, version)
	if err != nil {
		return "", err
	}
	if comparison > 0 {
		return controlPlane, nil
	}

	return "", nil
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
