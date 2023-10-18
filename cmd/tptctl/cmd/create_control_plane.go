/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"

	cli "github.com/threeport/threeport/pkg/cli/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	config "github.com/threeport/threeport/pkg/config/v0"
)

var createControlPlaneConfigPath string

// CreateControlPlaneCmd represents the create threeport command
var CreateControlPlaneCmd = &cobra.Command{
	Use:          "control-plane",
	Example:      "tptctl create control-plane --c my-threeport.yaml",
	Short:        "Create a new Threeport control plane",
	Long:         `Create a new control plane.`,
	SilenceUsage: true,
	Run: func(cmd *cobra.Command, args []string) {
		threeportConfig, requestedControlPlane, err := config.GetThreeportConfig(cliArgs.ControlPlaneName)
		if err != nil {
			cli.Error("failed to get threeport config", err)
			os.Exit(1)
		}

		apiEndpoint, err := threeportConfig.GetThreeportAPIEndpoint(requestedControlPlane)
		if err != nil {
			cli.Error("failed to get threeport API endpoint from config", err)
			os.Exit(1)
		}
		// get threeport API client
		cliArgs.AuthEnabled, err = threeportConfig.GetThreeportAuthEnabled(requestedControlPlane)
		if err != nil {
			cli.Error("failed to determine if auth is enabled on threeport API", err)
			os.Exit(1)
		}
		ca, clientCertificate, clientPrivateKey, err := threeportConfig.GetThreeportCertificatesForControlPlane(requestedControlPlane)
		if err != nil {
			cli.Error("failed to get threeport certificates from config", err)
			os.Exit(1)
		}
		apiClient, err := client.GetHTTPClient(cliArgs.AuthEnabled, ca, clientCertificate, clientPrivateKey, "")
		if err != nil {
			cli.Error("failed to create https client", err)
			os.Exit(1)
		}

		// load control plane config
		configContent, err := ioutil.ReadFile(createControlPlaneConfigPath)
		if err != nil {
			cli.Error("failed to read config file", err)
			os.Exit(1)
		}
		var controlPlaneConfig config.ControlPlaneConfig
		if err := yaml.Unmarshal(configContent, &controlPlaneConfig); err != nil {
			cli.Error("failed to unmarshal config file yaml content", err)
			os.Exit(1)
		}
		// create control plane
		controlPlane := controlPlaneConfig.ControlPlane
		cd, ci, err := controlPlane.Create(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to create control plane", err)
			os.Exit(1)
		}

		cli.Info(fmt.Sprintf("control plane definition %s created", *cd.Name))
		cli.Info(fmt.Sprintf("control plane instance %s created", *ci.Name))
		cli.Complete(fmt.Sprintf("control plane %s created", controlPlane.Name))

	},
}

func init() {
	createCmd.AddCommand(CreateControlPlaneCmd)
	CreateControlPlaneCmd.Flags().StringVarP(
		&createControlPlaneConfigPath,
		"config", "c", "", "Path to file with control plane config.",
	)
<<<<<<< HEAD
	CreateControlPlaneCmd.MarkFlagRequired("config")
=======
	CreateControlPlaneCmd.MarkFlagRequired("name")
	CreateControlPlaneCmd.Flags().StringVarP(
		&cliArgs.InfraProvider,
		"provider", "p", "kind", fmt.Sprintf("The infrasture provider to install upon. Supported infra providers: %s", v0.SupportedInfraProviders()),
	)
	// this flag will be enabled once production-ready control plane instances
	// are available.
	//CreateControlPlaneCmd.Flags().StringVarP(
	//	&tier,
	//	"tier", "t", threeport.ControlPlaneTierDev, "Determines the level of availability and data retention for the control plane.",
	//)
	CreateControlPlaneCmd.Flags().StringVar(
		&cliArgs.KubeconfigPath,
		"kind-kubeconfig", "", "Path to kubeconfig used for kind provider installs (default is ~/.kube/config).",
	)
	CreateControlPlaneCmd.Flags().StringVar(
		&cliArgs.AwsConfigProfile,
		"aws-config-profile", "default", "The AWS config profile to draw credentials from when using eks provider.",
	)
	CreateControlPlaneCmd.Flags().BoolVar(
		&cliArgs.AwsConfigEnv,
		"aws-config-env", false, "Retrieve AWS credentials from environment variables when using eks provider.",
	)
	CreateControlPlaneCmd.Flags().StringVar(
		&cliArgs.AwsRegion,
		"aws-region", "", "AWS region code to install threeport in when using eks provider. If provided, will take precedence over AWS config profile and environment variables.",
	)
	CreateControlPlaneCmd.PersistentFlags().StringVarP(
		&cliArgs.AwsRoleArn,
		"aws-role-arn", "r", "", "The AWS role ARN to assume when provisioning resources",
	)
	CreateControlPlaneCmd.PersistentFlags().StringVarP(
		&cliArgs.AwsSerialNumber,
		"aws-serial-number", "s", "", "The AWS serial number to use when authenticating via MFA",
	)
	CreateControlPlaneCmd.Flags().BoolVar(
		&cliArgs.ForceOverwriteConfig,
		"force-overwrite-config", false, "Force the overwrite of an existing Threeport instance config.  Warning: this will erase the connection info for the existing instance.  Only do this if the existing instance has already been deleted and is no longer in use.",
	)
	CreateControlPlaneCmd.Flags().BoolVar(
		&cliArgs.AuthEnabled,
		"auth-enabled", true, "Enable client certificate authentication. Can only be disabled when using kind provider.",
	)
	CreateControlPlaneCmd.Flags().StringVar(
		&cliArgs.CreateRootDomain,
		"root-domain", "", "The root domain name to use for the Threeport API. Requires a public hosted zone in AWS Route53. A subdomain for the Threeport API will be added to the root domain.",
	)
	CreateControlPlaneCmd.Flags().StringVar(
		&cliArgs.CreateAdminEmail,
		"admin-email", "", "Email address of control plane admin.  Provided to TLS provider.",
	)
	CreateControlPlaneCmd.Flags().StringVar(
		&cliArgs.ControlPlaneImageRepo,
		"control-plane-image-repo", "", "Alternate image repo to pull threeport control plane images from.",
	)
	CreateControlPlaneCmd.Flags().StringVar(
		&cliArgs.ControlPlaneImageTag,
		"control-plane-image-tag", "", "Alternate image tag to pull threeport control plane images from.",
	)
	CreateControlPlaneCmd.Flags().IntVar(
		&cliArgs.NumWorkerNodes,
		"num-worker-nodes", 0, "Number of additional worker nodes to deploy. Only applies to kind provider. (default is 0)")
>>>>>>> 7b3a5ce (feat: remove need for --provider-account-id)
}
