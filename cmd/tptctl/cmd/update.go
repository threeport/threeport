/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"github.com/spf13/cobra"
)

// updateCmd represents the update command
var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update Threeport objects",
	Long: `Update Threeport objects.

The update command does nothing by itself.  Use one of the avilable subcommands
to update different objects in the system.`,
}

func init() {
	rootCmd.AddCommand(updateCmd)
}
