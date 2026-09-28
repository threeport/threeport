/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/threeport/threeport/internal/version"
	auth "github.com/threeport/threeport/pkg/auth/v0"
	cli "github.com/threeport/threeport/pkg/cli/v0"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
	installer "github.com/threeport/threeport/pkg/threeport-installer/v0"
	"github.com/threeport/threeport/pkg/threeport-installer/v0/tptdev"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// reinstallApis holds the --apis group names. Empty auto-detects them from the cluster.
var reinstallApis string

// reinstallDropDatabase drops the schema before the install reapplies.
var reinstallDropDatabase bool

// reinstallConfirm holds the name typed back to authorize a database drop.
var reinstallConfirm string

// reinstallCmd reinstalls the stateless side of a dev control plane.
var reinstallCmd = &cobra.Command{
	Use:   "reinstall",
	Short: "Sweep and reapply stateless control plane resources",
	Long: `Reinstall the stateless side of a dev Threeport control plane.

Sweeps every installer-managed Deployment, then reapplies the install
with current images and specs from source. Preserves cockroachdb
data, nats data, the certificate authority, and the rest-api's
external service ip; recreates everything else.

Pass --drop-database to also reset that state: the control plane
scales down, its schema and nats streams drop, the install reapplies,
and migrations run from scratch. Requires --confirm with the control
plane name and a development-tier installation, which a cloud-hosted
control plane gets from 'tptctl up --tier development'.

Dev environments only. Does not build images; run 'tptdev build
--push' first if the image needs to change.`,
	Run: func(cmd *cobra.Command, args []string) {
		// apply control plane environment variables
		cliArgs.GetControlPlaneEnvVars()

		// get the threeport config
		threeportConfig, requestedControlPlane, err := cli.GetThreeportConfig(cliArgs.ControlPlaneName)
		if err != nil {
			cli.Error("failed to get threeport config", err)
			os.Exit(1)
		}
		// reject a name the config does not list
		if err := threeportConfig.ValidateControlPlaneName(requestedControlPlane); err != nil {
			cli.Error("cannot reinstall", err)
			os.Exit(1)
		}
		// keep the resolved control plane name
		cliArgs.ControlPlaneName = requestedControlPlane

		// refuse a drop unless --confirm matches the name
		if reinstallDropDatabase && reinstallConfirm != cliArgs.ControlPlaneName {
			cli.Error(fmt.Sprintf(
				"--drop-database destroys the database of control plane %q and its data cannot be recovered; re-run with --confirm %s to proceed",
				cliArgs.ControlPlaneName, cliArgs.ControlPlaneName,
			), nil)
			os.Exit(1)
		}

		// fill an empty image tag
		if cliArgs.ControlPlaneImageTag == "" {
			// resolve the tag from the repo and the version
			tag, err := util.ResolveImageTag(cliArgs.ThreeportPath, version.GetVersion())
			if err != nil {
				cli.Error(fmt.Sprintf("failed to resolve default image tag: %s\nspecify a tag explicitly with --tag/-t", err), nil)
				os.Exit(1)
			}
			// use the resolved tag
			cliArgs.ControlPlaneImageTag = tag
		}

		// create the installer
		cpi, err := cliArgs.CreateInstaller()
		if err != nil {
			cli.Error("failed to create threeport control plane installer", err)
			os.Exit(1)
		}
		// set the namespace and debug mode
		cpi.Opts.Namespace = installer.ControlPlaneNamespace
		cpi.Opts.Debug = cliArgs.Debug

		// refuse a gke restore that has no region before changing the cluster
		if reinstallDropDatabase {
			if err := cli.RequireRestoredRuntimeLocation(cpi.Opts.InfraProvider, cpi.Opts.GcpRegion); err != nil {
				cli.Error("cannot reinstall", err)
				os.Exit(1)
			}
		}

		// get a kubernetes client and rest mapper
		kubeClient, mapper, err := client_lib.GetKubeDynamicClientAndMapper(cliArgs.KubeconfigPath)
		if err != nil {
			cli.Error("failed to create kube client", err)
			os.Exit(1)
		}

		// select the controllers to reinstall
		selected, selectedNames, autoDetected, err := installer.SelectControllersForReinstall(
			kubeClient,
			cpi.Opts.Namespace,
			installer.ParseApis(reinstallApis),
			cpi.Opts.ControllerList,
		)
		if err != nil {
			cli.Error("failed to select controllers for reinstall", err)
			os.Exit(1)
		}
		// record whether --apis or the cluster supplied the set
		source := "specified via --apis"
		if autoDetected {
			source = "auto-detected from cluster"
		}
		// report the controllers this run reinstalls
		cli.Info(fmt.Sprintf(
			"reinstalling %d controller(s) (%s): %s",
			len(selected), source, strings.Join(selectedNames, ", "),
		))
		// install only the selected controllers
		cpi.Opts.ControllerList = selected

		// read auth from rest-api args; only -auth-enabled=false turns it off
		cpi.Opts.AuthEnabled = detectAuthEnabled(kubeClient)

		// leave the auth config unset until auth is enabled
		var authConfig *auth.AuthConfig
		// load the cluster CA so controller certs are signed by it
		if cpi.Opts.AuthEnabled {
			authConfig, err = cpi.LoadAuthConfigFromCluster(kubeClient, &mapper)
			if err != nil {
				cli.Error("failed to load existing CA from cluster", err)
				os.Exit(1)
			}
		}

		// hold module replica counts so they can be restored after reinstall
		var moduleScales []installer.ModuleDeploymentScale
		restoreScaledModules := func() {
			if len(moduleScales) == 0 {
				return
			}
			if err := cpi.RestoreModuleScale(kubeClient, moduleScales); err != nil {
				cli.Error("failed to restore module deployments", err)
			}
		}
		// read module namespaces from the api before the drop scales it down
		if reinstallDropDatabase {
			// refuse a non-development tier before any deployment is scaled down
			if err := cpi.RequireDevelopmentTier(kubeClient, &mapper); err != nil {
				cli.Error("cannot drop database", err)
				os.Exit(1)
			}

			// get an api client
			apiClient, err := threeportConfig.GetHTTPClient(requestedControlPlane)
			if err != nil {
				cli.Error("failed to get threeport API client", err)
				os.Exit(1)
			}
			// get the control plane config
			controlPlaneConfig, err := threeportConfig.GetControlPlaneConfig(requestedControlPlane)
			if err != nil {
				cli.Error("failed to get threeport control plane config", err)
				os.Exit(1)
			}

			// discover registered module controller deployments
			moduleDeployments, err := cpi.DiscoverModuleDeployments(apiClient, controlPlaneConfig.APIServer)
			if err != nil {
				cli.Error("failed to discover registered module deployments", err)
				os.Exit(1)
			}
			if len(moduleDeployments) > 0 {
				cli.Info(fmt.Sprintf("scaling down %d module deployment(s)", len(moduleDeployments)))
			}

			// scale module deployments to zero and keep their replica counts
			moduleScales, err = cpi.ScaleDownModules(kubeClient, moduleDeployments)
			if err != nil {
				restoreScaledModules()
				cli.Error("failed to scale down module deployments", err)
				os.Exit(1)
			}
		}

		// drop the schema and the message broker when requested
		if reinstallDropDatabase {
			// drop the control plane database
			if err := cpi.DropDatabase(kubeClient, &mapper); err != nil {
				restoreScaledModules()
				cli.Error("failed to drop control plane database", err)
				os.Exit(1)
			}

			// drop message broker state so streams do not outlive the schema
			if err := cpi.DropMessageBrokerState(kubeClient, &mapper); err != nil {
				restoreScaledModules()
				cli.Error("failed to drop control plane message broker state", err)
				os.Exit(1)
			}
		}

		// reinstall stateless resources
		if err := cpi.Reinstall(kubeClient, &mapper, authConfig); err != nil {
			restoreScaledModules()
			cli.Error("failed to reinstall threeport control plane", err)
			os.Exit(1)
		}

		// restore bootstrap records once the api is back up
		if reinstallDropDatabase {
			if err := cli.EnsureBootstrapObjects(cpi); err != nil {
				restoreScaledModules()
				cli.Error("failed to restore control plane bootstrap objects", err)
				os.Exit(1)
			}
		}

		// restore saved module replica counts, or no-op when none were saved
		if err := cpi.RestoreModuleScale(kubeClient, moduleScales); err != nil {
			cli.Error("failed to restore module deployments", err)
			os.Exit(1)
		}

		// report that the control plane is reinstalled
		cli.Complete("threeport control plane reinstalled")
	},
}

