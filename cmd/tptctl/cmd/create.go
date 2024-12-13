/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

// CreateCmd represents the create command
var CreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create Threeport objects",
	Long: `Create Threeport objects.

The create command does nothing by itself.  Use one of the avilable subcommands
to create different objects in the system.`,
	Run: func(cmd *cobra.Command, args []string) {
		switch len(args) {
		case 0:
			missingErr("create")
			os.Exit(1)
		default:
			unknownErr("create", args[0])
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(CreateCmd)
}
