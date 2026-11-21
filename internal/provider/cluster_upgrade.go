package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"

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

// UpgradeInfra upgrades the EKS cluster's control plane, then its node
// groups, then its add-ons, to version.  A control plane already past
// version, which EKS upgraded on its own while the node groups stayed
// behind, is kept where it is, as are the add-ons, which follow the control
// plane, and a node group already past it.  The stack keeps the cluster's
// upgrade policy, access configuration, zones and the range its network
// was laid out in, as a resumed create does, and the roles the create gave
// the cluster and its node group.
func (i *KubernetesRuntimeInfraEKS) UpgradeInfra(version string) error {
	i.KubernetesVersion = version
	controlPlane, err := eksControlPlaneVersion(i.AwsConfig, i.RuntimeInstanceName)
	if err != nil {
		return err
	}
	if i.controlPlaneVersion, err = versionKeptForUpgrade(controlPlane, version); err != nil {
		return err
	}
	// a node group already past the version keeps its own, as the control
	// plane does
	i.nodeGroupVersions = map[string]string{}
	for _, nodeGroupName := range i.nodeGroupNames() {
		nodeGroup, err := eksNodeGroupVersion(i.AwsConfig, i.RuntimeInstanceName, nodeGroupName)
		if err != nil {
			return err
		}
		keep, err := versionKeptForUpgrade(nodeGroup, version)
		if err != nil {
			return err
		}
		if keep != "" {
			i.nodeGroupVersions[nodeGroupName] = keep
		}
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
	i.keepStackCluster(exported.Deployment)

	if err := i.upgradeStack(stack); err != nil {
		return err
	}

	return i.upgradeEbsStorageAddon(i.controlPlaneKubernetesVersion())
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
// maintenance window.  The stack keeps the cluster's release channel, the
// range its network was laid out in, its location, its node limits, and the
// node pool's service account and boot disk, as a resumed create does.  It
// also applies the secrets encryption, boot disk, Secure Boot and intranode
// visibility the definition asks for, which run on the version the cluster
// runs: a node pool whose boot disk key or Secure Boot changes is replaced
// by a new one, unless DeletionProtection is set, when the upgrade is
// refused.
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
	// the cluster stays where it is, and its node pool keeps its zones and
	// how its autoscaler limits the nodes
	i.keepStackCluster(exported.Deployment)

	allowed, err := i.allowNodePoolReplacement(stack, exported.Deployment)
	if err != nil {
		return err
	}

	return i.upgradeStackAllowing(stack, allowed)
}

// allowNodePoolReplacement returns the deletions the upgrade makes of the
// node pools the stack holds that the program replaces with a node pool of
// another name, as it does one whose boot disk key or Secure Boot the
// definition changed: the new node pool is created first, and the old one is
// deleted once it is, its pods moving to the new one.  The old ones are no
// longer protected from deletion, as they were if they were created with
// DeletionProtection, which is since turned off.  While DeletionProtection
// is set, it returns an UpgradeRefusedError, before the preview, which
// Pulumi fails rather than show the deletion of a protected node pool.
func (i *KubernetesRuntimeInfraGKE) allowNodePoolReplacement(stack auto.Stack, deployment json.RawMessage) ([]string, error) {
	replaced, err := i.replaceableNodePools(deployment)
	if err != nil || len(replaced) == 0 {
		return nil, err
	}
	if err := unprotectStackResources(context.Background(), stack, func(urn string) bool {
		return slices.Contains(replaced, urn)
	}); err != nil {
		return nil, fmt.Errorf("failed to remove the deletion protection of the node pool to replace: %w", err)
	}

	return nodePoolDeletions(replaced), nil
}

// nodePoolDeletions returns the deletions of the node pools of URNs, as an
// upgrade's preview describes them.
func nodePoolDeletions(urns []string) []string {
	deletions := make([]string, 0, len(urns))
	for _, urn := range urns {
		deletions = append(deletions, fmt.Sprintf("delete %s %s", gkeNodePoolTypeToken, urn[strings.LastIndex(urn, "::")+len("::"):]))
	}

	return deletions
}

// replaceableNodePools returns the URNs of the node pools an exported
// deployment holds that the program replaces, and an UpgradeRefusedError
// naming their deletions when DeletionProtection is set, which protects
// them.
func (i *KubernetesRuntimeInfraGKE) replaceableNodePools(deployment json.RawMessage) ([]string, error) {
	replaced := i.replacedNodePools(deployment)
	if len(replaced) > 0 && i.DeletionProtection {
		return nil, &UpgradeRefusedError{Steps: nodePoolDeletions(replaced)}
	}

	return replaced, nil
}

// replacedNodePools returns the URNs of the node pools an exported deployment
// holds that the program doesn't make: those of another name than the node
// pool it does.
func (i *KubernetesRuntimeInfraGKE) replacedNodePools(deployment json.RawMessage) []string {
	var contents struct {
		Resources []struct {
			Urn  string `json:"urn"`
			Type string `json:"type"`
		} `json:"resources"`
	}
	if len(deployment) == 0 || json.Unmarshal(deployment, &contents) != nil {
		return nil
	}

	var replaced []string
	for _, resource := range contents.Resources {
		if resource.Type != gkeNodePoolTypeToken || resource.Urn == "" {
			continue
		}
		if resource.Urn[strings.LastIndex(resource.Urn, "::")+len("::"):] != i.workerNodePoolName() {
			replaced = append(replaced, resource.Urn)
		}
	}

	return replaced
}

// eksControlPlaneVersion reads the EKS control plane's minor version.  A test
// stands in for it.
var eksControlPlaneVersion = tpaws.EksClusterVersion

// versionKeptForUpgrade returns the version to keep the control plane, or a
// node group, on while the cluster is upgraded to version: its own, current,
// when it is already past version, as a downgrade would be refused, and "" to
// upgrade it to version otherwise.
func versionKeptForUpgrade(current, version string) (string, error) {
	comparison, err := api_v0.CompareKubernetesVersions(current, version)
	if err != nil {
		return "", err
	}
	if comparison > 0 {
		return current, nil
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
