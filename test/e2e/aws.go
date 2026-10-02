package e2e_test

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awseks "github.com/aws/aws-sdk-go-v2/service/eks"

	tpaws "github.com/threeport/threeport/pkg/aws/v0"
)

// verifyEksIrsa checks that the EBS CSI driver addon is running under the IAM
// role bound to its Kubernetes service account.
//
// This is the end-to-end proof that IAM Roles for Service Accounts works on
// the cluster: the addon's pods only reach Running if the OIDC provider is
// registered and the role's trust policy names their service account.  A
// cluster can otherwise look healthy while every add-on that needs AWS
// credentials is failing.
func verifyEksIrsa(ctx context.Context, awsConfig aws.Config, clusterName string) error {
	addon, err := awseks.NewFromConfig(awsConfig).DescribeAddon(ctx, &awseks.DescribeAddonInput{
		ClusterName: aws.String(clusterName),
		AddonName:   aws.String(tpaws.EksEbsStorageAddonName),
	})
	if err != nil {
		return fmt.Errorf("failed to describe the %s addon: %w", tpaws.EksEbsStorageAddonName, err)
	}
	if status := string(addon.Addon.Status); status != "ACTIVE" {
		return fmt.Errorf(
			"the %s addon is %s rather than ACTIVE",
			tpaws.EksEbsStorageAddonName, status,
		)
	}
	serviceAccountRole := aws.ToString(addon.Addon.ServiceAccountRoleArn)
	if !strings.Contains(serviceAccountRole, tpaws.EksStorageManagementRoleName(clusterName)) {
		return fmt.Errorf(
			"the %s addon runs as %q rather than the cluster's storage management role",
			tpaws.EksEbsStorageAddonName, serviceAccountRole,
		)
	}

	return nil
}
