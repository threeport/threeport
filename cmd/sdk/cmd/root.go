/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:   "threeport-sdk",
	Short: "Develop and maintain Threeport and extensions to Threeport",
	Long: `Develop and maintain Threeport and extensions to Threeport.

This command line tool is used to generate source code scaffolding and
boilerplate for the core Threeport project as well as extensions to
Threeport which are developed out-of-tree.`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := RootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	RootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
