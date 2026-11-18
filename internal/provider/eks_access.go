package provider

import (
	"fmt"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/cloudwatch"
	awseks "github.com/pulumi/pulumi-aws/sdk/v6/go/aws/eks"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	// eksAuditLogRetentionDays is how long CloudWatch Logs keeps a cluster's
	// audit log.  EKS creates its log group with no expiry, so the stack
	// creates it first with this one.
	eksAuditLogRetentionDays = 90

	// eksAuthenticationMode is how a cluster created with access entries
	// authorizes IAM principals: by its access entries alone, with no
	// aws-auth ConfigMap.
	eksAuthenticationMode = "API"
)

// eksAuditLogTypes are the control plane logs an audit log sends: the API
// server's requests, its audit log, and the IAM authenticator's decisions.
var eksAuditLogTypes = []string{"api", "audit", "authenticator"}

// clusterLogTypes returns the control plane logs the cluster sends to
// CloudWatch Logs: the audit log's, or none.
func (i *KubernetesRuntimeInfraEKS) clusterLogTypes() pulumi.StringArrayInput {
	if !i.AuditLog {
		return nil
	}

	return pulumi.ToStringArray(eksAuditLogTypes)
}

// eksAuditLogGroupName is the CloudWatch Logs group EKS sends a cluster's
// control plane logs to.
func eksAuditLogGroupName(clusterName string) string {
	return fmt.Sprintf("/aws/eks/%s/cluster", clusterName)
}

// auditLogGroup adds the log group the cluster's audit log goes to, when it
// has one, and returns what the cluster depends on.  It is created before
// the cluster, under the name EKS uses, so EKS writes to it rather than
// create one that keeps its logs forever.  It is deleted with the stack.
func (i *KubernetesRuntimeInfraEKS) auditLogGroup(ctx *pulumi.Context) ([]pulumi.Resource, error) {
	if !i.AuditLog {
		return nil, nil
	}
	logGroup, err := cloudwatch.NewLogGroup(ctx, fmt.Sprintf("%s-audit-log", i.RuntimeInstanceName), &cloudwatch.LogGroupArgs{
		Name:            pulumi.String(eksAuditLogGroupName(i.RuntimeInstanceName)),
		RetentionInDays: pulumi.Int(eksAuditLogRetentionDays),
		Tags:            i.resourceTags(eksAuditLogGroupName(i.RuntimeInstanceName)),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create the audit log's log group: %w", err)
	}

	return []pulumi.Resource{logGroup}, nil
}

// usesAccessEntries reports whether the cluster's access is through access
// entries: a new cluster's is, and a cluster the stack holds keeps what it
// was created with.
func (i *KubernetesRuntimeInfraEKS) usesAccessEntries() bool {
	return i.stackAccessEntries == nil || *i.stackAccessEntries
}

// accessConfig returns how the cluster authorizes IAM principals: by access
// entries alone, with no aws-auth ConfigMap.  The principal that creates the
// cluster, the IAM role Threeport provisions and reaches the cluster as, gets
// administrator access as an access entry EKS creates with the cluster, so
// Threeport's access shows among the cluster's access entries and can be
// revoked, rather than being hidden in the cluster's creation.  A cluster
// created before access entries keeps its configuration.
func (i *KubernetesRuntimeInfraEKS) accessConfig() *awseks.ClusterAccessConfigArgs {
	if !i.usesAccessEntries() {
		return nil
	}

	return &awseks.ClusterAccessConfigArgs{
		AuthenticationMode:                      pulumi.String(eksAuthenticationMode),
		BootstrapClusterCreatorAdminPermissions: pulumi.Bool(true),
	}
}
