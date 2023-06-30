/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"github.com/spf13/cobra"
)

// describeCmd represents the describe command
var describeCmd = &cobra.Command{
	Use:   "describe",
	Short: "Describe a Threeport object",
	Long: `Describe a Threeport object.

The describe command does nothing by itself.  Use one of the avilable subcommands
to describe different objects from the system.`,
}

func init() {
	rootCmd.AddCommand(describeCmd)
}
