// generated by 'threeport-sdk gen' - do not edit

package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	ghodss_yaml "github.com/ghodss/yaml"
	cobra "github.com/spf13/cobra"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	cli "github.com/threeport/threeport/pkg/cli/v0"
	client_v0 "github.com/threeport/threeport/pkg/client/v0"
	config_v0 "github.com/threeport/threeport/pkg/config/v0"
	encryption "github.com/threeport/threeport/pkg/encryption/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
	yaml "gopkg.in/yaml.v2"
	"os"
)

///////////////////////////////////////////////////////////////////////////////
// SecretDefinition
///////////////////////////////////////////////////////////////////////////////

var getSecretDefinitionVersion string

// GetSecretDefinitionsCmd represents the secret-definition command
var GetSecretDefinitionsCmd = &cobra.Command{
	Example: "  tptctl get secret-definitions",
	Long:    "Get secret definitions from the system.",
	PreRun:  CommandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, requestedControlPlane := GetClientContext(cmd)

		switch getSecretDefinitionVersion {
		case "v0":
			// get secret definitions
			secretDefinitions, err := client_v0.GetSecretDefinitions(apiClient, apiEndpoint)
			if err != nil {
				cli.Error("failed to retrieve secret definitions", err)
				os.Exit(1)
			}

			// write the output
			if len(*secretDefinitions) == 0 {
				cli.Info(fmt.Sprintf(
					"No secret definitions currently managed by %s threeport control plane",
					requestedControlPlane,
				))
				os.Exit(0)
			}
			if err := outputGetv0SecretDefinitionsCmd(
				secretDefinitions,
				apiClient,
				apiEndpoint,
			); err != nil {
				cli.Error("failed to produce output", err)
				os.Exit(0)
			}
		default:
			cli.Error("", errors.New("unrecognized object version"))
			os.Exit(1)
		}
	},
	Short:        "Get secret definitions from the system",
	SilenceUsage: true,
	Use:          "secret-definitions",
}

func init() {
	GetCmd.AddCommand(GetSecretDefinitionsCmd)

	GetSecretDefinitionsCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
	GetSecretDefinitionsCmd.Flags().StringVarP(
		&getSecretDefinitionVersion,
		"version", "v", "v0", "Version of secret definitions object to retrieve. One of: [v0]",
	)
}

var (
	createSecretDefinitionConfigPath string
	createSecretDefinitionVersion    string
)

// CreateSecretDefinitionCmd represents the secret-definition command
var CreateSecretDefinitionCmd = &cobra.Command{
	Example: "  tptctl create secret-definition --config path/to/config.yaml",
	Long:    "Create a new secret definition.",
	PreRun:  CommandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, _ := GetClientContext(cmd)

		// read secret definition config
		configContent, err := os.ReadFile(createSecretDefinitionConfigPath)
		if err != nil {
			cli.Error("failed to read config file", err)
			os.Exit(1)
		}
		// create secret definition based on version
		switch createSecretDefinitionVersion {
		case "v0":
			var secretDefinitionConfig config_v0.SecretDefinitionConfig
			if err := yaml.UnmarshalStrict(configContent, &secretDefinitionConfig); err != nil {
				cli.Error("failed to unmarshal config file yaml content", err)
				os.Exit(1)
			}

			// create secret definition
			secretDefinition := secretDefinitionConfig.SecretDefinition
			secretDefinition.SecretConfigPath = createSecretDefinitionConfigPath
			createdSecretDefinition, err := secretDefinition.Create(apiClient, apiEndpoint)
			if err != nil {
				cli.Error("failed to create secret definition", err)
				os.Exit(1)
			}

			cli.Complete(fmt.Sprintf("secret definition %s created", *createdSecretDefinition.Name))
		default:
			cli.Error("", errors.New("unrecognized object version"))
			os.Exit(1)
		}
	},
	Short:        "Create a new secret definition",
	SilenceUsage: true,
	Use:          "secret-definition",
}

func init() {
	CreateCmd.AddCommand(CreateSecretDefinitionCmd)

	CreateSecretDefinitionCmd.Flags().StringVarP(
		&createSecretDefinitionConfigPath,
		"config", "c", "", "Path to file with secret definition config.",
	)
	CreateSecretDefinitionCmd.MarkFlagRequired("config")
	CreateSecretDefinitionCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
	CreateSecretDefinitionCmd.Flags().StringVarP(
		&createSecretDefinitionVersion,
		"version", "v", "v0", "Version of secret definitions object to create. One of: [v0]",
	)
}

