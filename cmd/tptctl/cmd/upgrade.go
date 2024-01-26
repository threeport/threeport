/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"github.com/spf13/cobra"
)

// UpgradeCmd represents the get command
var UpgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrade Threeport related compoments",
	Long: `Upgrade Threeport related aspects.

The upgrade command does nothing by itself.  Use one of the avilable subcommands
to upgrade different aspects of the system.`,
}

func init() {
	rootCmd.AddCommand(UpgradeCmd)
}
