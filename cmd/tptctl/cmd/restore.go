/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	cli "github.com/threeport/threeport/pkg/cli/v0"
	installer "github.com/threeport/threeport/pkg/threeport-installer/v0"
)

// The backup to restore: LATEST or a backup path in the bucket
var tptctlRestoreBackup string

// The bucket to restore from, when not the one recorded for the control plane
var tptctlRestoreBucket string

// The control plane name required before the database is replaced
var tptctlRestoreConfirm string

// RestoreCmd replaces a control plane's database with one of its backups.
var RestoreCmd = &cobra.Command{
	Use:     "restore",
	Example: "tptctl restore --name my-threeport --confirm my-threeport",
	Short:   "Restore a control plane's database from a backup",
	Long: `Restore a Threeport control plane's database from one of its backups.

Production control planes on GKE back their database up to a GCS bucket
in another region. This replaces the database with the latest backup in
that bucket, or with the one named by --backup. The control plane is
scaled down while the database is replaced and scaled back up after.

The backup is restored alongside the database first and swapped in only
once it is complete, so a restore that fails leaves the database as it
was. Everything written since the backup was taken is lost, so the
command requires --confirm with the control plane name.

The cluster is reached through GKE with your own Google credentials
rather than through the Threeport API, so a restore works when the API
is down.`,
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
		if tptctlRestoreConfirm != requestedControlPlane {
			cli.Error(fmt.Sprintf(
				"restoring replaces the database of control plane %q and loses everything written since the backup; re-run with --confirm %s to proceed",
				requestedControlPlane, requestedControlPlane,
			), nil)
			os.Exit(1)
		}

		bucket, err := cli.RestoreBackupBucket(controlPlane, tptctlRestoreBucket)
		if err != nil {
			cli.Error("failed to find the backups to restore", err)
			os.Exit(1)
		}

		cli.Info(fmt.Sprintf("restoring backup %s from gs://%s into control plane %s", tptctlRestoreBackup, bucket, requestedControlPlane))
		if err := cli.RestoreControlPlaneDatabase(controlPlane, bucket, tptctlRestoreBackup); err != nil {
			cli.Error("failed to restore the control plane database", err)
			os.Exit(1)
		}

		cli.Complete(fmt.Sprintf("control plane %s restored from backup %s", requestedControlPlane, tptctlRestoreBackup))
	},
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
}
