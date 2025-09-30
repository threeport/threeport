/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

// ReplaceCmd represents the replace command
var ReplaceCmd = &cobra.Command{
	Use:   "replace",
	Short: "Replace Threeport objects",
	Long: `Replace Threeport objects.

The replace command does nothing by itself.  Use one of the avilable subcommands
to replace different objects in the system.`,
	Run: func(cmd *cobra.Command, args []string) {
		switch len(args) {
		case 0:
			missingErr("replace")
			os.Exit(1)
		default:
			unknownErr("replace", args[0])
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(ReplaceCmd)
}