var (
	deleteSecretDefinitionConfigPath string
	deleteSecretDefinitionName       string
	deleteSecretDefinitionVersion    string
)

// DeleteSecretDefinitionCmd represents the secret-definition command
var DeleteSecretDefinitionCmd = &cobra.Command{
	Example: "  # delete based on config file\n  tptctl delete secret-definition --config path/to/config.yaml\n\n  # delete based on name\n  tptctl delete secret-definition --name some-secret-definition",
	Long:    "Delete an existing secret definition.",
	PreRun:  CommandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, _ := GetClientContext(cmd)

		// flag validation
		if err := cli.ValidateConfigNameFlags(
			deleteSecretDefinitionConfigPath,
			deleteSecretDefinitionName,
			"secret definition",
		); err != nil {
			cli.Error("flag validation failed", err)
			os.Exit(1)
		}

		// delete secret definition based on version
		switch deleteSecretDefinitionVersion {
		case "v0":
			var secretDefinitionConfig config_v0.SecretDefinitionConfig
			if deleteSecretDefinitionConfigPath != "" {
				// load secret definition config
				configContent, err := os.ReadFile(deleteSecretDefinitionConfigPath)
				if err != nil {
					cli.Error("failed to read config file", err)
					os.Exit(1)
				}
				if err := yaml.UnmarshalStrict(configContent, &secretDefinitionConfig); err != nil {
					cli.Error("failed to unmarshal config file yaml content", err)
					os.Exit(1)
				}
			} else {
				secretDefinitionConfig = config_v0.SecretDefinitionConfig{
					SecretDefinition: config_v0.SecretDefinitionValues{
						Name: deleteSecretDefinitionName,
					},
				}
			}

			// delete secret definition
			secretDefinition := secretDefinitionConfig.SecretDefinition
			secretDefinition.SecretConfigPath = deleteSecretDefinitionConfigPath
			deletedSecretDefinition, err := secretDefinition.Delete(apiClient, apiEndpoint)
			if err != nil {
				cli.Error("failed to delete secret definition", err)
				os.Exit(1)
			}

			cli.Complete(fmt.Sprintf("secret definition %s deleted", *deletedSecretDefinition.Name))
		default:
			cli.Error("", errors.New("unrecognized object version"))
			os.Exit(1)
		}
	},
	Short:        "Delete an existing secret definition",
	SilenceUsage: true,
	Use:          "secret-definition",
}

func init() {
	DeleteCmd.AddCommand(DeleteSecretDefinitionCmd)

	DeleteSecretDefinitionCmd.Flags().StringVarP(
		&deleteSecretDefinitionConfigPath,
		"config", "c", "", "Path to file with secret definition config.",
	)
	DeleteSecretDefinitionCmd.Flags().StringVarP(
		&deleteSecretDefinitionName,
		"name", "n", "", "Name of secret definition.",
	)
	DeleteSecretDefinitionCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
	DeleteSecretDefinitionCmd.Flags().StringVarP(
		&deleteSecretDefinitionVersion,
		"version", "v", "v0", "Version of secret definitions object to delete. One of: [v0]",
	)
}

var (
	describeSecretDefinitionConfigPath string
	describeSecretDefinitionName       string
	describeSecretDefinitionField      string
	describeSecretDefinitionOutput     string
	describeSecretDefinitionVersion    string
)

