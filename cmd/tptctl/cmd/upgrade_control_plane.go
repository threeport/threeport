/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	auth "github.com/threeport/threeport/pkg/auth/v0"
	cli "github.com/threeport/threeport/pkg/cli/v0"
	"k8s.io/client-go/util/homedir"

	client "github.com/threeport/threeport/pkg/client/v0"
	threeport "github.com/threeport/threeport/pkg/threeport-installer/v0"
)

var updateImageTag string
var kubeconfigPath string
var controlPlaneNamespace string

// UpCmd represents the create threeport command
var UpgradeControlPlaneCmd = &cobra.Command{
	Use:     "control-plane",
	Example: "tptctl upgrade control-plane",
	Short:   "Upgrades the Threeport control plane to the current version",
	Long: `Upgrades the version of the Threeport control plane to the current version
	of tptctl`,
	SilenceUsage: true,
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, threeportConfig, apiEndpoint, requestedControlPlane := getClientContext(cmd)
		cpi := threeport.NewInstaller()

		authEnabled, err := threeportConfig.GetThreeportAuthEnabled(requestedControlPlane)
		if err != nil {
			cli.Error("failed to determine if auth enabled on control plane:", err)
			os.Exit(1)
		}

		cpi.SetAllImageTags(updateImageTag)
		cpi.Opts.CreateOrUpdateKubeResources = true
		cpi.Opts.ControlPlaneOnly = true
		cpi.Opts.AuthEnabled = authEnabled
		cpi.Opts.Namespace = controlPlaneNamespace

		// create dynamic client and rest mapper
		dynamicKubeClient, mapper, err := client.GetKubeDynamicClientAndMapper(kubeconfigPath)
		if err != nil {
			cli.Error("failed to create dynamic kube client and mapper", err)
			os.Exit(1)
		}

		var authConfig *auth.AuthConfig
		var authConfigErr error
		if authEnabled {
			authConfig, authConfigErr = threeportConfig.GetAuthConfig(requestedControlPlane)
			if authConfigErr != nil {
				cli.Error("could not retrieve auth config from threeport config", authConfigErr)
				os.Exit(1)
			}
		} else {
			authConfig = nil
		}

		if err := cpi.UpgradeControlPlaneComponents(dynamicKubeClient, &mapper, apiClient, apiEndpoint, authConfig); err != nil {
			cli.Error("could not upgrade control plane components", err)
			os.Exit(1)
		}

		cli.Complete("Succesfully updated all threeport control plane components")
	},
}

func init() {
	UpgradeCmd.AddCommand(UpgradeControlPlaneCmd)

	UpgradeControlPlaneCmd.Flags().StringVarP(
		&updateImageTag,
		"version", "t", "", "version to update Threeport Control plane.",
	)

	UpgradeControlPlaneCmd.Flags().StringVar(
		&kubeconfigPath,
		"kubeconfig", filepath.Join(homedir.HomeDir(), ".kube", "config"), "Kubeconfig file to use.",
	)

	UpgradeControlPlaneCmd.Flags().StringVar(
		&controlPlaneNamespace,
		"namespace", "threeport-control-plane", "Control plane namespace.",
	)

	UpgradeControlPlaneCmd.MarkFlagRequired("version")
}
