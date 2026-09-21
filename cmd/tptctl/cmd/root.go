/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"

	cli "github.com/threeport/threeport/pkg/cli/v0"
)

var cliArgs = &cli.GenesisControlPlaneCLIArgs{}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "tptctl",
	Short: "Manage Threeport",
	Long: `tptctl is a CLI tool for managing application delivery with Threeport.
It installs and manages Threeport iself, and provides commands for managing
your application delivery using Threeport.  Threeport manages the infrastructure,
runtime environments, managed service dependencies, installed support services,
as well as all component workloads of your application.

Plugins: tptctl plugins are installed at ~/.threeport/plugins.  If you install a
tptctl plugin in an alternative location, set the THREEPORT_PLUGIN_DIR environment
variable with the alternative install directory.

Visit https://threeport.io for more information.`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	// find installed plugins
	pluginDir, ok := os.LookupEnv("THREEPORT_PLUGIN_DIR")
	if !ok {
		p, err := cli.DefaultPluginDir()
		if err != nil {
			cli.Error("failed to determine default tptctl plugin directory", err)
			os.Exit(1)
		}
		pluginDir = p
	}
	installedPlugins := loadPlugins(pluginDir)

	// validate that no naming colisions exist between plugins and core commands
	var validatedPlugins []string
	for _, plug := range installedPlugins {
		validated := true
		for _, cmd := range rootCmd.Commands() {
			if cmd.Use == filepath.Base(plug) {
				cli.Warning(fmt.Sprintf(
					"plugin '%s' conflicts with core 'tptctl %s' command - plugin ignored",
					plug,
					cmd.Use,
				))
				validated = false
			}
		}
		if validated {
			validatedPlugins = append(validatedPlugins, plug)
		}
	}

	// call plugin executable if given as first arg to tptctl
	for _, plugFile := range validatedPlugins {
		if len(os.Args) > 1 && os.Args[1] == filepath.Base(plugFile) {
			plugArgs := os.Args[2:]
			plugCmd := exec.Command(plugFile, plugArgs...)
			// wire the plugin's stdio directly to tptctl's so its output
			// streams live rather than being buffered until the process exits
			plugCmd.Stdin = os.Stdin
			plugCmd.Stdout = os.Stdout
			plugCmd.Stderr = os.Stderr
			if err := plugCmd.Run(); err != nil {
				// preserve the plugin's own exit code when it exits non-zero
				if exitErr, ok := err.(*exec.ExitError); ok {
					os.Exit(exitErr.ExitCode())
				}
				cli.Error(
					fmt.Sprintf("failed to run plugin %s", filepath.Base(plugFile)),
					err,
				)
				os.Exit(1)
			}
			os.Exit(0)
		}
	}

	// execute core commands
	err := rootCmd.Execute()
	if err != nil {
		cli.Error("", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(
		&cliArgs.CfgFile, "threeport-config", "", "Path to config file (default is $HOME/.threeport/config.yaml). Can also be set with environment variable THREEPORT_CONFIG",
	)
	rootCmd.PersistentFlags().StringVar(
		&cliArgs.ProviderConfigDir, "provider-config", "", "Path to infra provider config directory (default is $HOME/.threeport/).",
	)
	cobra.OnInitialize(func() {
		cli.InitArgs(cliArgs)
	})
}

// CommandPreRunFunc is a function that is called before the command is executed.
// It initializes the config and the command context.
func CommandPreRunFunc(cmd *cobra.Command, args []string) {
	cli.InitConfig(cmd, cliArgs.CfgFile)

	if err := initializeCommandContext(cmd); err != nil {
		cli.Error("could not initialize command in pre run:", err)
		os.Exit(1)
	}
}

// initializeCommandContext initializes the command context.
func initializeCommandContext(cmd *cobra.Command) error {
	// get threeport config and extract threeport API endpoint
	threeportConfig, requestedControlPlane, err := cli.GetThreeportConfig(cliArgs.ControlPlaneName)
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

// GetClientContext gets the client context from the command context.
func GetClientContext(cmd *cobra.Command) (*http.Client, *cli.ThreeportConfig, string, string) {
	var apiClient *http.Client
	var threeportConfig *cli.ThreeportConfig
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
		if config, ok := contextThreeportConfig.(*cli.ThreeportConfig); ok {
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

// loadPlugins loads all tptctl plugins in the plugin dir.
func loadPlugins(pluginDir string) []string {
	// check if plugin dir exists
	if _, err := os.Stat(pluginDir); err != nil {
		if os.IsNotExist(err) {
			// plugin dir does not exist - assume no plugins installed
			cli.Warning("no plugins installed")
			return []string{}
		} else {
			cli.Error("failed to check for plugin directory", err)
			os.Exit(1)
		}
	}

	// walk through the plugin directory to collect plugin files
	var pluginFiles []string
	err := filepath.WalkDir(pluginDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// if there is an error walking the directory, just skip it
			return nil
		}
		if !d.IsDir() {
			pluginFiles = append(pluginFiles, path)
		}
		return nil
	})
	if err != nil {
		cli.Error("failed to collect plugin files", err)
		os.Exit(1)
	}

	return pluginFiles
}
