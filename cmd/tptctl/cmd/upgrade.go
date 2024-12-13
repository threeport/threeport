/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

// UpgradeCmd represents the get command
var UpgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrade Threeport related compoments",
	Long: `Upgrade Threeport related aspects.

The upgrade command does nothing by itself.  Use one of the avilable subcommands
to upgrade different aspects of the system.`,
	Run: func(cmd *cobra.Command, args []string) {
		switch len(args) {
		case 0:
			missingErr("upgrade")
			os.Exit(1)
		default:
			unknownErr("upgrade", args[0])
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(UpgradeCmd)
}
