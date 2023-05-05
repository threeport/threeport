/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/threeport/threeport/internal/cli"
	"github.com/threeport/threeport/internal/kube"
	"github.com/threeport/threeport/internal/provider"
	"github.com/threeport/threeport/internal/threeport"
	"github.com/threeport/threeport/internal/tptdev"
	"github.com/threeport/threeport/internal/util"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	config "github.com/threeport/threeport/pkg/config/v0"
)

var (
	createThreeportDevName string
	kubeconfigPath         string
	threeportPath          string
	authEnabled            bool
	threeportLocalAPIPort  int
	numWorkerNodes         int
	forceOverwriteConfig   bool
	cfgFile                string
	providerConfigDir      string
)

// upCmd represents the up command
var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Spin up a new threeport development environment",
	Long:  `Spin up a new threeport development environment.`,
	Run: func(cmd *cobra.Command, args []string) {

		threeportConfig := config.GetThreeportConfig()

		// check threeport config for exisiting instance
		threeportInstanceConfigExists := config.CheckThreeportConfigExists(threeportConfig, createThreeportDevName, forceOverwriteConfig)

		// get default kubeconfig if not provided
		if kubeconfigPath == "" {
			ck, err := kube.DefaultKubeconfig()
			if err != nil {
				cli.Error("failed to get path to default kubeconfig", err)
				os.Exit(1)
			}
			kubeconfigPath = ck
		}

		// set default threeport repo path if not provided
		// this is needed to map the container path to the host path for live
		// reloads of the code
		if threeportPath == "" {
			tp, err := os.Getwd()
			if err != nil {
				cli.Error("failed to get current working directory", err)
				os.Exit(1)
			}
			threeportPath = tp
		}

		// create kind cluster
		controlPlaneInfra := provider.ControlPlaneInfraKind{
			ThreeportInstanceName: createThreeportDevName,
			KubeconfigPath:        kubeconfigPath,
			ThreeportPath:         threeportPath,
		}
		devEnvironment := true
		kindConfig := controlPlaneInfra.GetKindConfig(devEnvironment, numWorkerNodes)
		controlPlaneInfra.KindConfig = kindConfig
		kubeConnectionInfo, err := controlPlaneInfra.Create()
		if err != nil {
			cli.Error("failed to create kind cluster", err)
			os.Exit(1)
		}

		// the cluster instance is the default compute space cluster to be added
		// to the API - it is used to kube client for creating control plane
		// resources
		clusterInstName := fmt.Sprintf("%s-compute-space-0", createThreeportDevName)
		controlPlaneCluster := true
		clusterInstance := v0.ClusterInstance{
			Instance: v0.Instance{
				Name: &clusterInstName,
			},
			ThreeportControlPlaneCluster: &controlPlaneCluster,
			APIEndpoint:                  &kubeConnectionInfo.APIEndpoint,
			CACertificate:                &kubeConnectionInfo.CACertificate,
			Certificate:                  &kubeConnectionInfo.Certificate,
			Key:                          &kubeConnectionInfo.Key,
		}

		// create a client to connect to kind cluster kube API
		dynamicKubeClient, mapper, err := kube.GetClient(&clusterInstance, false)
		if err != nil {
			cli.Error("failed to get a Kubernetes client and mapper", err)
			os.Exit(1)
		}

		// create threeport config for new instance
		newThreeportInstance := &config.Instance{
			Name:       createThreeportDevName,
			Provider:   "kind",
			APIServer:  fmt.Sprintf("https://%s:%d", threeport.ThreeportLocalAPIEndpoint, threeportLocalAPIPort),
			Kubeconfig: kubeconfigPath,
		}

		var authConfig *config.AuthConfig
		if authEnabled {
			authConfig = config.GetAuthConfig()

			// generate client certificate
			clientCertificate, clientPrivateKey, err := config.GenerateCertificate(authConfig.CAConfig, &authConfig.CAPrivateKey)
			if err != nil {
				cli.Error("failed to generate client certificate and private key", err)
				os.Exit(1)
			}

			clientCredentials := &config.Credential{
				Name:       createThreeportDevName,
				ClientCert: util.Base64Encode(clientCertificate),
				ClientKey:  util.Base64Encode(clientPrivateKey),
			}

			newThreeportInstance.Credentials = append(newThreeportInstance.Credentials, *clientCredentials)
			newThreeportInstance.CACert = authConfig.CABase64Encoded
		}

		config.UpdateThreeportConfig(threeportInstanceConfigExists, threeportConfig, createThreeportDevName, newThreeportInstance)

		// install the threeport control plane dependencies
		if err := threeport.InstallThreeportControlPlaneDependencies(dynamicKubeClient, mapper); err != nil {
			cli.Error("failed to install threeport control plane dependencies", err)
			os.Exit(1)
		}

		// build and load dev images for API and controllers
		if err := tptdev.PrepareDevImages(threeportPath, provider.ThreeportClusterName(createThreeportDevName)); err != nil {
			cli.Error("failed to build and load dev control plane images", err)
			os.Exit(1)
		}

		// install the threeport control plane API and controllers
		if err := threeport.InstallThreeportAPI(
			dynamicKubeClient,
			mapper,
			true,
			threeport.ThreeportLocalAPIEndpoint,
			"",
			"",
			authConfig,
		); err != nil {
			cli.Error("failed to install threeport control plane components", err)
			os.Exit(1)
		}

		// configure http client for calls to threeport API
		apiClient, err := config.GetHTTPClient(authEnabled)
		if err != nil {
			cli.Error("failed to create http client: ", err)
			os.Exit(1)
		}

		// wait for API server to start running
		cli.Info("waiting for threeport API to start running")
		if err := threeport.WaitForThreeportAPI(
			apiClient, fmt.Sprintf("%s:%d", threeport.ThreeportLocalAPIEndpoint, threeportLocalAPIPort),
		); err != nil {
			cli.Error("threeport API did not come up", err)
			os.Exit(1)
		}

		// install the threeport controllers - these need to be installed once
		// API server is running in dev environment because the air entrypoint
		// prevents the controllers from crashlooping if they come up before
		// the API server
		if err := threeport.InstallThreeportControllers(
			dynamicKubeClient,
			mapper,
			true,
			"",
			"",
			authConfig,
		); err != nil {
			cli.Error("failed to install threeport control plane components", err)
			os.Exit(1)
		}

		// create the default compute space cluster definition in threeport API
		clusterDefName := fmt.Sprintf("compute-space-%s", createThreeportDevName)
		clusterDefinition := v0.ClusterDefinition{
			Definition: v0.Definition{
				Name: &clusterDefName,
			},
		}
		clusterDefResult, err := client.CreateClusterDefinition(
			apiClient,
			fmt.Sprintf("%s:%d", threeport.ThreeportLocalAPIEndpoint, threeportLocalAPIPort),
			&clusterDefinition,
		)
		if err != nil {
			cli.Error("failed to create new cluster definition for default compute space", err)
			os.Exit(1)
		}

		// create default compute space cluster instance in threeport API
		clusterInstance.ClusterDefinitionID = clusterDefResult.ID

		_, err = client.CreateClusterInstance(
			apiClient,
			fmt.Sprintf("%s:%d", threeport.ThreeportLocalAPIEndpoint, threeportLocalAPIPort),
			&clusterInstance,
		)
		if err != nil {
			cli.Error("failed to create new cluster instance for default compute space", err)
			os.Exit(1)
		}

		// wait for 20 seconds to allow time for controllers to build and start
		// so that when this command returns, the control plane is ready
		time.Sleep(time.Second * 20)

		cli.Complete(fmt.Sprintf("threeport dev instance %s created", createThreeportDevName))
	},
}