// DescribeSecretDefinitionCmd representes the secret-definition command
var DescribeSecretDefinitionCmd = &cobra.Command{
	Example: "  # Get the plain output description for a secret definition\n  tptctl describe secret-definition -n some-secret-definition\n\n  # Get JSON output for a secret definition\n  tptctl describe secret-definition -n some-secret-definition -o json\n\n  # Get the value of the Name field for a secret definition\n  tptctl describe secret-definition -n some-secret-definition -f Name ",
	Long:    "Describe a secret definition.  This command can give you a plain output description, output all fields in JSON or YAML format, or provide the value of any specific field.\n\nNote: any values that are encrypted in the database will be redacted unless the field is specifically requested with the --field flag.",
	PreRun:  CommandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, _ := GetClientContext(cmd)

		// flag validation
		if err := cli.ValidateConfigNameFlags(
			describeSecretDefinitionConfigPath,
			describeSecretDefinitionName,
			"secret definition",
		); err != nil {
			cli.Error("flag validation failed", err)
			os.Exit(1)
		}

		if err := cli.ValidateDescribeOutputFlag(
			describeSecretDefinitionOutput,
			"secret definition",
		); err != nil {
			cli.Error("flag validation failed", err)
			os.Exit(1)
		}

		// get secret definition
		var secretDefinition interface{}
		switch describeSecretDefinitionVersion {
		case "v0":
			// load secret definition config by name or config file
			var secretDefinitionConfig config_v0.SecretDefinitionConfig
			if describeSecretDefinitionConfigPath != "" {
				configContent, err := os.ReadFile(describeSecretDefinitionConfigPath)
				if err != nil {
					cli.Error("failed to read config file", err)
					os.Exit(1)
				}
				if err := yaml.UnmarshalStrict(configContent, &secretDefinitionConfig); err != nil {
					cli.Error("failed to unmarshal config file yaml content", err)
					os.Exit(1)
				}
			} else {
				secretDefinitionConfig = config_v0.SecretDefinitionConfig{
					SecretDefinition: config_v0.SecretDefinitionValues{
						Name: describeSecretDefinitionName,
					},
				}
			}

			// get secret definition object by name
			obj, err := client_v0.GetSecretDefinitionByName(
				apiClient,
				apiEndpoint,
				secretDefinitionConfig.SecretDefinition.Name,
			)
			if err != nil {
				cli.Error("failed to retrieve secret definition details", err)
				os.Exit(1)
			}
			secretDefinition = obj

			// return plain output if requested
			if describeSecretDefinitionOutput == "plain" {
				if err := outputDescribev0SecretDefinitionCmd(
					secretDefinition.(*api_v0.SecretDefinition),
					&secretDefinitionConfig,
					apiClient,
					apiEndpoint,
				); err != nil {
					cli.Error("failed to describe secret definition", err)
					os.Exit(1)
				}
			}
		default:
			cli.Error("", errors.New("unrecognized object version"))
			os.Exit(1)
		}

		// return field value if specified
		if describeSecretDefinitionField != "" {
			fieldVal, err := util.GetObjectFieldValue(
				secretDefinition,
				describeSecretDefinitionField,
			)
			if err != nil {
				cli.Error("failed to get field value from secret definition", err)
				os.Exit(1)
			}

			// decrypt value as needed
			encrypted, err := encryption.IsEncryptedField(secretDefinition, describeSecretDefinitionField)
			if err != nil {
				cli.Error("", err)
			}
			if encrypted {
				// get encryption key from threeport config
				threeportConfig, requestedControlPlane, err := config_v0.GetThreeportConfig(cliArgs.ControlPlaneName)
				if err != nil {
					cli.Error("failed to get threeport config: %w", err)
					os.Exit(1)
				}
				encryptionKey, err := threeportConfig.GetThreeportEncryptionKey(requestedControlPlane)
				if err != nil {
					cli.Error("failed to get encryption key from threeport config: %w", err)
					os.Exit(1)
				}

				// decrypt value for output
				decryptedVal, err := encryption.Decrypt(encryptionKey, fieldVal.String())
				if err != nil {
					cli.Error("failed to decrypt value: %w", err)
				}
				fmt.Println(decryptedVal)
				os.Exit(0)
			} else {
				fmt.Println(fieldVal.Interface())
				os.Exit(0)
			}
		}

		// produce json or yaml output if requested
		switch describeSecretDefinitionOutput {
		case "json":
			// redact encrypted values
			redactedSecretDefinition := encryption.RedactEncryptedValues(secretDefinition)

			// marshal to JSON then print
			secretDefinitionJson, err := json.MarshalIndent(redactedSecretDefinition, "", "  ")
			if err != nil {
				cli.Error("failed to marshal secret definition into JSON", err)
				os.Exit(1)
			}

			fmt.Println(string(secretDefinitionJson))
		case "yaml":
			// redact encrypted values
			redactedSecretDefinition := encryption.RedactEncryptedValues(secretDefinition)

			// marshal to JSON then convert to YAML - this results in field
			// names with correct capitalization vs marshalling directly to YAML
			secretDefinitionJson, err := json.MarshalIndent(redactedSecretDefinition, "", "  ")
			if err != nil {
				cli.Error("failed to marshal secret definition into JSON", err)
				os.Exit(1)
			}
			secretDefinitionYaml, err := ghodss_yaml.JSONToYAML(secretDefinitionJson)
			if err != nil {
				cli.Error("failed to convert secret definition JSON to YAML", err)
				os.Exit(1)
			}

			fmt.Println(string(secretDefinitionYaml))
		}
	},
	Short:        "Describe a secret definition",
	SilenceUsage: true,
	Use:          "secret-definition",
}

