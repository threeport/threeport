/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package codegen

import (
	"github.com/spf13/cobra"
	"github.com/threeport/threeport/cmd/sdk/cmd"
)

// codegenCmd represents the parent command for all codegen related operations
var codegenCmd = &cobra.Command{
	Use:   "codegen",
	Short: "Generate code for Threeport or its extensions.",
	Long:  `Generate code for Threeport or its extensions.`,
}

func init() {
	cmd.RootCmd.AddCommand(codegenCmd)
}
