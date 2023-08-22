/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/threeport/threeport/internal/cli"
	"github.com/threeport/threeport/internal/threeport"
	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// TODO: will become a variable once production-ready control plane instances are
// available.
const tier = threeport.ControlPlaneTierDev

// CreateControlPlaneCmd represents the create threeport command
var CreateControlPlaneCmd = &cobra.Command{
	Use:          "control-plane",
	Example:      "tptctl create control-plane --name my-threeport",
	Short:        "Create a new instance of the Threeport control plane",
	Long:         `Create a new instance of the Threeport control plane.`,
	SilenceUsage: true,
	Run: func(cmd *cobra.Command, args []string) {
		err := cliArgs.CreateControlPlane()
		if err != nil {
			cli.Error("failed to create threeport control plane", err)
			if errors.Is(cli.ThreeportConfigAlreadyExistsErr, err) {
				cli.Info("if you wish to overwrite the existing config use --force-overwrite-config flag")
				cli.Warning("you will lose the ability to connect to the existing threeport instance if it is still running")
			}
			os.Exit(1)
		}
	},
}

func init() {
	createCmd.AddCommand(CreateControlPlaneCmd)

	cliArgs = &cli.ControlPlaneCLIArgs{}

	CreateControlPlaneCmd.Flags().StringVarP(
		&cliArgs.InstanceName,
		"name", "n", "", "Required. Name of control plane instance.",
	)
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
		&cliArgs.CreateProviderAccountID,
		"provider-account-id", "", "The provider account ID.  Required if providing a root domain for automated DNS management.",
	)
	CreateControlPlaneCmd.Flags().StringVar(
		&cliArgs.CreateAdminEmail,
		"admin-email", "", "Email address of control plane admin.  Provided to TLS provider.",
	)
	CreateControlPlaneCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneImageRepo,
		"control-plane-image-repo", "i", "", "Alternate image repo to pull threeport control plane images from.",
	)
	CreateControlPlaneCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneImageTag,
		"control-plane-image-tag", "t", "", "Alternate image tag to pull threeport control plane images from.",
	)
	CreateControlPlaneCmd.Flags().IntVar(
		&cliArgs.ThreeportLocalAPIPort,
		"threeport-api-port", 443, "Local port to bind threeport APIServer to. Only applies to kind provider.")
	CreateControlPlaneCmd.Flags().IntVar(
		&cliArgs.NumWorkerNodes,
		"num-worker-nodes", 0, "Number of additional worker nodes to deploy. Only applies to kind provider. (default is 0)")
}
