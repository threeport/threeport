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

// reinstallRestoreBootstrap recreates missing runtime and control plane records.
var reinstallRestoreBootstrap bool

// reinstallCmd reinstalls the stateless side of a dev control plane.
var reinstallCmd = &cobra.Command{
	Use:   "reinstall",
	Short: "Sweep and reapply stateless control plane resources",
	Long: `Reinstall the stateless side of a dev threeport control plane.

Sweeps every installer-managed Deployment in the control plane
namespace, then reapplies the install path so the pods come back with
the current images and specs from source.

Preserved across reinstall: cockroachdb data, nats data, the
certificate authority and signed certs, and the rest-api's external
service ip. Everything else (controller and api-server pods, their
configmaps, rbac) is recreated.

Pass --drop-database to reset the stored state as well, taking the whole
control plane from running to running with an empty schema in one
command: the control plane is scaled down, its schema is dropped by a
statement issued against the running database, every nats stream is
removed, the install reapplies, and the migrations run from scratch.

The nats streams go with the schema rather than as a separate choice.
They carry notifications naming rows by identifier, and the key-value
buckets carry reconciliation locks keyed the same way, so keeping them
across a drop leaves both pointing at rows that no longer exist. A
durable consumer also keeps whatever configuration created it, so a
delivery limit set by an older release would otherwise outlive every
later install with nothing reporting the divergence.

That data is not recoverable, so
the flag also requires --confirm with the control plane name, and the
target cluster must record itself as a development installation, which
a cloud-hosted control plane does by being installed with 'tptctl up
--tier development'.

The database's data volume, its certificates and the certificate
authority all survive a drop, so no volume is reprovisioned and no
credentials need to be re-issued or re-downloaded afterward. The
kubernetes runtime and control plane records the API needs in order to
accept work go out with the schema and are recreated from the local
threeport config once the API is back up.

Pass --restore-bootstrap to recreate those same records without
dropping anything. Use it when the database was emptied by something
other than this command, such as a database dropped by hand so that
renumbered migrations reapply from scratch. It creates only the records
that are missing, so running it against a control plane that still has
them changes nothing.

Intended for dev environments only. The reinstall command does not
build images; run 'tptdev build --push' first if the image needs to
change.`,
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
		if reinstallDropDatabase || reinstallRestoreBootstrap {
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

			// discover namespaces of registered modules
			moduleNamespaces, err := cpi.DiscoverModuleNamespaces(apiClient, controlPlaneConfig.APIServer)
			if err != nil {
				cli.Error("failed to discover registered module namespaces", err)
				os.Exit(1)
			}
			// log module namespaces before scale-down
			if len(moduleNamespaces) > 0 {
				cli.Info(fmt.Sprintf(
					"scaling down %d module namespace(s): %s",
					len(moduleNamespaces), strings.Join(moduleNamespaces, ", "),
				))
			}

			// scale module deployments to zero and keep their replica counts
			moduleScales, err = cpi.ScaleDownModules(kubeClient, moduleNamespaces)
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
		if reinstallDropDatabase || reinstallRestoreBootstrap {
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
	// register the restore-bootstrap flag
	reinstallCmd.Flags().BoolVar(
		&reinstallRestoreBootstrap,
		"restore-bootstrap", false, "Recreate the kubernetes runtime and control plane records the API needs in order to accept work, for a database emptied outside this command. Creates only the records that are missing. Implied by --drop-database.",
	)
}
