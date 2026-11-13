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
// IAM, so while the project holds a private cluster Threeport provisioned,
// the control plane's namespace holds a role with it on the project.  The
// role lets a caller see clusters and connect to them; what it may do inside
// a cluster is still RBAC's to decide.
//
// The grant is on the project because it cannot be narrowed to one cluster:
// GKE has no IAM policy of its own on a cluster, and a project binding whose
// IAM condition names the cluster by resource type and name did not let a
// caller connect to it, 17 minutes after it was made.
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

// clusterConnectClientOptions authenticates as the runtime's own credentials
// when it has them, and as the process's otherwise.
func (i *KubernetesRuntimeInfraGKE) clusterConnectClientOptions(scope string) []option.ClientOption {
	clientOpts := []option.ClientOption{option.WithScopes(scope)}
	if i.ServiceAccountCredentials != "" {
		clientOpts = append(clientOpts, option.WithCredentialsJSON([]byte(i.ServiceAccountCredentials)))
	}

	return clientOpts
}

// clusterConnectPolicies returns the project policy store and the member the
// role is granted to.
func (i *KubernetesRuntimeInfraGKE) clusterConnectPolicies() (projectPolicies, string, error) {
	crmService, err := cloudresourcemanager.NewService(
		context.Background(),
		i.clusterConnectClientOptions(cloudresourcemanager.CloudPlatformScope)...,
	)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create Cloud Resource Manager service client: %w", err)
	}
	projectNumber, err := i.projectNumber(crmService)
	if err != nil {
		return nil, "", err
	}

	return crmProjectPolicies{service: crmService}, controlPlaneConnectMember(projectNumber, i.ProjectID), nil
}

// grantControlPlaneClusterConnect lets the control plane's controllers reach
// a private cluster through its DNS endpoint.  A cluster with a public IP
// endpoint is reached there, without the IAM check, and grants nothing.  It
// is granted once the cluster, and with it the project's Workload Identity
// pool, exists.
func (i *KubernetesRuntimeInfraGKE) grantControlPlaneClusterConnect() error {
	if !i.PrivateApi {
		return nil
	}
	policies, member, err := i.clusterConnectPolicies()
	if err != nil {
		return err
	}

	return grantClusterConnect(policies, i.ProjectID, member)
}

// controlPlaneClusterConnectGranted reports whether the control plane's
// controllers hold the connect role a private cluster's create grants.  A
// cluster with a public IP endpoint needs none.
func (i *KubernetesRuntimeInfraGKE) controlPlaneClusterConnectGranted() (bool, error) {
	if !i.PrivateApi {
		return true, nil
	}
	policies, member, err := i.clusterConnectPolicies()
	if err != nil {
		return false, err
	}

	return clusterConnectGranted(policies, i.ProjectID, member)
}

// clusterConnectGranted reports whether member holds gkeClusterConnectRole on
// a project, as grantClusterConnect binds it: unconditionally.
func clusterConnectGranted(policies projectPolicies, projectID, member string) (bool, error) {
	policy, err := policies.get(projectID)
	if err != nil {
		return false, fmt.Errorf("failed to get the IAM policy of project %s: %w", projectID, err)
	}
	for _, binding := range policy.Bindings {
		if binding.Role != gkeClusterConnectRole || binding.Condition != nil {
			continue
		}
		for _, m := range binding.Members {
			if m == member {
				return true, nil
			}
		}
	}

	return false, nil
}

// revokeControlPlaneClusterConnect removes the control plane's connect role
// once the project holds no private cluster Threeport provisioned other than
// this one.  The member is the namespace on every cluster in the project, so
// it is shared by every control plane there that uses the same namespace.
func (i *KubernetesRuntimeInfraGKE) revokeControlPlaneClusterConnect() error {
	policies, member, err := i.clusterConnectPolicies()
	if err != nil {
		return err
	}
	listClusters := func() ([]*containerpb.Cluster, error) {
		ctx := context.Background()
		clusterManagerClient, err := container.NewClusterManagerClient(
			ctx,
			i.clusterConnectClientOptions(cloudresourcemanager.CloudPlatformScope)...,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create cluster manager client: %w", err)
		}
		defer clusterManagerClient.Close()

		response, err := clusterManagerClient.ListClusters(ctx, &containerpb.ListClustersRequest{
			Parent: fmt.Sprintf("projects/%s/locations/-", i.ProjectID),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list the project's GKE clusters: %w", err)
		}

		return completeClusterList(response)
	}

	return revokeClusterConnect(policies, i.ProjectID, member, listClusters, i.RuntimeInstanceName, i.Region)
}

// completeClusterList returns the clusters a listing found, or an error when
// GKE could not reach some of the project's locations: a cluster there would
// be missing from the list, and the role revoked from under it.
func completeClusterList(response *containerpb.ListClustersResponse) ([]*containerpb.Cluster, error) {
	if missing := response.GetMissingZones(); len(missing) > 0 {
		return nil, fmt.Errorf("GKE could not list the clusters in %v, so the control plane's connect role is kept until it can", missing)
	}

	return response.GetClusters(), nil
}

// grantClusterConnect binds member to gkeClusterConnectRole on a project.
//
// The policy is written even when member already holds the role.  A
// concurrent revoke decides from the clusters it lists after reading the
// policy, and the write changes the policy's etag, so a revoke that listed
// before this cluster existed fails on its stale etag and decides again, and
// one that lists after sees it.
func grantClusterConnect(policies projectPolicies, projectID, member string) error {
	if _, err := updateProjectPolicy(policies, projectID, isTransientIAMError, func(p *cloudresourcemanager.Policy) bool {
		addMemberToPolicy(p, member, []string{gkeClusterConnectRole})
		return true
	}); err != nil {
		return fmt.Errorf("failed to let the control plane connect to the project's private GKE clusters: %w", err)
	}

	return nil
}

// revokeClusterConnect removes member from gkeClusterConnectRole on a
// project, leaving any other role it holds, unless the project holds a
// private cluster Threeport provisioned other than the one named name in
// location.  The clusters are listed after each read of the policy, so the
// decision is made against the policy it is written to; see
// grantClusterConnect.
func revokeClusterConnect(
	policies projectPolicies,
	projectID, member string,
	listClusters func() ([]*containerpb.Cluster, error),
	name, location string,
) error {
	var listErr error
	if _, err := updateProjectPolicy(policies, projectID, isTransientIAMError, func(p *cloudresourcemanager.Policy) bool {
		if !removeMemberFromRole(p, member, gkeClusterConnectRole) {
			return false
		}
		clusters, err := listClusters()
		if err != nil {
			listErr = err
			return false
		}

		return !privateThreeportClusterOtherThan(clusters, name, location)
	}); err != nil {
		return fmt.Errorf("failed to remove the control plane's connect role from project %s: %w", projectID, err)
	}

	return listErr
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

// privateThreeportClusterOtherThan reports whether clusters holds a private
// one Threeport provisioned other than the one named name in the region
// location.  Clusters in different regions may share a name, so the region
// tells them apart; the cluster itself is in the region, or in one of its
// zones when it is zonal.  A cluster still being created counts, as it may
// already be one whose create granted the role.
func privateThreeportClusterOtherThan(clusters []*containerpb.Cluster, name, location string) bool {
	for _, cluster := range clusters {
		if cluster.GetResourceLabels()[GcpLabelProvisionedBy] != GcpLabelProvisionedByValue {
			continue
		}
		if cluster.GetName() == name && gkeLocationInRegion(cluster.GetLocation(), location) {
			continue
		}
		if !gkeHasPublicIpEndpoint(cluster) {
			return true
		}
	}

	return false
}
