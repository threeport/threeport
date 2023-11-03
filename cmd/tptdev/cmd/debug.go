/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"os"
	"strings"

	"github.com/spf13/cobra"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	cli "github.com/threeport/threeport/pkg/cli/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	config "github.com/threeport/threeport/pkg/config/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
	v0 "github.com/threeport/threeport/pkg/threeport-installer/v0"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/dynamic"
)

var debugDisable bool

// buildCmd represents the up command
var debugCmd = &cobra.Command{
	Use:   "debug",
	Short: "Spin up a new threeport development environment",
	Long:  `Spin up a new threeport development environment.`,
	Run: func(cmd *cobra.Command, args []string) {

		// create list of images to build
		imageNamesList := []string{}
		switch all {
		case true:
			for _, controller := range v0.AllControlPlaneComponents() {
				imageNamesList = append(imageNamesList, controller.Name)
			}
		case false:
			imageNamesList = strings.Split(imageNames, ",")
		}

		// configure control plane image repo via env var if not provided by cli
		if cliArgs.ControlPlaneImageRepo == "" && os.Getenv("CONTROL_PLANE_IMAGE_REPO") != "" {
			cliArgs.ControlPlaneImageRepo = os.Getenv("CONTROL_PLANE_IMAGE_REPO")
		}

		// configure control plane image tag via env var if not provided by cli
		if cliArgs.ControlPlaneImageTag == "" && os.Getenv("CONTROL_PLANE_IMAGE_TAG") != "" {
			cliArgs.ControlPlaneImageTag = os.Getenv("CONTROL_PLANE_IMAGE_TAG")
		}

		if debugDisable {
			cliArgs.ControlPlaneImageRepo = ""
			cliArgs.ControlPlaneImageTag = ""
		}

		cpi, err := cliArgs.CreateInstaller()
		if err != nil {
			cli.Error("failed to create threeport control plane installer", err)
			os.Exit(1)
		}

		// configure which components to update
		debugComponents := []*api_v0.ControlPlaneComponent{}
		for _, component := range v0.AllControlPlaneComponents() {
			for _, imageName := range imageNamesList {
				if component.Name == imageName {
					debugComponents = append(debugComponents, component)
				}
			}
		}

		// set CreateOrUpdateKubeResources so we can update existing deployments
		cpi.Opts.CreateOrUpdateKubeResources = true
		cpi.Opts.Debug = !debugDisable
		cpi.Opts.DevEnvironment = false

		// get threeport config and extract threeport API endpoint
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
		apiClient, err := threeportConfig.GetHTTPClient(requestedControlPlane)
		if err != nil {
			cli.Error("failed to get threeport API client", err)
			os.Exit(1)
		}

		var id uint
		for _, controlPlane := range threeportConfig.ControlPlanes {
			if controlPlane.Name == requestedControlPlane {
				if controlPlaneInstance, err := client.GetControlPlaneInstanceByName(apiClient, apiEndpoint, controlPlane.Name); err != nil {
					cli.Error("failed to retrieve current control plane instance", err)
					os.Exit(1)
				} else {
					id = *controlPlaneInstance.KubernetesRuntimeInstanceID
				}
			}

		}

		// get kubernetes runtime instances
		kubernetesRuntimeInstance, err := client.GetKubernetesRuntimeInstanceByID(apiClient, apiEndpoint, id)
		if err != nil {
			cli.Error("failed to retrieve kubernetes runtime instances", err)
			os.Exit(1)
		}

		var dynamicKubeClient dynamic.Interface
		var mapper *meta.RESTMapper
		encryptionKey, err := threeportConfig.GetEncryptionKey(requestedControlPlane)
		if err != nil {
			cli.Error("failed to get encryption key", err)
			os.Exit(1)
		}

		if dynamicKubeClient, mapper, err = kube.GetClient(
			kubernetesRuntimeInstance,
			false,
			nil,
			"",
			encryptionKey,
		); err != nil {
			cli.Error("failed to create kube client", err)
			os.Exit(1)
		}

		authEnabled, err := threeportConfig.GetThreeportAuthEnabled(requestedControlPlane)
		if err != nil {
			cli.Error("failed to get threeport auth enabled", err)
			os.Exit(1)
		}

		for _, component := range debugComponents {
			if err = cpi.InstallController(
				dynamicKubeClient,
				mapper,
				false,
				*component,
				authEnabled,
			); err != nil {
				cli.Error("failed to apply threeport controllers", err)
				os.Exit(1)
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(debugCmd)
	debugCmd.Flags().StringVar(
		&imageNames,
		"image-names", "", "Image name",
	)
	debugCmd.Flags().StringVar(
		&cliArgs.ControlPlaneImageRepo,
		"control-plane-image-repo", "", "Alternate image repo to pull threeport control plane images from.",
	)
	debugCmd.Flags().StringVar(
		&cliArgs.ControlPlaneImageTag,
		"control-plane-image-tag", "", "Alternate image tag to pull threeport control plane images from.",
	)
	debugCmd.Flags().BoolVar(
		&all,
		"all", false, "Alternate image tag to pull threeport control plane images from.",
	)
	debugCmd.Flags().BoolVar(
		&debugDisable,
		"disable", false, "Disable debug mode.",
	)
}
