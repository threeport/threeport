/*
Copyright © 2023 Threeport admin@threeport.io
*/

package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	cli "github.com/threeport/threeport/pkg/cli/v0"
)

// The namespace of the control plane to back up
var tptctlBackupNamespace string

// The bucket to back up to, when the control plane has none recorded
var tptctlBackupBucket string

// The control plane instance to back up, when not the current one
var tptctlBackupControlPlane string

// BackupCmd scales a control plane down and backs up its database.
var BackupCmd = &cobra.Command{
	Use:          "backup",
	Example:      "tptctl backup --name my-threeport",
	Short:        "Scale a control plane down and back up its database",
	SilenceUsage: true,
	PreRun:       CommandPreRunFunc,
	Long: `Scale registered module deployments down, then the control plane, and back up its database.

Module names are read from the API before that API is scaled down.
The writers are scaled back up after the backup. The backup goes to the bucket recorded
when the control plane was installed, or to the bucket given with --bucket.
The cluster is reached with your own credentials for that provider.`,
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

		// read module names from the API before it is scaled down
		apiClient, _, apiEndpoint, _ := GetClientContext(cmd)

		// scale modules and the control plane down, then back up the database
		backedUp := requestedControlPlane
		if tptctlBackupControlPlane != "" {
			backedUp = tptctlBackupControlPlane
			var encryptionKey string
			encryptionKey, err = threeportConfig.GetThreeportEncryptionKey(requestedControlPlane)
			if err != nil {
				cli.Error("failed to get encryption key", err)
				os.Exit(1)
			}
			err = cli.BackupNamedControlPlane(
				controlPlane,
				apiClient,
				apiEndpoint,
				encryptionKey,
				tptctlBackupControlPlane,
				tptctlBackupNamespace,
				tptctlBackupBucket,
				cli.ResumeWriters,
			)
		} else {
			err = cli.BackupControlPlane(
				controlPlane,
				tptctlBackupNamespace,
				tptctlBackupBucket,
				apiClient,
				apiEndpoint,
				cli.ResumeWriters,
			)
		}
		if err != nil {
			cli.Error("failed to back up threeport control plane", err)
			os.Exit(1)
		}

		cli.Complete(fmt.Sprintf("control plane %s backed up", backedUp))
	},
}

// init registers the backup command and its flags.
func init() {
	// register the command and its flags
	rootCmd.AddCommand(BackupCmd)
	BackupCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"name", "n", "", "Name of the control plane to back up. Defaults to the current control plane.",
	)
	BackupCmd.Flags().StringVar(
		&tptctlBackupNamespace,
		"namespace", "", "Namespace of the control plane to back up. Defaults to threeport-control-plane.",
	)
	BackupCmd.Flags().StringVar(
		&tptctlBackupBucket,
		"bucket", "", "The GCS bucket to back up to. Defaults to the bucket recorded when the control plane was installed.",
	)
	BackupCmd.Flags().StringVar(
		&tptctlBackupControlPlane,
		"control-plane-name", "", "Name of a control plane instance to back up. A child may back itself up.",
	)
}
