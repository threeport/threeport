/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"net/http"
	"os"

	"github.com/spf13/cobra"

	cli "github.com/threeport/threeport/pkg/cli/v0"
)

var cliArgs = &cli.GenesisControlPlaneCLIArgs{}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "tptctl",
	Short: "Manage Threeport",
	Long: `Threeport is a global control plane for your software.  The tptctl
CLI installs and manages instances of the Threeport control plane as well as
applications that are deployed into the Threeport compute space.`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		cli.Error("", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(
		&cliArgs.CfgFile, "threeport-config", "", "Path to config file (default is $HOME/.config/threeport/config.yaml).",
	)
	rootCmd.PersistentFlags().StringVar(
		&cliArgs.ProviderConfigDir, "provider-config", "", "Path to infra provider config directory (default is $HOME/.config/threeport/).",
	)
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	cobra.OnInitialize(func() {
		cli.InitConfig(cliArgs.CfgFile)
		cli.InitArgs(cliArgs)
	})
}

func checkContext(cmd *cobra.Command) (*http.Client, string) {
	var apiClient *http.Client
	var apiEndpoint string

	contextApiClient := cmd.Context().Value("apiClient")
	if contextApiClient != nil {
		if client, ok := contextApiClient.(*http.Client); ok {
			apiClient = client
		}
	}

	contextApiEndpoint := cmd.Context().Value("apiEndpoint")
	if contextApiEndpoint != nil {
		if client, ok := contextApiEndpoint.(string); ok {
			apiEndpoint = client
		}
	}

	return apiClient, apiEndpoint
}
