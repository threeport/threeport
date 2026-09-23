/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	auth "github.com/threeport/threeport/pkg/auth/v0"
	cli "github.com/threeport/threeport/pkg/cli/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
	installer "github.com/threeport/threeport/pkg/threeport-installer/v0"
)

// The api object groups to install, empty to detect the controller set from the cluster
var tptctlReinstallApis string

// A flag that drops the database and message broker state before reinstall
var tptctlReinstallDropDatabase bool

// The control plane name required before a database drop proceeds
var tptctlReinstallConfirm string

// A flag that recreates the records the API needs after an emptied database
var tptctlReinstallRestoreBootstrap bool

// ReinstallCmd deletes installer-managed control plane resources and reapplies the install.
var ReinstallCmd = &cobra.Command{
	Use:   "reinstall",
	Short: "Sweep and reapply stateless control plane resources",
	Long: `Reinstall the stateless side of a Threeport control plane.

Sweeps every installer-managed Deployment in the control plane
namespace, then reapplies the install path so the pods come back with
the current images and specs.

Preserved across reinstall: cockroachdb data, nats data, the
certificate authority and signed certs, and the rest-api's external
service ip. Everything else (controller and api-server pods, their
configmaps, rbac) is recreated.

Pass --drop-database to reset the stored state as well. That flag
requires --confirm with the control plane name, and the target cluster
must record itself as a development installation.

Use this for any spec, RBAC, or configmap change. Use
'tptctl upgrade control-plane' to change Deployment image tags only.`,
	SilenceUsage: true,
	PreRun:       CommandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		// get client context
		apiClient, config, apiEndpoint, requestedControlPlane := GetClientContext(cmd)
		cliArgs.ControlPlaneName = requestedControlPlane

		// require the control plane name before a database drop
		if tptctlReinstallDropDatabase && tptctlReinstallConfirm != requestedControlPlane {
			cli.Error(fmt.Sprintf(
				"--drop-database destroys the database of control plane %q and its data cannot be recovered; re-run with --confirm %s to proceed",
				requestedControlPlane, requestedControlPlane,
			), nil)
			os.Exit(1)
		}

		// get the encryption key
		encryptionKey, err := config.GetThreeportEncryptionKey(requestedControlPlane)
		if err != nil {
			cli.Error("failed to retrieve encryption key for control plane", err)
			os.Exit(1)
		}

		// get the kubernetes runtime instance
		kubernetesRuntimeInstance, err := client.GetThreeportControlPlaneKubernetesRuntimeInstance(
			apiClient,
			apiEndpoint,
		)
		if err != nil {
			cli.Error("failed to retrieve kubernetes runtime instance from threeport API", err)
			os.Exit(1)
		}

		// get the control plane instance
		controlPlaneInstance, err := client.GetSelfControlPlaneInstance(
			apiClient,
			apiEndpoint,
		)
		if err != nil {
			cli.Error("failed to retrieve self control plane instance from threeport API", err)
			os.Exit(1)
		}

		// use the instance namespace when the API records one
		namespace := installer.ControlPlaneNamespace
		if controlPlaneInstance.Namespace != nil && *controlPlaneInstance.Namespace != "" {
			namespace = *controlPlaneInstance.Namespace
		}

		// get the kube client
		dynamicKubeClient, mapper, err := kube.GetClient(
			kubernetesRuntimeInstance,
			false,
			apiClient,
			apiEndpoint,
			encryptionKey,
		)
		if err != nil {
			cli.Error("failed to get kube client", err)
			os.Exit(1)
		}

		// create the control plane installer
		cpi, err := cliArgs.CreateInstaller()
		if err != nil {
			cli.Error("failed to create threeport control plane installer", err)
			os.Exit(1)
		}
		cpi.Opts.Namespace = namespace
		cpi.Opts.ControlPlaneName = requestedControlPlane
		cpi.Opts.Debug = cliArgs.Debug

		// select the controllers to reinstall
		selected, selectedNames, autoDetected, err := installer.SelectControllersForReinstall(
			dynamicKubeClient,
			cpi.Opts.Namespace,
			installer.ParseApis(tptctlReinstallApis),
			cpi.Opts.ControllerList,
		)
		if err != nil {
			cli.Error("failed to select controllers for reinstall", err)
			os.Exit(1)
		}
		source := "specified via --apis"
		if autoDetected {
			source = "auto-detected from cluster"
		}
		cli.Info(fmt.Sprintf(
			"reinstalling %d controller(s) (%s): %s",
			len(selected), source, strings.Join(selectedNames, ", "),
		))
		cpi.Opts.ControllerList = selected

		// detect whether api auth is enabled
		cpi.Opts.AuthEnabled = installer.DetectAuthEnabled(dynamicKubeClient, cpi.Opts.Namespace)

		// load the cluster CA when auth is enabled so reinstall does not mint one
		var authConfig *auth.AuthConfig
		if cpi.Opts.AuthEnabled {
			authConfig, err = cpi.LoadAuthConfigFromCluster(dynamicKubeClient, mapper)
			if err != nil {
				cli.Error("failed to load existing CA from cluster", err)
				os.Exit(1)
			}
		}

		// hold module replica counts for the restore after reinstall
		var moduleScales []installer.ModuleDeploymentScale
		// drop stored state when requested
		if tptctlReinstallDropDatabase {
			// get the control plane config
			controlPlaneConfig, err := config.GetControlPlaneConfig(requestedControlPlane)
			if err != nil {
				cli.Error("failed to get threeport control plane config", err)
				os.Exit(1)
			}

			// discover registered module namespaces
			moduleNamespaces, err := cpi.DiscoverModuleNamespaces(apiClient, controlPlaneConfig.APIServer)
			if err != nil {
				cli.Error("failed to discover registered module namespaces", err)
				os.Exit(1)
			}
			if len(moduleNamespaces) > 0 {
				cli.Info(fmt.Sprintf(
					"scaling down %d module namespace(s): %s",
					len(moduleNamespaces), strings.Join(moduleNamespaces, ", "),
				))
			}

			// scale module deployments to zero
			moduleScales, err = cpi.ScaleDownModules(dynamicKubeClient, moduleNamespaces)
			if err != nil {
				cli.Error("failed to scale down module deployments", err)
				os.Exit(1)
			}

			// drop the control plane database
			if err := cpi.DropDatabase(dynamicKubeClient, mapper); err != nil {
				cli.Error("failed to drop control plane database", err)
				os.Exit(1)
			}

			// drop message broker state
			if err := cpi.DropMessageBrokerState(dynamicKubeClient, mapper); err != nil {
				cli.Error("failed to drop control plane message broker state", err)
				os.Exit(1)
			}
		}

		// reinstall the control plane
		if err := cpi.Reinstall(dynamicKubeClient, mapper, authConfig); err != nil {
			cli.Error("failed to reinstall threeport control plane", err)
			os.Exit(1)
		}

		// restore bootstrap objects when the database was emptied
		if tptctlReinstallDropDatabase || tptctlReinstallRestoreBootstrap {
			if err := cli.EnsureBootstrapObjects(cpi); err != nil {
				cli.Error("failed to restore control plane bootstrap objects", err)
				os.Exit(1)
			}
		}

		// restore module deployment scale
		if err := cpi.RestoreModuleScale(dynamicKubeClient, moduleScales); err != nil {
			cli.Error("failed to restore module deployments", err)
			os.Exit(1)
		}

		// report reinstall complete
		cli.Complete("threeport control plane reinstalled")
	},
}