func init() {
	rootCmd.AddCommand(upCmd)

	upCmd.Flags().StringVarP(
		&kubeconfigPath,
		"kubeconfig", "k", "", "Path to kubeconfig (default is ~/.kube/config).",
	)
	upCmd.Flags().BoolVar(
		&forceOverwriteConfig,
		"force-overwrite-config", false, "Force the overwrite of an existing Threeport instance config. Warning: this will erase the connection info for the existing instance.  Only do this if the existing instance has already been deleted and is no longer in use.",
	)
	upCmd.Flags().BoolVar(
		&authEnabled,
		"auth-enabled", false, "Enable client certificate authentication (default is false).",
	)
	upCmd.Flags().StringVarP(
		&createThreeportDevName,
		"name", "n", tptdev.DefaultInstanceName, "Name of dev control plane instance.",
	)
	upCmd.Flags().StringVarP(
		&threeportPath,
		"threeport-path", "t", "", "Path to threeport repository root (default is './').",
	)
	rootCmd.PersistentFlags().StringVar(
		&cfgFile,
		"threeport-config", "", "Path to config file (default is $HOME/.config/threeport/config.yaml).",
	)
	rootCmd.PersistentFlags().StringVar(
		&providerConfigDir,
		"provider-config", "", "Path to infra provider config directory (default is $HOME/.config/threeport/).",
	)
	upCmd.Flags().IntVar(
		&threeportLocalAPIPort,
		"threeport-api-port", 443, "Local port to bind threeport APIServer to (default is 443).",
	)
	upCmd.Flags().IntVar(
		&numWorkerNodes,
		"num-worker-nodes", 0, "Number of additional worker nodes to deploy (default is 0).",
	)
	cobra.OnInitialize(func() {
		config.InitConfig(cfgFile, providerConfigDir)
	})
}
