/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package api

import (
	"github.com/spf13/cobra"
	"github.com/threeport/threeport/cmd/sdk/cmd"
)

// createCmd represents the createCmd command
var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create initial source code scaffolding.",
	Long:  `Create initial source code scaffolding for Threeport API or its extensions.`,
}

// init initializes the create subcommand
func init() {

	cmd.RootCmd.AddCommand(createCmd)
}