func init() {
	DescribeCmd.AddCommand(DescribeSecretDefinitionCmd)

	DescribeSecretDefinitionCmd.Flags().StringVarP(
		&describeSecretDefinitionConfigPath,
		"config", "c", "", "Path to file with secret definition config.",
	)
	DescribeSecretDefinitionCmd.Flags().StringVarP(
		&describeSecretDefinitionName,
		"name", "n", "", "Name of secret definition.",
	)
	DescribeSecretDefinitionCmd.Flags().StringVarP(
		&describeSecretDefinitionOutput,
		"output", "o", "plain", "Output format for object description. One of 'plain','json','yaml'.  Will be ignored if the --field flag is also used.  Plain output produces select details about the object.  JSON and YAML output formats include all direct attributes of the object",
	)
	DescribeSecretDefinitionCmd.Flags().StringVarP(
		&describeSecretDefinitionField,
		"field", "f", "", "Object field to get value for. If used, --output flag will be ignored.  *Only* the value of the desired field will be returned.  Will not return information on related objects, only direct attributes of the object itself.",
	)
	DescribeSecretDefinitionCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
	DescribeSecretDefinitionCmd.Flags().StringVarP(
		&describeSecretDefinitionVersion,
		"version", "v", "v0", "Version of secret definitions object to describe. One of: [v0]",
	)
}

///////////////////////////////////////////////////////////////////////////////
// Secret
///////////////////////////////////////////////////////////////////////////////

// GetSecretsCmd represents the secret command
var GetSecretsCmd = &cobra.Command{
	Example: "  tptctl get secrets",
	Long:    "Get secrets from the system.\n\nA secret is a simple abstraction of secret definitions and secret instances.\nThis command displays all instances and the definitions used to configure them.",
	PreRun:  CommandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, requestedControlPlane := GetClientContext(cmd)

		// get secrets
		v0secretInstances, err := client_v0.GetSecretInstances(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to retrieve secret instances", err)
			os.Exit(1)
		}

		// write the output
		if len(*v0secretInstances) == 0 {
			cli.Info(fmt.Sprintf(
				"No secret instances currently managed by %s threeport control plane",
				requestedControlPlane,
			))
			os.Exit(0)
		}
		if err := outputGetSecretsCmd(
			v0secretInstances,
			apiClient,
			apiEndpoint,
		); err != nil {
			cli.Error("failed to produce output: %s", err)
			os.Exit(0)
		}
	},
	Short:        "Get secrets from the system",
	SilenceUsage: true,
	Use:          "secrets",
}

func init() {
	GetCmd.AddCommand(GetSecretsCmd)

	GetSecretsCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
}

var (
	createSecretConfigPath string
	createSecretVersion    string
)

// CreateSecretCmd represents the secret command
var CreateSecretCmd = &cobra.Command{
	Example: "  tptctl create secret --config path/to/config.yaml",
	Long:    "Create a new secret. This command creates a new secret definition and secret instance based on the secret config.",
	PreRun:  CommandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, _ := GetClientContext(cmd)

		// read secret config
		configContent, err := os.ReadFile(createSecretConfigPath)
		if err != nil {
			cli.Error("failed to read config file", err)
			os.Exit(1)
		}

		// create secret based on version
		switch createSecretVersion {
		case "v0":
			var secretConfig config_v0.SecretConfig
			if err := yaml.UnmarshalStrict(configContent, &secretConfig); err != nil {
				cli.Error("failed to unmarshal config file yaml content", err)
				os.Exit(1)
			}

			// create secret
			secret := secretConfig.Secret
			secret.SecretConfigPath = createSecretConfigPath
			createdSecretDefinition, createdSecretInstance, err := secret.Create(
				apiClient,
				apiEndpoint,
			)
			if err != nil {
				cli.Error("failed to create secret", err)
				os.Exit(1)
			}

			cli.Info(fmt.Sprintf("secret definition %s created", *createdSecretDefinition.Name))
			cli.Info(fmt.Sprintf("secret instance %s created", *createdSecretInstance.Name))
			cli.Complete(fmt.Sprintf("secret %s created", secretConfig.Secret.Name))
		default:
			cli.Error("", errors.New("unrecognized object version"))
			os.Exit(1)
		}
	},
	Short:        "Create a new secret",
	SilenceUsage: true,
	Use:          "secret",
}

