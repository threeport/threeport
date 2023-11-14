/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"github.com/spf13/cobra"
)

// CreateCmd represents the create command
var CreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create Threeport objects",
	Long: `Create Threeport objects.

The create command does nothing by itself.  Use one of the avilable subcommands
to create different objects in the system.`,
}

func init() {
	rootCmd.AddCommand(CreateCmd)
}
