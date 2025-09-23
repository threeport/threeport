/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"os"

	"github.com/spf13/cobra"
	cli "github.com/threeport/threeport/pkg/cli/v0"
)

// DownCommandPreRunFunc is a custom PreRun function for the down command that
// skips threeport config initialization when --infra-only is used
func DownCommandPreRunFunc(cmd *cobra.Command, args []string) {
	// Initialize basic CLI args first
	cli.InitConfig(cmd, cliArgs.CfgFile)
	
	// Skip threeport config initialization if --infra-only is used
	if cliArgs.InfraOnly {
		// For infra-only teardown, provider must be specified
		if cliArgs.InfraProvider == "" {
			cli.Error("--provider flag is required when using --infra-only", nil)
			os.Exit(1)
		}
		
		// For OKE provider, oci-region is required
		if cliArgs.InfraProvider == "oke" && cliArgs.OciRegion == "" {
			cli.Error("--oci-region flag is required when using --infra-only with oke provider", nil)
			os.Exit(1)
		}
		
		// For infra-only teardown, we don't need the threeport API client
		return
	}
	
	// For normal teardown, initialize the full command context
	if err := initializeCommandContext(cmd); err != nil {
		cli.Error("could not initialize command in pre run:", err)
		os.Exit(1)
	}
}

// DownCmd represents the delete threeports
var DownCmd = &cobra.Command{
	Use:          "down",
	Example:      "tptctl down --name my-threeport",
	Short:        "Spin down a deployment of the Threeport control plane",
	Long:         `Spin down a deployment of the Threeport control plane.`,
	PreRun:       DownCommandPreRunFunc,
	SilenceUsage: true,
	Run: func(cmd *cobra.Command, args []string) {
		cpi, err := cliArgs.CreateInstaller()
		if err != nil {
			cli.Error("failed to create threeport control plane installer", err)
			os.Exit(1)
		}

		err = cli.DeleteGenesisControlPlane(cpi)
		if err != nil {
			cli.Error("failed to delete threeport control plane", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(DownCmd)

	DownCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"name", "n", "", "Required. Name of genesis control plane.",
	)
	DownCmd.Flags().BoolVar(
		&cliArgs.ControlPlaneOnly,
		"control-plane-only", false, "Tear down the control plane and leave runtime intact. Defaults to false.",
	)
	DownCmd.Flags().BoolVar(
		&cliArgs.InfraOnly,
		"infra-only", false, "Tear down only the infrastructure without the control plane. Defaults to false.",
	)
	DownCmd.Flags().BoolVar(
		&cliArgs.AwsConfigEnv,
		"aws-config-env", false, "Retrieve AWS credentials from environment variables when using eks provider.",
	)
	DownCmd.Flags().StringVarP(
		&cliArgs.InfraProvider,
		"provider", "p", "", "The infrastructure provider (required for --infra-only mode). Supported providers: kind, eks, oke.",
	)
	DownCmd.Flags().StringVar(
		&cliArgs.OciRegion,
		"oci-region", "", "OCI region code when using oke provider with --infra-only.",
	)
	DownCmd.Flags().StringVar(
		&cliArgs.OciConfigProfile,
		"oci-config-profile", "DEFAULT", "OCI config profile when using oke provider with --infra-only.",
	)
	DownCmd.Flags().StringVar(
		&cliArgs.OciCompartmentOcid,
		"oci-compartment-ocid", "", "OCI compartment OCID when using oke provider with --infra-only.",
	)
	DownCmd.MarkFlagRequired("name")
}