func init() {
	CreateCmd.AddCommand(CreateSecretCmd)

	CreateSecretCmd.Flags().StringVarP(
		&createSecretConfigPath,
		"config", "c", "", "Path to file with secret config.",
	)
	CreateSecretCmd.MarkFlagRequired("config")
	CreateSecretCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
	CreateSecretCmd.Flags().StringVarP(
		&createSecretVersion,
		"version", "v", "v0", "Version of secrets object to create. One of: [v0]",
	)
}

var (
	deleteSecretConfigPath string
	deleteSecretName       string
	deleteSecretVersion    string
)

// DeleteSecretCmd represents the secret command
var DeleteSecretCmd = &cobra.Command{
	Example: "  # delete based on config file\n  tptctl delete secret --config path/to/config.yaml\n\n  # delete based on name\n  tptctl delete secret --name some-secret",
	Long:    "Delete an existing secret. This command deletes an existing secret definition and secret instance based on the secret config.",
	PreRun:  CommandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, _ := GetClientContext(cmd)

		// flag validation
		if deleteSecretConfigPath == "" {
			cli.Error("flag validation failed", errors.New("config file path is required"))
		}

		// read secret config
		configContent, err := os.ReadFile(deleteSecretConfigPath)
		if err != nil {
			cli.Error("failed to read config file", err)
			os.Exit(1)
		}

		// delete secret based on version
		switch deleteSecretVersion {
		case "v0":
			var secretConfig config_v0.SecretConfig
			if err := yaml.UnmarshalStrict(configContent, &secretConfig); err != nil {
				cli.Error("failed to unmarshal config file yaml content", err)
				os.Exit(1)
			}

			// delete secret
			secret := secretConfig.Secret
			secret.SecretConfigPath = deleteSecretConfigPath
			_, _, err = secret.Delete(apiClient, apiEndpoint)
			if err != nil {
				cli.Error("failed to delete secret", err)
				os.Exit(1)
			}

			cli.Info(fmt.Sprintf("secret definition %s deleted", secret.Name))
			cli.Info(fmt.Sprintf("secret instance %s deleted", secret.Name))
			cli.Complete(fmt.Sprintf("secret %s deleted", secretConfig.Secret.Name))
		default:
			cli.Error("", errors.New("unrecognized object version"))
			os.Exit(1)
		}
	},
	Short:        "Delete an existing secret",
	SilenceUsage: true,
	Use:          "secret",
}

func init() {
	DeleteCmd.AddCommand(DeleteSecretCmd)

	DeleteSecretCmd.Flags().StringVarP(
		&deleteSecretConfigPath,
		"config", "c", "", "Path to file with secret config.",
	)
	DeleteSecretCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
	DeleteSecretCmd.Flags().StringVarP(
		&deleteSecretVersion,
		"version", "v", "v0", "Version of secrets object to delete. One of: [v0]",
	)
}

///////////////////////////////////////////////////////////////////////////////
// SecretInstance
///////////////////////////////////////////////////////////////////////////////

var getSecretInstanceVersion string

// GetSecretInstancesCmd represents the secret-instance command
var GetSecretInstancesCmd = &cobra.Command{
	Example: "  tptctl get secret-instances",
	Long:    "Get secret instances from the system.",
	PreRun:  CommandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, requestedControlPlane := GetClientContext(cmd)

		switch getSecretInstanceVersion {
		case "v0":
			// get secret instances
			secretInstances, err := client_v0.GetSecretInstances(apiClient, apiEndpoint)
			if err != nil {
				cli.Error("failed to retrieve secret instances", err)
				os.Exit(1)
			}

			// write the output
			if len(*secretInstances) == 0 {
				cli.Info(fmt.Sprintf(
					"No secret instances currently managed by %s threeport control plane",
					requestedControlPlane,
				))
				os.Exit(0)
			}
			if err := outputGetv0SecretInstancesCmd(
				secretInstances,
				apiClient,
				apiEndpoint,
			); err != nil {
				cli.Error("failed to produce output", err)
				os.Exit(0)
			}
		default:
			cli.Error("", errors.New("unrecognized object version"))
			os.Exit(1)
		}
	},
	Short:        "Get secret instances from the system",
	SilenceUsage: true,
	Use:          "secret-instances",
}

