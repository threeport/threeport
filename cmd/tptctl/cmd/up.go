/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/threeport/threeport/internal/provider"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	cli "github.com/threeport/threeport/pkg/cli/v0"
	threeport "github.com/threeport/threeport/pkg/threeport-installer/v0"
)

// upApis holds the --apis flag value: a comma-separated list
// of sdk-config ApiObjectGroup names whose controllers to install.
var upApis string

// TODO: will become a variable once production-ready control plane instances are
// available.
const tier = threeport.ControlPlaneTierDev

// UpCmd represents the create threeport command
var UpCmd = &cobra.Command{
	Use:     "up",
	Example: "tptctl up --name genesis",
	Short:   "Spin up a new deployment of the Threeport control plane",
	Long: `Spin up a new deployment of the Threeport control plane. A Threeport
control plane created with this command is called a 'genesis' control plane.  Subsequent
Threeport control planes can be created by the genesis control plane via the control plane API.
These are called 'derived' control planes.  These can also be referred to as 'parent' or 'child'
control planes if they are used to create or are created by another control plane.
`,
	SilenceUsage: true,
	PreRun: func(cmd *cobra.Command, args []string) {
		// if using eks provider, ensure aws-region is provided
		switch cliArgs.InfraProvider {
		case v0.KubernetesRuntimeInfraProviderEKS:
			cmd.MarkFlagRequired("aws-region")
		case v0.KubernetesRuntimeInfraProviderOKE:
			cmd.MarkFlagRequired("oci-region")
		}
	},
	Run: func(cmd *cobra.Command, args []string) {
		// set the config file path based on:
		// 1. the config file path provided with a flag by the user
		// 2. an environment variable
		// 3. the default config file path
		cfgFile := cli.DetermineThreeportConfigPath(cliArgs.CfgFile)
		// create a new threeport config file if it doesn't exist
		if _, err := os.Stat(cfgFile); errors.Is(err, os.ErrNotExist) {
			cfgDir := filepath.Dir(cfgFile)
			if err := os.MkdirAll(cfgDir, os.ModePerm); err != nil {
				cli.Error("failed to create directory for Threeport config file", err)
				os.Exit(1)
			}
			if err := viper.WriteConfigAs(cfgFile); err != nil {
				cli.Error("failed to write Threeport config file to disk", err)
				os.Exit(1)
			}
			// ensure config permissions are read/write for user only
			if err := os.Chmod(cfgFile, 0600); err != nil {
				cli.Error("failed to set permissions to read/write only", err)
				os.Exit(1)
			}
		}

		// set the config file path
		viper.SetConfigFile(cfgFile)

		// read the config file
		if err := viper.ReadInConfig(); err != nil {
			cli.Error("failed to read config", err)
			os.Exit(1)
		}

		// default --cluster-name to the threeport- prefixed runtime name
		// in control-plane-only mode when not supplied; this is the name
		// tptctl applies when it provisions the cluster itself (e.g. via
		// a prior --infra-only run)
		if cliArgs.ControlPlaneOnly && cliArgs.ClusterName == "" {
			cliArgs.ClusterName = provider.ThreeportRuntimeName(cliArgs.ControlPlaneName)
		}

		// flag validation
		if err := cli.ValidateCreateGenesisControlPlaneFlags(
			cliArgs.ControlPlaneName,
			cliArgs.InfraProvider,
			cliArgs.CreateRootDomain,
			cliArgs.AuthEnabled,
			cliArgs.KindPortMappings,
			cliArgs.ControlPlaneOnly,
			cliArgs.ClusterName,
		); err != nil {
			cli.Error("flag validation failed:", err)
			os.Exit(1)
		}

		cpi, err := cliArgs.CreateInstaller()
		if err != nil {
			cli.Error("failed to create threeport control plane installer", err)
			os.Exit(1)
		}

		// narrow the controller list when --apis is set so the install
		// brings up only the requested apis' controllers alongside the
		// rest-api and agent.
		if upApis != "" {
			selected, err := threeport.SelectControllersByGroup(
				parseUpApis(upApis),
				cpi.Opts.ControllerList,
			)
			if err != nil {
				cli.Error("failed to select controllers", err)
				os.Exit(1)
			}
			selectedNames := make([]string, 0, len(selected))
			for _, controller := range selected {
				selectedNames = append(selectedNames, controller.Name)
			}
			cli.Info(fmt.Sprintf(
				"limiting install to %d controller(s): %s",
				len(selected), strings.Join(selectedNames, ", "),
			))
			cpi.Opts.ControllerList = selected
		}

		err = cli.CreateGenesisControlPlane(cpi)
		if err != nil {
			cli.Error("failed to create threeport control plane", err)
			if errors.Is(err, cli.ErrThreeportConfigAlreadyExists) {
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
		"name", "n", "", "Required. Name of genesis control plane.",
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
		"kind-kubeconfig", "", "Path to kubeconfig used for kind provider installs (default is $KUBECONFIG, then ~/.kube/config).",
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
	UpCmd.Flags().StringVar(
		&cliArgs.OciRegion,
		"oci-region", "", "OCI region code to install threeport in when using oke provider.",
	)
	UpCmd.Flags().StringVar(
		&cliArgs.OciConfigProfile,
		"oci-config-profile", "DEFAULT", "The OCI config profile to draw credentials from when using oke provider.",
	)
	UpCmd.Flags().StringVar(
		&cliArgs.GcpProjectId,
		"gcp-project-id", "", "Google Cloud project ID to install threeport in when using gke provider. If provided, will take precedence over environment variables.",
	)
	UpCmd.Flags().StringVar(
		&cliArgs.GcpRegion,
		"gcp-region", "", "Google Cloud region code to install threeport in when using gke provider. If provided, will take precedence over environment variables.",
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
	UpCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneImageRepo,
		"control-plane-image-namespace", "r", "", "Alternate image namespace to pull threeport control plane images from.",
	)
	UpCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneImageTag,
		"control-plane-image-tag", "t", "", "Alternate image tag for threeport control plane images.",
	)
	UpCmd.Flags().IntVar(
		&cliArgs.NumWorkerNodes,
		"num-worker-nodes", 0, "Number of additional worker nodes to deploy. Only applies to kind provider. (default is 0)",
	)
	UpCmd.Flags().BoolVar(
		&cliArgs.Debug,
		"debug", false, "Enable debug mode. Defaults to false.",
	)
	UpCmd.Flags().BoolVar(
		&cliArgs.TeardownOnFailure,
		"teardown-on-failure", false, "Automatically tear down control plane resources if an error is encountered.",
	)
	UpCmd.Flags().BoolVar(
		&cliArgs.ControlPlaneOnly,
		"control-plane-only", false, "Deploy the control plane on an existing runtime. Defaults to false.",
	)
	UpCmd.Flags().StringVar(
		&cliArgs.ClusterName,
		"cluster-name", "", "Optional. Name of the existing kubernetes cluster to install the control plane on. Only applicable with --control-plane-only.",
	)
	UpCmd.Flags().BoolVar(
		&cliArgs.InfraOnly,
		"infra-only", false, "Deploy only the infrastructure without the control plane. Defaults to false.",
	)
	UpCmd.Flags().BoolVar(
		&cliArgs.LocalRegistry,
		"local-registry", false, "Connects a local container registry to Threeport control plane cluster.  Only applicable with provider 'kind'.",
	)
	UpCmd.Flags().StringSliceVar(
		&cliArgs.KindPortMappings,
		"kind-port-mappings", []string{}, "Port mappings for kind provider. Format: <container-port>:<host-port>,<container-port>:<host-port>,...",
	)
	UpCmd.Flags().StringVar(
		&upApis,
		"apis", "",
		"Comma-separated sdk-config api names (e.g. kubernetes_workload,gateway) "+
			"whose controllers to install. NOT component names. When omitted, installs the "+
			"full default controller list.",
	)
}

// parseUpApis splits the --apis flag value on commas and trims
// whitespace from each entry, dropping empty fragments produced by
// leading or trailing commas.
func parseUpApis(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
