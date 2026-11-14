/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	cli "github.com/threeport/threeport/pkg/cli/v0"
	clientapi "github.com/threeport/threeport/pkg/client/v0"
	installer "github.com/threeport/threeport/pkg/threeport-installer/v0"
)

// The backup to restore: LATEST or a backup path in the bucket
var tptctlRestoreBackup string

// The bucket to restore from, when not the one recorded for the control plane
var tptctlRestoreBucket string

// The control plane name required before the database is replaced
var tptctlRestoreConfirm string
var tptctlRestoreNamespace string
var tptctlRestoreFromNamespace string
var tptctlRestoreStopControllers bool

// The control plane instance to restore, when not the current one
var tptctlRestoreControlPlane string

// RestoreCmd replaces a control plane's database with one of its backups.
var RestoreCmd = &cobra.Command{
	Use:     "restore",
	Example: "tptctl restore --name my-threeport --confirm my-threeport",
	Short:   "Restore a control plane's database from a backup",
	Long: `Restore a Threeport control plane's database from one of its backups.

A control plane that records a backup bucket is restored from that bucket,
or from the backup named by --backup. The control plane is scaled down
while the database is replaced and scaled back up after.

The backup is restored alongside the database first and swapped in only
once it is complete, so a restore that fails before the swap leaves the
database as it was and brings the control plane back on it. One that
fails during the swap leaves the control plane scaled down, since the
database may be part way; running the restore again finishes it. Everything written since the backup was taken is lost, so the
command requires --confirm with the control plane name.

The cluster is reached with your own credentials for its provider rather
than through the Threeport API, so a restore works when the API is down.`,
	SilenceUsage: true,
	PreRun:       CommandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		// get the control plane's config
		threeportConfig, requestedControlPlane, err := cli.GetThreeportConfig(cliArgs.ControlPlaneName)
		if err != nil {
			cli.Error("failed to get threeport config", err)
			os.Exit(1)
		}
		controlPlane, err := threeportConfig.GetControlPlaneConfig(requestedControlPlane)
		if err != nil {
			cli.Error("failed to get threeport control plane config", err)
			os.Exit(1)
		}

		// require the control plane name before the database is replaced
		confirmName := requestedControlPlane
		if tptctlRestoreControlPlane != "" {
			confirmName = tptctlRestoreControlPlane
		}
		if tptctlRestoreConfirm != confirmName {
			cli.Error(fmt.Sprintf(
				"restoring replaces the database of control plane %q and loses everything written since the backup; re-run with --confirm %s to proceed",
				confirmName, confirmName,
			), nil)
			os.Exit(1)
		}

		// reject a child before the database is replaced.
		// The API being down is how a root restore still runs, so a child that cannot be read is not stopped here.
		apiClient, err := threeportConfig.GetHTTPClient(requestedControlPlane)
		if err != nil {
			cli.Error("failed to get threeport http client", err)
			os.Exit(1)
		}
		apiEndpoint, err := threeportConfig.GetThreeportAPIEndpoint(requestedControlPlane)
		if err != nil {
			cli.Error("failed to get threeport API endpoint", err)
			os.Exit(1)
		}
		if instance, err := clientapi.GetSelfControlPlaneInstance(apiClient, apiEndpoint); err != nil {
			cli.Warning(fmt.Sprintf("could not read the control plane to check its parent: %s", err))
		} else if err := api_v0.ChildControlPlaneRequest(instance); err != nil {
			cli.Error(err.Error(), nil)
			os.Exit(1)
		}

		restored := requestedControlPlane
		if tptctlRestoreControlPlane != "" {
			restored = tptctlRestoreControlPlane
			encryptionKey, err := threeportConfig.GetThreeportEncryptionKey(requestedControlPlane)
			if err != nil {
				cli.Error("failed to get encryption key", err)
				os.Exit(1)
			}
			cli.Info(fmt.Sprintf("restoring backup %s into control plane %s", tptctlRestoreBackup, restored))
			err = cli.RestoreNamedControlPlane(
				controlPlane,
				apiClient,
				apiEndpoint,
				encryptionKey,
				tptctlRestoreControlPlane,
				tptctlRestoreBucket,
				tptctlRestoreBackup,
				tptctlRestoreNamespace,
				tptctlRestoreFromNamespace,
				tptctlRestoreStopControllers,
			)
			if err != nil {
				cli.Error("failed to restore the control plane database", err)
				os.Exit(1)
			}
		} else {
			bucket, err := cli.RestoreBackupBucket(controlPlane, tptctlRestoreBucket)
			if err != nil {
				cli.Error("failed to find the backups to restore", err)
				os.Exit(1)
			}

			cli.Info(fmt.Sprintf("restoring backup %s from gs://%s into control plane %s", tptctlRestoreBackup, bucket, requestedControlPlane))
			if err := cli.RestoreControlPlaneDatabase(controlPlane, bucket, tptctlRestoreBackup, tptctlRestoreNamespace, tptctlRestoreFromNamespace, tptctlRestoreStopControllers); err != nil {
				cli.Error("failed to restore the control plane database", err)
				os.Exit(1)
			}
			if !tptctlRestoreStopControllers {
				if err := cli.RedeployRestoredModules(controlPlane, tptctlRestoreNamespace, apiClient, apiEndpoint); err != nil {
					cli.Error("failed to retarget installed modules", err)
					os.Exit(1)
				}
			}

			// record the restored control plane in the local config
			if err := recordRestoredControlPlane(
				threeportConfig,
				controlPlane,
				tptctlRestoreNamespace,
			); err != nil {
				cli.Error("failed to update the local control plane config", err)
				os.Exit(1)
			}
		}

		cli.Complete(fmt.Sprintf("control plane %s restored from backup %s", restored, tptctlRestoreBackup))
	},
}

