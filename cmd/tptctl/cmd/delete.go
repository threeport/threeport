/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"github.com/spf13/cobra"
)

// DeleteCmd represents the delete command
var DeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete Threeport objects",
	Long: `Delete Threeport objects.

The delete command does nothing by itself.  Use one of the avilable subcommands
to delete different objects in the system.`,
}

func init() {
	rootCmd.AddCommand(DeleteCmd)
}