func init() {
	GetCmd.AddCommand(GetSecretInstancesCmd)

	GetSecretInstancesCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
	GetSecretInstancesCmd.Flags().StringVarP(
		&getSecretInstanceVersion,
		"version", "v", "v0", "Version of secret instances object to retrieve. One of: [v0]",
	)
}

var (
	createSecretInstanceConfigPath string
	createSecretInstanceVersion    string
)

// CreateSecretInstanceCmd represents the secret-instance command
var CreateSecretInstanceCmd = &cobra.Command{
	Example: "  tptctl create secret-instance --config path/to/config.yaml",
	Long:    "Create a new secret instance.",
	PreRun:  CommandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, _ := GetClientContext(cmd)

		// read secret instance config
		configContent, err := os.ReadFile(createSecretInstanceConfigPath)
		if err != nil {
			cli.Error("failed to read config file", err)
			os.Exit(1)
		}
		// create secret instance based on version
		switch createSecretInstanceVersion {
		case "v0":
			var secretInstanceConfig config_v0.SecretInstanceConfig
			if err := yaml.UnmarshalStrict(configContent, &secretInstanceConfig); err != nil {
				cli.Error("failed to unmarshal config file yaml content", err)
				os.Exit(1)
			}

			// create secret instance
			secretInstance := secretInstanceConfig.SecretInstance
			secretInstance.SecretConfigPath = createSecretInstanceConfigPath
			createdSecretInstance, err := secretInstance.Create(apiClient, apiEndpoint)
			if err != nil {
				cli.Error("failed to create secret instance", err)
				os.Exit(1)
			}

			cli.Complete(fmt.Sprintf("secret instance %s created", *createdSecretInstance.Name))
		default:
			cli.Error("", errors.New("unrecognized object version"))
			os.Exit(1)
		}
	},
	Short:        "Create a new secret instance",
	SilenceUsage: true,
	Use:          "secret-instance",
}

func init() {
	CreateCmd.AddCommand(CreateSecretInstanceCmd)

	CreateSecretInstanceCmd.Flags().StringVarP(
		&createSecretInstanceConfigPath,
		"config", "c", "", "Path to file with secret instance config.",
	)
	CreateSecretInstanceCmd.MarkFlagRequired("config")
	CreateSecretInstanceCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
	CreateSecretInstanceCmd.Flags().StringVarP(
		&createSecretInstanceVersion,
		"version", "v", "v0", "Version of secret instances object to create. One of: [v0]",
	)
}

var (
	deleteSecretInstanceConfigPath string
	deleteSecretInstanceName       string
	deleteSecretInstanceVersion    string
)

// DeleteSecretInstanceCmd represents the secret-instance command
var DeleteSecretInstanceCmd = &cobra.Command{
	Example: "  # delete based on config file\n  tptctl delete secret-instance --config path/to/config.yaml\n\n  # delete based on name\n  tptctl delete secret-instance --name some-secret-instance",
	Long:    "Delete an existing secret instance.",
	PreRun:  CommandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, _ := GetClientContext(cmd)

		// flag validation
		if err := cli.ValidateConfigNameFlags(
			deleteSecretInstanceConfigPath,
			deleteSecretInstanceName,
			"secret instance",
		); err != nil {
			cli.Error("flag validation failed", err)
			os.Exit(1)
		}

		// delete secret instance based on version
		switch deleteSecretInstanceVersion {
		case "v0":
			var secretInstanceConfig config_v0.SecretInstanceConfig
			if deleteSecretInstanceConfigPath != "" {
				// load secret instance config
				configContent, err := os.ReadFile(deleteSecretInstanceConfigPath)
				if err != nil {
					cli.Error("failed to read config file", err)
					os.Exit(1)
				}
				if err := yaml.UnmarshalStrict(configContent, &secretInstanceConfig); err != nil {
					cli.Error("failed to unmarshal config file yaml content", err)
					os.Exit(1)
				}
			} else {
				secretInstanceConfig = config_v0.SecretInstanceConfig{
					SecretInstance: config_v0.SecretInstanceValues{
						Name: deleteSecretInstanceName,
					},
				}
			}

			// delete secret instance
			secretInstance := secretInstanceConfig.SecretInstance
			secretInstance.SecretConfigPath = deleteSecretInstanceConfigPath
			deletedSecretInstance, err := secretInstance.Delete(apiClient, apiEndpoint)
			if err != nil {
				cli.Error("failed to delete secret instance", err)
				os.Exit(1)
			}

			cli.Complete(fmt.Sprintf("secret instance %s deleted", *deletedSecretInstance.Name))
		default:
			cli.Error("", errors.New("unrecognized object version"))
			os.Exit(1)
		}
	},
	Short:        "Delete an existing secret instance",
	SilenceUsage: true,
	Use:          "secret-instance",
}

