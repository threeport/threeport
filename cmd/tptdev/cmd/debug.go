/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"k8s.io/client-go/util/homedir"

	cli "github.com/threeport/threeport/pkg/cli/v0"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
	installer "github.com/threeport/threeport/pkg/threeport-installer/v0"
	"github.com/threeport/threeport/pkg/threeport-installer/v0/tptdev"
)

var disable bool
var liveReload bool
var authEnabled bool
var debugComponentNames string
var kubeconfigPath string
var controlPlaneNamespace string

// buildCmd represents the up command
var DebugCmd = &cobra.Command{
	Use:   "debug",
	Short: "Debug threeport control plane components.",
	Long:  `Debug threeport control plane components.`,
	Run: func(cmd *cobra.Command, args []string) {

		// create list of components to build
		debugComponents, err := GetComponentList(debugComponentNames, installer.AllControlPlaneComponents())
		if err != nil {
			cli.Error("failed to get debug component list: %w", err)
			os.Exit(1)
		}

		// update cli args based on env vars
		cliArgs.GetControlPlaneEnvVars()

		// create threeport control plane installer
		cpi, err := cliArgs.CreateInstaller()
		if err != nil {
			cli.Error("failed to create threeport control plane installer", err)
			os.Exit(1)
		}

		// set CreateOrUpdateKubeResources so we can update existing deployments
		cpi.Opts.CreateOrUpdateKubeResources = true
		cpi.Opts.Debug = !disable
		cpi.Opts.LiveReload = liveReload
		cpi.Opts.DevEnvironment = false
		cpi.Opts.AuthEnabled = authEnabled
		cpi.Opts.Namespace = controlPlaneNamespace

		// create dynamic client and rest mapper
		dynamicKubeClient, mapper, err := client_lib.GetKubeDynamicClientAndMapper(kubeconfigPath)
		if err != nil {
			cli.Error("failed to create dynamic kube client and mapper", err)
			os.Exit(1)
		}

		// update deployments
		for _, component := range debugComponents {
			switch component.Name {
			case "rest-api":
				if err := cpi.UpdateThreeportAPIDeployment(
					dynamicKubeClient,
					&mapper,
				); err != nil {
					cli.Error("failed to apply threeport rest api", err)
					os.Exit(1)
				}
				continue
			case "agent":
				if err := cpi.UpdateThreeportAgentDeployment(
					dynamicKubeClient,
					&mapper,
					controlPlaneNamespace,
				); err != nil {
					cli.Error("failed to apply threeport agent", err)
					os.Exit(1)
				}
				continue
			default:
				if err = cpi.UpdateControllerDeployment(
					dynamicKubeClient,
					&mapper,
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
	rootCmd.AddCommand(DebugCmd)
	DebugCmd.Flags().StringVarP(
		&debugComponentNames,
		"names", "n", "", "Comma-delimited list of component names to update with debug images (rest-api,agent,workload-controller etc). Defaults to all components.",
	)
	DebugCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneImageRepo,
		"control-plane-image-repo", "r", "", "Alternate image repo to pull threeport control plane images from.",
	)
	DebugCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneImageTag,
		"control-plane-image-tag", "t", "", "Alternate image tag to pull threeport control plane images from.",
	)
	DebugCmd.Flags().BoolVar(
		&disable,
		"disable", false, "Disable debug mode.",
	)
	DebugCmd.Flags().BoolVar(
		&liveReload,
		"live-reload", false, "Enable live-reload via air.",
	)
	DebugCmd.Flags().BoolVar(
		&cliArgs.Verbose,
		"verbose", false, "Enable verbose logging in control plane components, delve, and cli logs.",
	)
	DebugCmd.Flags().BoolVar(
		&authEnabled,
		"auth-enabled", false, "Specify if auth is enabled on target control plane.",
	)
	DebugCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "c", tptdev.DefaultInstanceName, "Name of dev control plane instance.",
	)
	DebugCmd.Flags().StringVar(
		&kubeconfigPath,
		"kubeconfig", filepath.Join(homedir.HomeDir(), ".kube", "config"), "Kubeconfig file to use.",
	)
	DebugCmd.Flags().StringVar(
		&controlPlaneNamespace,
		"namespace", "threeport-control-plane", "Control plane namespace.",
	)
}