// recordRestoredControlPlane writes the restored control plane into the local config and makes it current.
// A namespace other than the named control plane is registered from the API on the same cluster.
func recordRestoredControlPlane(
	threeportConfig *cli.ThreeportConfig,
	parent *cli.ControlPlane,
	namespace string,
) error {
	// the named control plane keeps the client material from install
	if cli.RestoringNamedControlPlane(parent.Name, namespace) {
		return cli.RecordRestoredParent(threeportConfig, parent.Name, namespace)
	}

	// read the child from the API that manages it
	apiClient, err := threeportConfig.GetHTTPClient(parent.Name)
	if err != nil {
		return err
	}
	apiEndpoint, err := threeportConfig.GetThreeportAPIEndpoint(parent.Name)
	if err != nil {
		return err
	}
	instance, err := clientapi.GetControlPlaneInstanceByName(
		apiClient,
		apiEndpoint,
		namespace,
	)
	if err != nil {
		return err
	}

	// store the child's client material, cluster location, and encryption key
	return updateThreeportConfigWithControlPlaneInstance(
		apiClient,
		apiEndpoint,
		instance,
		threeportConfig,
	)
}

// init registers the restore command and its flags.
func init() {
	rootCmd.AddCommand(RestoreCmd)

	RestoreCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"name", "n", "", "Name of the control plane to restore. Defaults to the current control plane.",
	)
	RestoreCmd.Flags().StringVar(
		&tptctlRestoreBackup,
		"backup", installer.LatestBackup, "The backup to restore: LATEST, or a backup path such as /2026/10/20-050726.51 as listed under threeport_api/ in the bucket.",
	)
	RestoreCmd.Flags().StringVar(
		&tptctlRestoreBucket,
		"bucket", "", "The GCS bucket to restore from. Defaults to the bucket recorded when the control plane was installed.",
	)
	RestoreCmd.Flags().StringVar(
		&tptctlRestoreConfirm,
		"confirm", "", "Name of the control plane whose database is being replaced. Must match the control plane being restored.",
	)
	RestoreCmd.Flags().StringVar(
		&tptctlRestoreNamespace,
		"namespace", "", "Namespace of the control plane to restore. Defaults to threeport-control-plane.",
	)
	RestoreCmd.Flags().StringVar(
		&tptctlRestoreFromNamespace,
		"from-namespace", "", "Namespace whose backup is restored. Defaults to the namespace being restored.",
	)
	RestoreCmd.Flags().BoolVar(
		&tptctlRestoreStopControllers,
		"stop-controllers", false, "Leave the controllers scaled down after the restore. The API is scaled back up.",
	)
	RestoreCmd.Flags().StringVar(
		&tptctlRestoreControlPlane,
		"control-plane-name", "", "Name of a control plane instance to restore. A child cannot restore itself.",
	)
}
