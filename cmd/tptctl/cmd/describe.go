/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

// DescribeCmd represents the describe command
var DescribeCmd = &cobra.Command{
	Use:   "describe",
	Short: "Describe a Threeport object",
	Long: `Describe a Threeport object.

The describe command does nothing by itself.  Use one of the avilable subcommands
to describe different objects from the system.`,
	Run: func(cmd *cobra.Command, args []string) {
		switch len(args) {
		case 0:
			missingErr("describe")
			os.Exit(1)
		default:
			unknownErr("describe", args[0])
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(DescribeCmd)
}
