/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	cli "github.com/threeport/threeport/pkg/cli/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
	threeport "github.com/threeport/threeport/pkg/threeport-installer/v0"
	"gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/dynamic"
)

var updateControllerConfigPath string

// UpCmd represents the create threeport command
var UpgradeControlPlaneControllerCmd = &cobra.Command{
	Use:     "control-plane-controller",
	Example: "tptctl upgrade control-plane-controller --config /path/to/config.yaml",
	Short:   "Upgrades the Threeport control plane with a new controller",
	Long: `Upgrades the Threeport control plane with an additional controller. The provided config should
	be a valid control plane component to deploy.`,
	SilenceUsage: true,
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, config, apiEndpoint, requestedControlPlane := getClientContext(cmd)

		// load control plane component config
		configContent, err := os.ReadFile(updateControllerConfigPath)
		if err != nil {
			cli.Error("failed to read config file", err)
			os.Exit(1)
		}

		var controllerComponent v0.ControlPlaneComponent
		if err := yaml.Unmarshal(configContent, &controllerComponent); err != nil {
			cli.Error("failed to unmarshal config file yaml content", err)
			os.Exit(1)
		}

		encyptionKey, err := config.GetEncryptionKey(requestedControlPlane)
		if err != nil {
			cli.Error("failed to retrieve encryption key for control plane:", err)
			os.Exit(1)
		}

		authConfig, err := config.GetAuthConfig(requestedControlPlane)
		if err != nil {
			cli.Error("failed to retrieve auth config for control plane:", err)
			os.Exit(1)
		}

		// get the kubernetes runtime instance object
		kubernetesRuntimeInstance, err := client.GetThreeportControlPlaneKubernetesRuntimeInstance(
			apiClient,
			apiEndpoint,
		)
		if err != nil {
			cli.Error("failed to retrieve kubernetes runtime instance from threeport API:", err)
			os.Exit(1)
		}

		// get the kubernetes runtime instance object
		controlPlaneInstance, err := client.GetSelfControlPlaneInstance(
			apiClient,
			apiEndpoint,
		)
		if err != nil {
			cli.Error("failed to retrieve self control plane instance from threeport API:", err)
			os.Exit(1)
		}

		var dynamicKubeClient dynamic.Interface
		var mapper *meta.RESTMapper
		dynamicKubeClient, mapper, err = kube.GetClient(
			kubernetesRuntimeInstance,
			false,
			apiClient,
			apiEndpoint,
			encyptionKey,
		)
		if err != nil {
			cli.Error("failed to get kube client:", err)
			os.Exit(1)
		}

		cpi := threeport.NewInstaller()
		cpi.Opts.Namespace = *controlPlaneInstance.Namespace
		cpi.Opts.ControllerList = []*v0.ControlPlaneComponent{&controllerComponent}

		// install the controllers
		if err := cpi.InstallThreeportControllers(
			dynamicKubeClient,
			mapper,
			authConfig,
		); err != nil {
			cli.Error(fmt.Sprintf("failed to upgrade threeport control plane with: %s", controllerComponent.Name), err)
			os.Exit(1)
		}

		cli.Complete(fmt.Sprintf("Succesfully updated threeport control plane with controller: %s", controllerComponent.Name))
	},
}

func init() {
	UpgradeCmd.AddCommand(UpgradeControlPlaneControllerCmd)

	UpgradeControlPlaneControllerCmd.Flags().StringVarP(
		&updateControllerConfigPath,
		"config", "c", "", "Path to file with controller component config.",
	)

	UpgradeControlPlaneControllerCmd.MarkFlagRequired("config")
}
