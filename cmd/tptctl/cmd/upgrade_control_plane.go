/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	cli "github.com/threeport/threeport/pkg/cli/v0"
	"k8s.io/client-go/util/homedir"

	client "github.com/threeport/threeport/pkg/client/v0"
	threeport "github.com/threeport/threeport/pkg/threeport-installer/v0"
)

var updateImageTag string
var kubeconfigPath string
var controlPlaneNamespace string
var authEnabled bool

// UpCmd represents the create threeport command
var UpgradeControlPlaneCmd = &cobra.Command{
	Use:     "control-plane",
	Example: "tptctl upgrade control-plane --version=v0.5.0",
	Short:   "Upgrades the version of the Threeport control plane",
	Long: `Upgrades the version of the Threeport control plane. The version should be a valid
	image tag.`,
	SilenceUsage: true,
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		cpi := threeport.NewInstaller()

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

		if err := cpi.UpdateThreeportAPIDeployment(
			dynamicKubeClient,
			&mapper,
		); err != nil {
			cli.Error("failed to update threeport rest api", err)
			os.Exit(1)
		}

		if err := cpi.UpdateThreeportAgentDeployment(
			dynamicKubeClient,
			&mapper,
			controlPlaneNamespace,
		); err != nil {
			cli.Error("failed to update threeport agent", err)
			os.Exit(1)
		}

		for _, c := range cpi.Opts.ControllerList {
			if err = cpi.UpdateControllerDeployment(
				dynamicKubeClient,
				&mapper,
				*c,
			); err != nil {
				cli.Error(fmt.Sprintf("failed to update threeport controller: %s", c.Name), err)
				os.Exit(1)
			}
		}

		cli.Complete(fmt.Sprintf("Succesfully updated all threeport deployments to version: %s", updateImageTag))
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

	UpgradeControlPlaneCmd.Flags().BoolVar(
		&authEnabled,
		"auth-enabled", false, "Specify if auth is enabled on target control plane.",
	)

	UpgradeControlPlaneCmd.MarkFlagRequired("version")
}
