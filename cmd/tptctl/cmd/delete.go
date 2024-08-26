/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

// DeleteCmd represents the delete command
var DeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete Threeport objects",
	Long: `Delete Threeport objects.

The delete command does nothing by itself.  Use one of the avilable subcommands
to delete different objects in the system.`,
	Run: func(cmd *cobra.Command, args []string) {
		switch len(args) {
		case 0:
			missingErr("delete")
			os.Exit(1)
		default:
			unknownErr("delete", args[0])
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(DeleteCmd)
}
