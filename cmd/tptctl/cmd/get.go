/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"github.com/spf13/cobra"
)

// GetCmd represents the get command
var GetCmd = &cobra.Command{
	Use:   "get",
	Short: "Get Threeport objects",
	Long: `Get Threeport objects.

The get command does nothing by itself.  Use one of the avilable subcommands
to get different objects from the system.`,
}

func init() {
	rootCmd.AddCommand(GetCmd)
}