// init adds the reinstall command and its flags.
func init() {
	// add the reinstall command
	rootCmd.AddCommand(reinstallCmd)

	// register the name flag
	reinstallCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"name", "n", tptdev.DefaultInstanceName, "Name of dev genesis control plane.",
	)
	// register the kubeconfig flag
	reinstallCmd.Flags().StringVarP(
		&cliArgs.KubeconfigPath,
		"kubeconfig", "k", "", "Path to kubeconfig (default is $KUBECONFIG, then ~/.kube/config).",
	)
	// register the image-namespace flag
	reinstallCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneImageRepo,
		"control-plane-image-namespace", "r", "", "Image namespace to pull threeport control plane images from.",
	)
	// register the image-tag flag
	reinstallCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneImageTag,
		"control-plane-image-tag", "t", "", "Image tag for threeport control plane images. Defaults to the resolved git and build version.",
	)
	// register the debug flag
	reinstallCmd.Flags().BoolVar(
		&cliArgs.Debug,
		"debug", false, "If true, pod imagePullPolicy is set to Always so each rollout re-pulls the tag.",
	)
	// register the apis flag
	reinstallCmd.Flags().StringVar(
		&reinstallApis,
		"apis", "", "Optional. Comma-separated list of sdk-config api object group names (e.g. kubernetes_workload,gateway) to install as the resulting controller set. Controllers not named are deleted and not recreated. Use none to install zero optional controllers. Defaults to empty, which auto-detects the controller subset from the cluster's installer-managed deployments.",
	)
	// register the drop-database flag
	reinstallCmd.Flags().BoolVar(
		&reinstallDropDatabase,
		"drop-database", false, "Drop the database schema before reapplying, so the migrations run from scratch. Requires --confirm and a control plane installed at the development tier. The data cannot be recovered.",
	)
	// register the confirm flag
	reinstallCmd.Flags().StringVar(
		&reinstallConfirm,
		"confirm", "", "Name of the control plane whose database is being dropped. Must match --name. Required with --drop-database.",
	)
}
