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
	aws_config "github.com/nukleros/aws-builder/pkg/config"
	builder_iam "github.com/nukleros/aws-builder/pkg/iam"
	"github.com/spf13/cobra"

	"github.com/threeport/threeport/internal/provider"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	cli "github.com/threeport/threeport/pkg/cli/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	threeport "github.com/threeport/threeport/pkg/threeport-installer/v0"
)

var awsAccountName string
var awsProfile string
var awsRegion string
var providerRegion string
var roleName string
var awsAccountId string
var defaultAccount bool
var externalRoleName string

// ConfigCurrentInstanceCmd represents the current-instance command
var ConfigAwsCloudAccountCmd = &cobra.Command{
	Use:     "aws-account",
	Example: "tptctl config aws-account --aws-account-name my-account --aws-region us-east-1 --aws-profile my-profile --aws-account-id 123456789012",
	Short:   "Configure an aws account",
	Long: `Configure AWS account permissions. This creates an account in the Threeport API
	and the configures the respective customer-managed AWS account.`,
	SilenceUsage: true,
	PreRun:       CommandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, requestedControlPlane := GetClientContext(cmd)

		// load AWS configuration
		awsConf, err := aws_config.LoadAWSConfig(
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

		// if runtime manager role name is not provided, generate it
		if roleName == "" {
			roleName = provider.GetResourceManagerRoleName(requestedControlPlane)
		}

		// ensure role doesn't exist
		var nse types.NoSuchEntityException
		var existingRole *iam.GetRoleOutput
		svcIam := iam.NewFromConfig(*awsConf)
		if existingRole, err = svcIam.GetRole(
			context.Background(),
			&iam.GetRoleInput{
				RoleName: &roleName,
			},
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
			Name:           ptr.String(awsAccountName),
			AccountID:      callerIdentity.Account,
			DefaultAccount: ptr.Bool(defaultAccount),
			DefaultRegion:  ptr.String(awsRegion),
		}
		createdAwsAccount, err := client.CreateAwsAccount(apiClient, apiEndpoint, &awsAccount)
		if err != nil {
			cli.Error("failed to create aws account", err)
			os.Exit(1)
		}

		// if external role name is not provided, generate it
		if externalRoleName == "" {
			externalRoleName = provider.GetResourceManagerRoleName(requestedControlPlane)
		}

		// create resource manager role
		role, err := provider.CreateResourceManagerRole(
			threeport.ControlPlaneNamespace,
			builder_iam.CreateIamTags(
				requestedControlPlane,
				map[string]string{},
			),
			roleName,
			*callerIdentity.Account,
			awsAccountId,
			externalRoleName,
			*createdAwsAccount.ExternalId,
			true,
			true,
			*awsConf,
			make([]string, 0),
		)
		if err != nil {
			cli.Error("failed to create role", err)

			_, err = client.DeleteAwsAccount(apiClient, apiEndpoint, *createdAwsAccount.ID)
			if err != nil {
				cli.Error("failed to delete aws account", err)
			}
			os.Exit(1)
		}

		// update aws account with role arn
		createdAwsAccount.RoleArn = role.Arn
		_, err = client.UpdateAwsAccount(apiClient, apiEndpoint, createdAwsAccount)
		if err != nil {
			cli.Error("failed to update aws account", err)

			_, err = client.DeleteAwsAccount(apiClient, apiEndpoint, *createdAwsAccount.ID)
			if err != nil {
				cli.Error("failed to delete aws account", err)
			}

			err = provider.DeleteResourceManagerRole(roleName, *awsConf)
			if err != nil {
				cli.Error("failed to delete threeport role", err)
			}
			os.Exit(1)
		}

		cli.Complete(fmt.Sprintf("Configured AWS account with runtime manager role: %s", *role.Arn))
	},
}

func init() {
	ConfigCmd.AddCommand(ConfigAwsCloudAccountCmd)

	ConfigAwsCloudAccountCmd.Flags().StringVar(
		&awsAccountName,
		"aws-account-name",
		"",
		"The name of the AwsAccount object to create in the Threeport API.",
	)
	ConfigAwsCloudAccountCmd.Flags().StringVar(
		&awsRegion,
		"aws-region",
		"",
		"AWS region code to install threeport runtimes in.",
	)
	ConfigAwsCloudAccountCmd.Flags().StringVar(
		&awsProfile,
		"aws-profile",
		"",
		"The AWS profile to use. Defaults to the default profile.",
	)
	ConfigAwsCloudAccountCmd.Flags().StringVar(
		&roleName,
		"runtime-manager-role-name",
		"",
		fmt.Sprintf("The name of the runtime manager role to create. Defaults to %s-<instance-name>", provider.ResourceManagerRoleName),
	)
	ConfigAwsCloudAccountCmd.Flags().StringVar(
		&externalRoleName,
		"external-runtime-manager-role-name",
		"",
		fmt.Sprintf("The name of the external runtime manager role to allow access into this account. Defaults to %s-<instance-name>", provider.ResourceManagerRoleName),
	)
	ConfigAwsCloudAccountCmd.Flags().StringVar(
		&awsAccountId,
		"aws-account-id",
		"",
		"The external account to grant access to.",
	)
	ConfigAwsCloudAccountCmd.Flags().BoolVar(
		&defaultAccount,
		"default-account",
		false,
		"Set whether the created AwsAccount object in Threeport should be used by default.",
	)
	ConfigAwsCloudAccountCmd.MarkFlagRequired("aws-account-name")
	ConfigAwsCloudAccountCmd.MarkFlagRequired("aws-region")
	ConfigAwsCloudAccountCmd.MarkFlagRequired("aws-profile")
	ConfigAwsCloudAccountCmd.MarkFlagRequired("aws-account-id")
}
