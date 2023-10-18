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

var awsProfile string
var providerRegion string
var runtimeManagerRoleName string
var externalAwsAccountId string

// ConfigCurrentInstanceCmd represents the current-instance command
var ConfigAwsCloudAccountCmd = &cobra.Command{
	Use:     "aws-account",
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
		apiClient, err := threeportConfig.GetHTTPClient(requestedInstance)
		if err != nil {
			cli.Error("failed to get threeport API client", err)
			os.Exit(1)
		}

		// load AWS configuration
		awsConf, err := resource.LoadAWSConfig(
			false,
			awsProfile,
			providerRegion,
			"",
			"",
			"",
		)
		if err != nil {
			cli.Error("failed to load AWS configuration with local config", err)
			os.Exit(1)
		}

		// test AWS configuration by getting caller identity
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
		if runtimeManagerRoleName == "" {
			runtimeManagerRoleName = provider.GetResourceManagerRoleName(requestedInstance)
		}
		var existingRole *iam.GetRoleOutput
		getRoleInput := iam.GetRoleInput{
			RoleName: &runtimeManagerRoleName,
		}
		if existingRole, err = svcIam.GetRole(
			context.Background(),
			&getRoleInput,
		); err != nil && !provider.IsException(&err, nse.ErrorCode()) {
			cli.Error("failed to get role", err)
			os.Exit(1)
		}

		// if the role already exists, throw an error
		if err == nil {
			cli.Error("role already exists: ", fmt.Errorf("%s", *existingRole.Role.Arn))
			os.Exit(1)
		}

		// create aws account in threeport API to generate an external ID value
		awsAccount := v0.AwsAccount{
			Name:           ptr.String("my-account"),
			AccountID:      callerIdentity.Account,
			DefaultAccount: ptr.Bool(true),
			DefaultRegion:  ptr.String("us-east-1"),
		}
		createdAwsAccount, err := client.CreateAwsAccount(apiClient, apiEndpoint, &awsAccount)
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
			externalAwsAccountId,
			*createdAwsAccount.ExternalId,
			*awsConf,
		)
		if err != nil {
			cli.Error("failed to create role", err)
			os.Exit(1)
		}

		// update aws account with role arn
		createdAwsAccount.RoleArn = role.Arn
		_, err = client.UpdateAwsAccount(apiClient, apiEndpoint, createdAwsAccount)
		if err != nil {
			cli.Error("failed to update aws account", err)
			os.Exit(1)
		}

		cli.Complete(fmt.Sprintf("Configured AWS cloud account with runtime manager role: %s", *role.Arn))
	},
}

func init() {
	configCmd.AddCommand(ConfigAwsCloudAccountCmd)

	ConfigAwsCloudAccountCmd.Flags().StringVar(
		&runtimeManagerRoleName,
		"runtime-manager-role-name",
		"",
		fmt.Sprintf("The name of the runtime manager role to create. Defaults to %s-<instance-name>", provider.ResourceManagerRoleName),
	)
	ConfigAwsCloudAccountCmd.Flags().StringVar(
		&awsProfile,
		"aws-profile",
		"",
		"The AWS profile to use. Defaults to the default profile.",
	)
	ConfigAwsCloudAccountCmd.Flags().StringVar(
		&externalAwsAccountId,
		"external-account-id",
		"",
		"The AWS profile to use. Defaults to the default profile.",
	)
	ConfigAwsCloudAccountCmd.MarkFlagRequired("aws-profile")
	ConfigAwsCloudAccountCmd.MarkFlagRequired("external-account-id")
}
