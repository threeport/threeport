/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	cli "github.com/threeport/threeport/pkg/cli/v0"
	threeport "github.com/threeport/threeport/pkg/threeport-installer/v0"
)

// TODO: will become a variable once production-ready control plane instances are
// available.
const tier = threeport.ControlPlaneTierDev

// UpCmd represents the create threeport command
var UpCmd = &cobra.Command{
	Use:          "up",
	Example:      "tptctl up --name my-threeport",
	Short:        "Spin up a new deployment of the Threeport control plane",
	Long:         `Spin up a new deployment of the Threeport control plane.`,
	SilenceUsage: true,
	PreRun: func(cmd *cobra.Command, args []string) {
		switch cliArgs.InfraProvider {
		case v0.KubernetesRuntimeInfraProviderEKS:
			cmd.MarkFlagRequired("aws-region")
		}
	},
	Run: func(cmd *cobra.Command, args []string) {
		// flag validation
		if err := cli.ValidateCreateGenesisControlPlaneFlags(
			cliArgs.ControlPlaneName,
			cliArgs.InfraProvider,
			cliArgs.CreateRootDomain,
			cliArgs.AuthEnabled,
		); err != nil {
			cli.Error("flag validation failed:", err)
			os.Exit(1)
		}
		cpi, err := cliArgs.CreateInstaller()
		if err != nil {
			cli.Error("failed to create threeport control plane installer", err)
			os.Exit(1)
		}

		err = cli.CreateGenesisControlPlane(cpi)
		if err != nil {
			cli.Error("failed to create threeport control plane", err)
			if errors.Is(cli.ErrThreeportConfigAlreadyExists, err) {
				cli.Info("if you wish to overwrite the existing config use --force-overwrite-config flag")
				cli.Warning("you will lose the ability to connect to the existing threeport control planes if they are still running")
			}
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(UpCmd)

	UpCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"name", "n", "", "Required. Name of control plane instance.",
	)
	UpCmd.MarkFlagRequired("name")
	UpCmd.Flags().StringVarP(
		&cliArgs.InfraProvider,
		"provider", "p", "kind", fmt.Sprintf("The infrasture provider to install upon. Supported infra providers: %s", v0.SupportedInfraProviders()),
	)
	// this flag will be enabled once production-ready control plane instances
	// are available.
	//UpCmd.Flags().StringVarP(
	//	&tier,
	//	"tier", "t", threeport.ControlPlaneTierDev, "Determines the level of availability and data retention for the control plane.",
	//)
	UpCmd.Flags().StringVar(
		&cliArgs.KubeconfigPath,
		"kind-kubeconfig", "", "Path to kubeconfig used for kind provider installs (default is ~/.kube/config).",
	)
	UpCmd.Flags().StringVar(
		&cliArgs.AwsConfigProfile,
		"aws-config-profile", "default", "The AWS config profile to draw credentials from when using eks provider.",
	)
	UpCmd.Flags().BoolVar(
		&cliArgs.AwsConfigEnv,
		"aws-config-env", false, "Retrieve AWS credentials from environment variables when using eks provider.",
	)
	UpCmd.Flags().StringVar(
		&cliArgs.AwsRegion,
		"aws-region", "", "AWS region code to install threeport in when using eks provider. If provided, will take precedence over AWS config profile and environment variables.",
	)
	UpCmd.Flags().BoolVar(
		&cliArgs.ForceOverwriteConfig,
		"force-overwrite-config", false, "Force the overwrite of an existing Threeport instance config.  Warning: this will erase the connection info for the existing instance.  Only do this if the existing instance has already been deleted and is no longer in use.",
	)
	UpCmd.Flags().BoolVar(
		&cliArgs.AuthEnabled,
		"auth-enabled", true, "Enable client certificate authentication. Can only be disabled when using kind provider.",
	)
	UpCmd.Flags().StringVar(
		&cliArgs.CreateRootDomain,
		"root-domain", "", "The root domain name to use for the Threeport API. Requires a public hosted zone in AWS Route53. A subdomain for the Threeport API will be added to the root domain.",
	)
	UpCmd.Flags().StringVar(
		&cliArgs.CreateAdminEmail,
		"admin-email", "", "Email address of control plane admin.  Provided to TLS provider.",
	)
	UpCmd.Flags().StringVar(
		&cliArgs.ControlPlaneImageRepo,
		"control-plane-image-repo", "", "Alternate image repo to pull threeport control plane images from.",
	)
	UpCmd.Flags().StringVar(
		&cliArgs.ControlPlaneImageTag,
		"control-plane-image-tag", "", "Alternate image tag to pull threeport control plane images from.",
	)
	UpCmd.Flags().IntVar(
		&cliArgs.NumWorkerNodes,
		"num-worker-nodes", 0, "Number of additional worker nodes to deploy. Only applies to kind provider. (default is 0)")
	UpCmd.Flags().BoolVar(
		&cliArgs.Debug,
		"debug", false, "Enable debug mode. Defaults to false.",
	)
}
