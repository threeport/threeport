/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/threeport/threeport/internal/version"
)

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version of tptctl",
	Long:  `Print the version of tptctl.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(version.GetVersion())
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
