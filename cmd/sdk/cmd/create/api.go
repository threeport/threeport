/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package api

import (
	"github.com/spf13/cobra"
)

// apiCmd represents the api object to create
var apiCmd = &cobra.Command{
	Use:   "api",
	Short: "Manage threeport api objects.",
	Long:  `Manage threeport api objects.`,
}

func init() {
	createCmd.AddCommand(apiCmd)
}
