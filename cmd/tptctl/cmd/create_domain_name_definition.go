/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"

	cli "github.com/threeport/threeport/pkg/cli/v0"
	config "github.com/threeport/threeport/pkg/config/v0"
)

var createDomainNameDefinitionConfigPath string

// CreateDomainNameDefinitionCmd represents the domain-name-definition command
var CreateDomainNameDefinitionCmd = &cobra.Command{
	Use:          "domain-name-definition",
	Example:      "tptctl create domain-name-definition --config /path/to/config.yaml",
	Short:        "Create a new domain name definition",
	Long:         `Create a new domain name definition.`,
	SilenceUsage: true,
	Run: func(cmd *cobra.Command, args []string) {
		// get threeport config and extract threeport API endpoint
		threeportConfig, requestedControlPlane, err := config.GetThreeportConfig(cliArgs.ControlPlaneName)
		if err != nil {
			cli.Error("failed to get threeport config", err)
			os.Exit(1)
		}
		apiEndpoint, err := threeportConfig.GetThreeportAPIEndpoint(requestedControlPlane)
		if err != nil {
			cli.Error("failed to get threeport API endpoint from config", err)
			os.Exit(1)
		}

		// load domain name definition config
		configContent, err := os.ReadFile(createDomainNameDefinitionConfigPath)
		if err != nil {
			cli.Error("failed to read config file", err)
			os.Exit(1)
		}
		var domainNameDefinitionConfig config.DomainNameDefinitionConfig
		if err := yaml.Unmarshal(configContent, &domainNameDefinitionConfig); err != nil {
			cli.Error("failed to unmarshal config file yaml content", err)
			os.Exit(1)
		}

		// get threeport API client
		apiClient, err := threeportConfig.GetHTTPClient(requestedControlPlane)
		if err != nil {
			cli.Error("failed to get threeport API client", err)
			os.Exit(1)
		}

		// create domain name definition
		domainNameDefinition := domainNameDefinitionConfig.DomainNameDefinition
		wd, err := domainNameDefinition.CreateIfNotExist(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to create domain name definition", err)
			os.Exit(1)
		}

		cli.Complete(fmt.Sprintf("domain name definition %s created", *wd.Name))
	},
}

func init() {
	CreateCmd.AddCommand(CreateDomainNameDefinitionCmd)

	CreateDomainNameDefinitionCmd.Flags().StringVarP(
		&createDomainNameDefinitionConfigPath,
		"config", "c", "", "Path to file with domain name definition config.",
	)
	CreateDomainNameDefinitionCmd.MarkFlagRequired("config")
	CreateDomainNameDefinitionCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
}
