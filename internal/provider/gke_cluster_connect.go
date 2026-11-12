package provider

import (
	"context"
	"fmt"

	container "cloud.google.com/go/container/apiv1"
	"cloud.google.com/go/container/apiv1/containerpb"
	cloudresourcemanager "google.golang.org/api/cloudresourcemanager/v1"
	"google.golang.org/api/option"

	installer "github.com/threeport/threeport/pkg/threeport-installer/v0"
)

// The controllers of a control plane on GKE reach the GKE clusters it manages
// in the same project as their own Workload Identity principals, which
// Kubernetes RBAC on each cluster authorizes.  A private cluster is reached
// through its DNS endpoint, which also requires container.clusters.connect in
// IAM, so the control plane's namespace is granted a role with it on the
// project.  The role lets a caller see clusters and connect to them; what it
// may do inside a cluster is still RBAC's to decide.
const (
	// gkeClusterConnectRole is the narrowest predefined role with
	// container.clusters.connect: that, container.clusters.get and list, and
	// reading the project.
	gkeClusterConnectRole = "roles/container.clusterViewer"

	// workloadIdentityNamespaceFormat is the Workload Identity Federation
	// principal set of every Kubernetes service account in one namespace, by
	// project number, project ID and namespace.  The pool is the project's,
	// so it holds that namespace on every cluster in the project.
	workloadIdentityNamespaceFormat = "principalSet://iam.googleapis.com/projects/%d/locations/global/workloadIdentityPools/%s.svc.id.goog/namespace/%s"
)

// controlPlaneConnectMember is the IAM member of the control plane's
// controllers in a project.
func controlPlaneConnectMember(projectNumber int64, projectID string) string {
	return fmt.Sprintf(workloadIdentityNamespaceFormat, projectNumber, projectID, installer.ControlPlaneNamespace)
}

// grantControlPlaneClusterConnect lets the control plane's controllers reach
// the project's private clusters through their DNS endpoints.  It is granted
// once the cluster, and with it the project's Workload Identity pool, exists.
func (i *KubernetesRuntimeInfraGKE) grantControlPlaneClusterConnect() error {
	crmService, err := cloudresourcemanager.NewService(
		context.Background(),
		option.WithScopes(cloudresourcemanager.CloudPlatformScope),
	)
	if err != nil {
		return fmt.Errorf("failed to create Cloud Resource Manager service client: %w", err)
	}
	projectNumber, err := i.projectNumber(crmService)
	if err != nil {
		return err
	}

	return grantClusterConnect(crmProjectPolicies{service: crmService}, i.ProjectID, controlPlaneConnectMember(projectNumber, i.ProjectID))
}

// grantClusterConnect binds member to gkeClusterConnectRole on a project.
func grantClusterConnect(policies projectPolicies, projectID, member string) error {
	if _, err := updateProjectPolicy(policies, projectID, isTransientIAMError, func(p *cloudresourcemanager.Policy) bool {
		return addMemberToPolicy(p, member, []string{gkeClusterConnectRole})
	}); err != nil {
		return fmt.Errorf("failed to let the control plane connect to the project's GKE clusters: %w", err)
	}

	return nil
}

// revokeControlPlaneClusterConnect removes the control plane's connect role
// once the project holds no cluster Threeport provisioned.  The member is the
// namespace on every cluster in the project, so it is shared by every control
// plane there that uses the same namespace, and one that may still reach a
// cluster keeps it.
func (i *KubernetesRuntimeInfraGKE) revokeControlPlaneClusterConnect(
	crmService *cloudresourcemanager.Service,
	clientOpts []option.ClientOption,
) error {
	remaining, err := i.otherThreeportClustersRemain(clientOpts)
	if err != nil {
		return err
	}
	if remaining {
		return nil
	}
	projectNumber, err := i.projectNumber(crmService)
	if err != nil {
		return err
	}

	return revokeClusterConnect(crmProjectPolicies{service: crmService}, i.ProjectID, controlPlaneConnectMember(projectNumber, i.ProjectID))
}

// revokeClusterConnect removes member from gkeClusterConnectRole on a project,
// leaving any other role it holds.
func revokeClusterConnect(policies projectPolicies, projectID, member string) error {
	if _, err := updateProjectPolicy(policies, projectID, isTransientIAMError, func(p *cloudresourcemanager.Policy) bool {
		return removeMemberFromRole(p, member, gkeClusterConnectRole)
	}); err != nil {
		return fmt.Errorf("failed to remove the control plane's connect role from project %s: %w", projectID, err)
	}

	return nil
}

// removeMemberFromRole drops member from role's unconditional binding, and
// the binding when no member is left, reporting whether the policy changed.
func removeMemberFromRole(policy *cloudresourcemanager.Policy, member, role string) bool {
	changed := false
	kept := make([]*cloudresourcemanager.Binding, 0, len(policy.Bindings))
	for _, binding := range policy.Bindings {
		if binding.Role == role && binding.Condition == nil {
			members := make([]string, 0, len(binding.Members))
			for _, m := range binding.Members {
				if m != member {
					members = append(members, m)
				}
			}
			if len(members) != len(binding.Members) {
				changed = true
				binding.Members = members
			}
			if len(binding.Members) == 0 {
				continue
			}
		}
		kept = append(kept, binding)
	}
	policy.Bindings = kept

	return changed
}

// otherThreeportClustersRemain reports whether the project holds a GKE
// cluster Threeport provisioned other than this one.
func (i *KubernetesRuntimeInfraGKE) otherThreeportClustersRemain(clientOpts []option.ClientOption) (bool, error) {
	ctx := context.Background()
	clusterManagerClient, err := container.NewClusterManagerClient(ctx, clientOpts...)
	if err != nil {
		return false, fmt.Errorf("failed to create cluster manager client: %w", err)
	}
	defer clusterManagerClient.Close()

	response, err := clusterManagerClient.ListClusters(ctx, &containerpb.ListClustersRequest{
		Parent: fmt.Sprintf("projects/%s/locations/-", i.ProjectID),
	})
	if err != nil {
		return false, fmt.Errorf("failed to list the project's GKE clusters: %w", err)
	}

	return threeportClusterOtherThan(response.GetClusters(), i.RuntimeInstanceName), nil
}

// threeportClusterOtherThan reports whether clusters holds one Threeport
// provisioned that isn't named name.
func threeportClusterOtherThan(clusters []*containerpb.Cluster, name string) bool {
	for _, cluster := range clusters {
		if cluster.GetResourceLabels()[GcpLabelProvisionedBy] == GcpLabelProvisionedByValue && cluster.GetName() != name {
			return true
		}
	}

	return false
}
