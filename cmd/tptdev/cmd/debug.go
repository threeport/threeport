/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"os"

	"github.com/spf13/cobra"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	cli "github.com/threeport/threeport/pkg/cli/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	config "github.com/threeport/threeport/pkg/config/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
	installer "github.com/threeport/threeport/pkg/threeport-installer/v0"
	"github.com/threeport/threeport/pkg/threeport-installer/v0/tptdev"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/dynamic"
)

var disable bool
var liveReload bool
var debugComponentNames string

// buildCmd represents the up command
var debugCmd = &cobra.Command{
	Use:   "debug",
	Short: "Debug threeport control plane components.",
	Long:  `Debug threeport control plane components.`,
	Run: func(cmd *cobra.Command, args []string) {

		// create list of components to build
		debugComponents, err := GetComponentList(debugComponentNames, installer.AllControlPlaneComponents())
		if err != nil {
			cli.Error("failed to get debug component list: %w", err)
		}

		// update cli args based on env vars
		cliArgs.GetControlPlaneEnvVars()

		// create threeport control plane installer
		cpi, err := cliArgs.CreateInstaller()
		if err != nil {
			cli.Error("failed to create threeport control plane installer", err)
			os.Exit(1)
		}

		// get threeport config and extract threeport API endpoint
		threeportConfig, requestedControlPlane, err := config.GetThreeportConfig(cliArgs.ControlPlaneName)
		if err != nil {
			cli.Error("failed to get threeport config", err)
			os.Exit(1)
		}

		// get threeport auth enabled
		authEnabled, err := threeportConfig.GetThreeportAuthEnabled(requestedControlPlane)
		if err != nil {
			cli.Error("failed to get threeport auth enabled", err)
			os.Exit(1)
		}

		// set CreateOrUpdateKubeResources so we can update existing deployments
		cpi.Opts.CreateOrUpdateKubeResources = true
		cpi.Opts.Debug = !disable
		cpi.Opts.LiveReload = liveReload
		cpi.Opts.DevEnvironment = false
		cpi.Opts.AuthEnabled = authEnabled

		// get threeport infra provider
		infraProvider, err := threeportConfig.GetThreeportInfraProvider(requestedControlPlane)
		if err != nil {
			cli.Error("failed to get threeport infra provider", err)
			os.Exit(1)
		}

		// ensure live reload is only used on kind
		if infraProvider != "kind" && liveReload {
			cli.Error("live-reload is only supported for kind infra provider", err)
			os.Exit(1)
		}

		// get threeport API endpoint
		apiEndpoint, err := threeportConfig.GetThreeportAPIEndpoint(requestedControlPlane)
		if err != nil {
			cli.Error("failed to get threeport API endpoint from config", err)
			os.Exit(1)
		}

		// get threeport API client
		apiClient, err := threeportConfig.GetHTTPClient(requestedControlPlane)
		if err != nil {
			cli.Error("failed to get threeport API client", err)
			os.Exit(1)
		}

		// get threeport control plane instance
		controlPlaneInstance, err := threeportConfig.GetControlPlaneInstance(requestedControlPlane)
		if err != nil {
			cli.Error("failed to get control plane instance", err)
			os.Exit(1)
		}

		// get kubernetes runtime instances
		kubernetesRuntimeInstance, err := client.GetKubernetesRuntimeInstanceByID(
			apiClient,
			apiEndpoint,
			*controlPlaneInstance.KubernetesRuntimeInstanceID,
		)
		if err != nil {
			cli.Error("failed to retrieve kubernetes runtime instances", err)
			os.Exit(1)
		}

		// get encryption key
		encryptionKey, err := threeportConfig.GetEncryptionKey(requestedControlPlane)
		if err != nil {
			cli.Error("failed to get encryption key", err)
			os.Exit(1)
		}

		// perform provider-specific auth steps
		switch infraProvider {
		case api_v0.KubernetesRuntimeInfraProviderEKS:
			// get aws config resource manager
			_, awsConfigResourceManager, _, err := threeportConfig.GetAwsConfigs(requestedControlPlane)
			if err != nil {
				cli.Error("failed to get AWS configs from threeport config: %w", err)
			}

			// refresh EKS connection with local config
			// and return updated kubernetesRuntimeInstance
			kubernetesRuntimeInstance, err = cli.RefreshEKSConnectionWithLocalConfig(
				awsConfigResourceManager,
				kubernetesRuntimeInstance,
				apiClient,
				apiEndpoint,
			)
			if err != nil {
				cli.Error("failed to refresh EKS connection with local config: %w", err)
				os.Exit(1)
			}
		}

		// get kube client
		var dynamicKubeClient dynamic.Interface
		var mapper *meta.RESTMapper
		if dynamicKubeClient, mapper, err = kube.GetClient(
			kubernetesRuntimeInstance,
			false,
			apiClient,
			apiEndpoint,
			encryptionKey,
		); err != nil {
			cli.Error("failed to create kube client", err)
			os.Exit(1)
		}

		// update deployments
		for _, component := range debugComponents {
			switch component.Name {
			case "rest-api":
				if err := cpi.UpdateThreeportAPIDeployment(
					dynamicKubeClient,
					mapper,
					encryptionKey,
				); err != nil {
					cli.Error("failed to apply threeport rest api", err)
					os.Exit(1)
				}
				continue
			case "agent":
				if err := cpi.UpdateThreeportAgentDeployment(
					dynamicKubeClient,
					mapper,
					requestedControlPlane,
				); err != nil {
					cli.Error("failed to apply threeport agent", err)
					os.Exit(1)
				}
				continue
			default:
				if err = cpi.UpdateControllerDeployment(
					dynamicKubeClient,
					mapper,
					*component,
				); err != nil {
					cli.Error("failed to apply threeport controllers", err)
					os.Exit(1)
				}
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(debugCmd)
	debugCmd.Flags().StringVarP(
		&debugComponentNames,
		"names", "n", "", "Comma-delimited list of component names to update with debug images (rest-api,agent,workload-controller etc). Defaults to all components.",
	)
	debugCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneImageRepo,
		"control-plane-image-repo", "r", "", "Alternate image repo to pull threeport control plane images from.",
	)
	debugCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneImageTag,
		"control-plane-image-tag", "t", "", "Alternate image tag to pull threeport control plane images from.",
	)
	debugCmd.Flags().BoolVar(
		&disable,
		"disable", false, "Disable debug mode.",
	)
	debugCmd.Flags().BoolVar(
		&liveReload,
		"live-reload", false, "Enable live-reload via air.",
	)
	debugCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "c", tptdev.DefaultInstanceName, "Name of dev control plane instance.",
	)
}