func init() {
	DeleteCmd.AddCommand(DeleteSecretInstanceCmd)

	DeleteSecretInstanceCmd.Flags().StringVarP(
		&deleteSecretInstanceConfigPath,
		"config", "c", "", "Path to file with secret instance config.",
	)
	DeleteSecretInstanceCmd.Flags().StringVarP(
		&deleteSecretInstanceName,
		"name", "n", "", "Name of secret instance.",
	)
	DeleteSecretInstanceCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
	DeleteSecretInstanceCmd.Flags().StringVarP(
		&deleteSecretInstanceVersion,
		"version", "v", "v0", "Version of secret instances object to delete. One of: [v0]",
	)
}

var (
	describeSecretInstanceConfigPath string
	describeSecretInstanceName       string
	describeSecretInstanceField      string
	describeSecretInstanceOutput     string
	describeSecretInstanceVersion    string
)

// DescribeSecretInstanceCmd representes the secret-instance command
var DescribeSecretInstanceCmd = &cobra.Command{
	Example: "  # Get the plain output description for a secret instance\n  tptctl describe secret-instance -n some-secret-instance\n\n  # Get JSON output for a secret instance\n  tptctl describe secret-instance -n some-secret-instance -o json\n\n  # Get the value of the Name field for a secret instance\n  tptctl describe secret-instance -n some-secret-instance -f Name ",
	Long:    "Describe a secret instance.  This command can give you a plain output description, output all fields in JSON or YAML format, or provide the value of any specific field.\n\nNote: any values that are encrypted in the database will be redacted unless the field is specifically requested with the --field flag.",
	PreRun:  CommandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, _ := GetClientContext(cmd)

		// flag validation
		if err := cli.ValidateConfigNameFlags(
			describeSecretInstanceConfigPath,
			describeSecretInstanceName,
			"secret instance",
		); err != nil {
			cli.Error("flag validation failed", err)
			os.Exit(1)
		}

		if err := cli.ValidateDescribeOutputFlag(
			describeSecretInstanceOutput,
			"secret instance",
		); err != nil {
			cli.Error("flag validation failed", err)
			os.Exit(1)
		}

		// get secret instance
		var secretInstance interface{}
		switch describeSecretInstanceVersion {
		case "v0":
			// load secret instance config by name or config file
			var secretInstanceConfig config_v0.SecretInstanceConfig
			if describeSecretInstanceConfigPath != "" {
				configContent, err := os.ReadFile(describeSecretInstanceConfigPath)
				if err != nil {
					cli.Error("failed to read config file", err)
					os.Exit(1)
				}
				if err := yaml.UnmarshalStrict(configContent, &secretInstanceConfig); err != nil {
					cli.Error("failed to unmarshal config file yaml content", err)
					os.Exit(1)
				}
			} else {
				secretInstanceConfig = config_v0.SecretInstanceConfig{
					SecretInstance: config_v0.SecretInstanceValues{
						Name: describeSecretInstanceName,
					},
				}
			}

			// get secret instance object by name
			obj, err := client_v0.GetSecretInstanceByName(
				apiClient,
				apiEndpoint,
				secretInstanceConfig.SecretInstance.Name,
			)
			if err != nil {
				cli.Error("failed to retrieve secret instance details", err)
				os.Exit(1)
			}
			secretInstance = obj

			// return plain output if requested
			if describeSecretInstanceOutput == "plain" {
				if err := outputDescribev0SecretInstanceCmd(
					secretInstance.(*api_v0.SecretInstance),
					&secretInstanceConfig,
					apiClient,
					apiEndpoint,
				); err != nil {
					cli.Error("failed to describe secret instance", err)
					os.Exit(1)
				}
			}
		default:
			cli.Error("", errors.New("unrecognized object version"))
			os.Exit(1)
		}

		// return field value if specified
		if describeSecretInstanceField != "" {
			fieldVal, err := util.GetObjectFieldValue(
				secretInstance,
				describeSecretInstanceField,
			)
			if err != nil {
				cli.Error("failed to get field value from secret instance", err)
				os.Exit(1)
			}

			// decrypt value as needed
			encrypted, err := encryption.IsEncryptedField(secretInstance, describeSecretInstanceField)
			if err != nil {
				cli.Error("", err)
			}
			if encrypted {
				// get encryption key from threeport config
				threeportConfig, requestedControlPlane, err := config_v0.GetThreeportConfig(cliArgs.ControlPlaneName)
				if err != nil {
					cli.Error("failed to get threeport config: %w", err)
					os.Exit(1)
				}
				encryptionKey, err := threeportConfig.GetThreeportEncryptionKey(requestedControlPlane)
				if err != nil {
					cli.Error("failed to get encryption key from threeport config: %w", err)
					os.Exit(1)
				}

				// decrypt value for output
				decryptedVal, err := encryption.Decrypt(encryptionKey, fieldVal.String())
				if err != nil {
					cli.Error("failed to decrypt value: %w", err)
				}
				fmt.Println(decryptedVal)
				os.Exit(0)
			} else {
				fmt.Println(fieldVal.Interface())
				os.Exit(0)
			}
		}

		// produce json or yaml output if requested
		switch describeSecretInstanceOutput {
		case "json":
			// redact encrypted values
			redactedSecretInstance := encryption.RedactEncryptedValues(secretInstance)

			// marshal to JSON then print
			secretInstanceJson, err := json.MarshalIndent(redactedSecretInstance, "", "  ")
			if err != nil {
				cli.Error("failed to marshal secret instance into JSON", err)
				os.Exit(1)
			}

			fmt.Println(string(secretInstanceJson))
		case "yaml":
			// redact encrypted values
			redactedSecretInstance := encryption.RedactEncryptedValues(secretInstance)

			// marshal to JSON then convert to YAML - this results in field
			// names with correct capitalization vs marshalling directly to YAML
			secretInstanceJson, err := json.MarshalIndent(redactedSecretInstance, "", "  ")
			if err != nil {
				cli.Error("failed to marshal secret instance into JSON", err)
				os.Exit(1)
			}
			secretInstanceYaml, err := ghodss_yaml.JSONToYAML(secretInstanceJson)
			if err != nil {
				cli.Error("failed to convert secret instance JSON to YAML", err)
				os.Exit(1)
			}

			fmt.Println(string(secretInstanceYaml))
		}
	},
	Short:        "Describe a secret instance",
	SilenceUsage: true,
	Use:          "secret-instance",
}

