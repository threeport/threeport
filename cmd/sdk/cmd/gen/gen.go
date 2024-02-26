/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package gen

import (
	"github.com/spf13/cobra"
	"github.com/threeport/threeport/cmd/sdk/cmd"
)

// genCmd represents the parent command for all codegen related operations
var genCmd = &cobra.Command{
	Use:   "gen",
	Short: "Generate code for Threeport or its extensions.",
	Long:  `Generate code for Threeport or its extensions.`,
}

func init() {
	cmd.RootCmd.AddCommand(genCmd)
}