// init registers the reinstall command and its flags.
func init() {
	// register the reinstall command
	rootCmd.AddCommand(ReinstallCmd)

	// register reinstall flags

	ReinstallCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"name", "n", "", "Name of genesis control plane.",
	)
	ReinstallCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneImageRepo,
		"control-plane-image-namespace", "r", "", "Image namespace to pull threeport control plane images from.",
	)
	ReinstallCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneImageTag,
		"control-plane-image-tag", "t", "", "Image tag for threeport control plane images.",
	)
	ReinstallCmd.Flags().BoolVar(
		&cliArgs.Debug,
		"debug", false, "If true, pod imagePullPolicy is set to Always so each rollout re-pulls the tag.",
	)
	ReinstallCmd.Flags().StringVar(
		&tptctlReinstallApis,
		"apis", "", "Optional. Comma-separated list of sdk-config api object group names to install as the resulting controller set. Defaults to empty, which auto-detects the controller subset from the cluster.",
	)
	ReinstallCmd.Flags().BoolVar(
		&tptctlReinstallDropDatabase,
		"drop-database", false, "Drop the database schema before reapplying, so the migrations run from scratch. Requires --confirm and a control plane installed at the development tier. The data cannot be recovered.",
	)
	ReinstallCmd.Flags().StringVar(
		&tptctlReinstallConfirm,
		"confirm", "", "Name of the control plane whose database is being dropped. Must match --name. Required with --drop-database.",
	)
	ReinstallCmd.Flags().BoolVar(
		&tptctlReinstallRestoreBootstrap,
		"restore-bootstrap", false, "Recreate the kubernetes runtime and control plane records the API needs in order to accept work, for a database emptied outside this command. Implied by --drop-database.",
	)
}
