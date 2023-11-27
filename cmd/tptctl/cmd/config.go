/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"github.com/spf13/cobra"
)

// ConfigCmd represents the config command
var ConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage your local Threeport config",
	Long: `Manage your local Threeport config.

The tptctl command line tool uses a Threeport config file to store connection and
configuration information for one or more installations of Threeport.  By default
the file lives at ~/.config/threeport/config.yaml on your filesystem.  You can edit
this config file manually if you like, but we recommend you use the config command
to do so where possible.

The config command does nothing by itself.  Use one of the avilable subcommands
to manage your Threeport config.`,
}

func init() {
	rootCmd.AddCommand(ConfigCmd)
}