func init() {
	DescribeCmd.AddCommand(DescribeSecretInstanceCmd)

	DescribeSecretInstanceCmd.Flags().StringVarP(
		&describeSecretInstanceConfigPath,
		"config", "c", "", "Path to file with secret instance config.",
	)
	DescribeSecretInstanceCmd.Flags().StringVarP(
		&describeSecretInstanceName,
		"name", "n", "", "Name of secret instance.",
	)
	DescribeSecretInstanceCmd.Flags().StringVarP(
		&describeSecretInstanceOutput,
		"output", "o", "plain", "Output format for object description. One of 'plain','json','yaml'.  Will be ignored if the --field flag is also used.  Plain output produces select details about the object.  JSON and YAML output formats include all direct attributes of the object",
	)
	DescribeSecretInstanceCmd.Flags().StringVarP(
		&describeSecretInstanceField,
		"field", "f", "", "Object field to get value for. If used, --output flag will be ignored.  *Only* the value of the desired field will be returned.  Will not return information on related objects, only direct attributes of the object itself.",
	)
	DescribeSecretInstanceCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
	DescribeSecretInstanceCmd.Flags().StringVarP(
		&describeSecretInstanceVersion,
		"version", "v", "v0", "Version of secret instances object to describe. One of: [v0]",
	)
}
