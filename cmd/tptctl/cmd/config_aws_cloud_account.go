/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go/ptr"
	"github.com/nukleros/eks-cluster/pkg/resource"
	"github.com/spf13/cobra"
	"github.com/threeport/threeport/internal/provider"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	cli "github.com/threeport/threeport/pkg/cli/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	config "github.com/threeport/threeport/pkg/config/v0"
)

var providerAccountProfile string
var providerRegion string
var externalAwsAccountId int
var runtimeManagerRoleName string

// ConfigCurrentInstanceCmd represents the current-instance command
var ConfigAwsCloudAccountCmd = &cobra.Command{
	Use:     "provider-account",
	Example: "tptctl config aws-account --name my-account",
	Short:   "Configure an aws account",
	Long: `Configure AWS account permissions. This ensures that
	a configured AWS account in Threeport can access and manage resources within
	the respective customer-managed AWS account.`,
	SilenceUsage: true,
	Run: func(cmd *cobra.Command, args []string) {

		// get threeport config and extract threeport API endpoint
		threeportConfig, requestedInstance, err := config.GetThreeportConfig(cliArgs.InstanceName)
		if err != nil {
			cli.Error("failed to get threeport config", err)
			os.Exit(1)
		}
		apiEndpoint, err := threeportConfig.GetThreeportAPIEndpoint(requestedInstance)
		if err != nil {
			cli.Error("failed to get threeport API endpoint from config", err)
			os.Exit(1)
		}

		// get threeport API client
		cliArgs.AuthEnabled, err = threeportConfig.GetThreeportAuthEnabled(requestedInstance)
		if err != nil {
			cli.Error("failed to determine if auth is enabled on threeport API", err)
			os.Exit(1)
		}
		ca, clientCertificate, clientPrivateKey, err := threeportConfig.GetThreeportCertificatesForInstance(requestedInstance)
		if err != nil {
			cli.Error("failed to get threeport certificates from config", err)
			os.Exit(1)
		}
		apiClient, err := client.GetHTTPClient(cliArgs.AuthEnabled, ca, clientCertificate, clientPrivateKey, "")
		if err != nil {
			cli.Error("failed to create threeport API client", err)
			os.Exit(1)
		}

		resourceManagerRoleName := provider.GetResourceManagerRoleName(requestedInstance)
		awsConf, err := resource.LoadAWSConfig(
			false,
			providerAccountProfile,
			providerRegion,
			"",
			"",
			"",
		)
		if err != nil {
			cli.Error("failed to load AWS configuration with local config", err)
			os.Exit(1)
		}

		svcSts := sts.NewFromConfig(*awsConf)
		callerIdentity, err := svcSts.GetCallerIdentity(
			context.Background(),
			&sts.GetCallerIdentityInput{},
		)
		if err != nil {
			cli.Error("failed to get caller identity", err)
			os.Exit(1)
		}

		svcIam := iam.NewFromConfig(*awsConf)
		var nse types.NoSuchEntityException

		// ensure role doesn't exist
		var existingRole *iam.GetRoleOutput
		if existingRole, err = svcIam.GetRole(
			context.Background(),
			&iam.GetRoleInput{RoleName: &resourceManagerRoleName},
		); err != nil && !provider.IsException(&err, nse.ErrorCode()) {
			cli.Error("failed to get role", err)
			os.Exit(1)
		}

		// if the role already exists, throw an error
		if err == nil {
			cli.Error("role already exists: ", fmt.Errorf("%s", *existingRole.Role.Arn))
			os.Exit(1)
		}

		createdAwsAccount, err := client.CreateAwsAccount(apiClient, apiEndpoint, &v0.AwsAccount{
			Name:           ptr.String("my-account"),
			AccountID:      callerIdentity.Account,
			DefaultAccount: ptr.Bool(true),
		})

		if err != nil {
			cli.Error("failed to create aws account", err)
			os.Exit(1)
		}

		// create resource manager role
		role, err := provider.CreateResourceManagerRole(
			resource.CreateIAMTags(
				requestedInstance,
				map[string]string{},
			),
			requestedInstance,
			*createdAwsAccount.AccountID,
			*createdAwsAccount.ExternalId,
			*awsConf,
		)
		if err != nil {
			cli.Error("failed to create role", err)
			os.Exit(1)
		}

		cli.Complete(fmt.Sprintf("Configured AWS cloud account with runtime manager role: %s", *role.Arn))
	},
}

func init() {
	configCmd.AddCommand(ConfigCurrentInstanceCmd)

	ConfigAwsCloudAccountCmd.Flags().StringVarP(
		&configCurrentInstanceName,
		"instance-name", "i", "", "The name of the Threeport instance to set as current.",
	)
	ConfigAwsCloudAccountCmd.Flags().StringVar(
		&runtimeManagerRoleName,
		"runtime-manage-role-name", "", fmt.Sprintf("The name of the runtime manager role to create. Defaults to %s-<instance-name>", provider.ResourceManagerRoleName),
	)
	ConfigAwsCloudAccountCmd.Flags().IntVar(
		&externalAwsAccountId,
		"aws-account-id", 0, "The AWS account ID of the external account.",
	)
	ConfigAwsCloudAccountCmd.MarkFlagRequired("instance-name")
}
