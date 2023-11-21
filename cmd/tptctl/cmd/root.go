/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/spf13/cobra"

	cli "github.com/threeport/threeport/pkg/cli/v0"
	config "github.com/threeport/threeport/pkg/config/v0"
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

func commandPreRunFunc(cmd *cobra.Command, args []string) {
	if err := initializeCommandContext(cmd); err != nil {
		cli.Error("could not initialize command in pre run: %w", err)
	}
}

func initializeCommandContext(cmd *cobra.Command) error {
	// get threeport config and extract threeport API endpoint
	threeportConfig, requestedControlPlane, err := config.GetThreeportConfig(cliArgs.ControlPlaneName)
	if err != nil {
		return fmt.Errorf("failed to get threeport config: %w", err)
	}

	apiEndpoint, err := threeportConfig.GetThreeportAPIEndpoint(requestedControlPlane)
	if err != nil {
		return fmt.Errorf("failed to get threeport API endpoint from config: %w", err)
	}

	apiClient, err := threeportConfig.GetHTTPClient(requestedControlPlane)
	if err != nil {
		return fmt.Errorf("failed to create threeport API client: %w", err)
	}

	ctx := context.WithValue(cmd.Context(), "apiClient", apiClient)
	ctx = context.WithValue(ctx, "config", threeportConfig)
	ctx = context.WithValue(ctx, "apiEndpoint", apiEndpoint)
	ctx = context.WithValue(ctx, "requestedControlPlane", requestedControlPlane)
	cmd.SetContext(ctx)

	return nil
}

func getClientContext(cmd *cobra.Command) (*http.Client, *config.ThreeportConfig, string, string) {
	var apiClient *http.Client
	var threeportConfig *config.ThreeportConfig
	var apiEndpoint string
	var requestedControlPlane string

	contextApiClient := cmd.Context().Value("apiClient")
	if contextApiClient != nil {
		if client, ok := contextApiClient.(*http.Client); ok {
			apiClient = client
		}
	}

	contextThreeportConfig := cmd.Context().Value("config")
	if contextThreeportConfig != nil {
		if config, ok := contextThreeportConfig.(*config.ThreeportConfig); ok {
			threeportConfig = config
		}
	}

	contextApiEndpoint := cmd.Context().Value("apiEndpoint")
	if contextApiEndpoint != nil {
		if client, ok := contextApiEndpoint.(string); ok {
			apiEndpoint = client
		}
	}

	contextControlPlane := cmd.Context().Value("requestedControlPlane")
	if contextControlPlane != nil {
		if controlPlane, ok := contextControlPlane.(string); ok {
			requestedControlPlane = controlPlane
		}
	}

	return apiClient, threeportConfig, apiEndpoint, requestedControlPlane
}
