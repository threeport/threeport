// generated by 'threeport-sdk codegen api-model' - do not edit

package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	ghodss_yaml "github.com/ghodss/yaml"
	cobra "github.com/spf13/cobra"
	cli "github.com/threeport/threeport/pkg/cli/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	config "github.com/threeport/threeport/pkg/config/v0"
	encryption "github.com/threeport/threeport/pkg/encryption/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
	yaml "gopkg.in/yaml.v2"
	"os"
)

///////////////////////////////////////////////////////////////////////////////
// Workload
///////////////////////////////////////////////////////////////////////////////

// GetWorkloadsCmd represents the workload command
var GetWorkloadsCmd = &cobra.Command{
	Example: "  tptctl get workloads",
	Long:    "Get workloads from the system.\\n\\nA workload is a simple abstraction of workload definitions and workload instances.\\nThis command displays all instances and the definitions used to configure them.",
	PreRun:  commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, requestedControlPlane := getClientContext(cmd)

		// get workloads
		workloadInstances, err := client.GetWorkloadInstances(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to retrieve workload instances", err)
			os.Exit(1)
		}

		// write the output
		if len(*workloadInstances) == 0 {
			cli.Info(fmt.Sprintf(
				"No workload instances currently managed by %s threeport control plane",
				requestedControlPlane,
			))
			os.Exit(0)
		}
		if err := outputGetWorkloadsCmd(
			workloadInstances,
			apiClient,
			apiEndpoint,
		); err != nil {
			cli.Error("failed to produce output: %s", err)
			os.Exit(0)
		}
	},
	Short:        "Get workloads from the system",
	SilenceUsage: true,
	Use:          "workloads",
}

func init() {
	GetCmd.AddCommand(GetWorkloadsCmd)

	GetWorkloadsCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
}

var createWorkloadConfigPath string

// CreateWorkloadCmd represents the workload command
var CreateWorkloadCmd = &cobra.Command{
	Example: "  tptctl create workload --config path/to/config.yaml",
	Long:    "Create a new workload. This command creates a new workload definition and workload instance based on the workload config.",
	PreRun:  commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, _ := getClientContext(cmd)

		// load workload config
		configContent, err := os.ReadFile(createWorkloadConfigPath)
		if err != nil {
			cli.Error("failed to read config file", err)
			os.Exit(1)
		}
		var workloadConfig config.WorkloadConfig
		if err := yaml.UnmarshalStrict(configContent, &workloadConfig); err != nil {
			cli.Error("failed to unmarshal config file yaml content", err)
			os.Exit(1)
		}

		// create workload
		workload := workloadConfig.Workload
		workload.WorkloadConfigPath = createWorkloadConfigPath
		createdWorkloadDefinition, createdWorkloadInstance, err := workload.Create(
			apiClient,
			apiEndpoint,
		)
		if err != nil {
			cli.Error("failed to create workload", err)
			os.Exit(1)
		}

		cli.Info(fmt.Sprintf("workload definition %s created", *createdWorkloadDefinition.Name))
		cli.Info(fmt.Sprintf("workload instance %s created", *createdWorkloadInstance.Name))
		cli.Complete(fmt.Sprintf("workload %s created", workloadConfig.Workload.Name))
	},
	Short:        "Create a new workload",
	SilenceUsage: true,
	Use:          "workload",
}

func init() {
	CreateCmd.AddCommand(CreateWorkloadCmd)

	CreateWorkloadCmd.Flags().StringVarP(
		&createWorkloadConfigPath,
		"config", "c", "", "Path to file with workload config.",
	)
	CreateWorkloadCmd.MarkFlagRequired("config")
	CreateWorkloadCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
}

var (
	deleteWorkloadConfigPath string
	deleteWorkloadName       string
)

// DeleteWorkloadCmd represents the workload command
var DeleteWorkloadCmd = &cobra.Command{
	Example: "  # delete based on config file\n  tptctl delete workload --config path/to/config.yaml\n\n  # delete based on name\n  tptctl delete workload --name some-workload",
	Long:    "Delete an existing workload. This command deletes an existing workload definition and workload instance based on the workload config.",
	PreRun:  commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, _ := getClientContext(cmd)

		// flag validation
		if deleteWorkloadConfigPath == "" {
			cli.Error("flag validation failed", errors.New("config file path is required"))
		}

		var workloadConfig config.WorkloadConfig
		// load workload config
		configContent, err := os.ReadFile(deleteWorkloadConfigPath)
		if err != nil {
			cli.Error("failed to read config file", err)
			os.Exit(1)
		}
		if err := yaml.UnmarshalStrict(configContent, &workloadConfig); err != nil {
			cli.Error("failed to unmarshal config file yaml content", err)
			os.Exit(1)
		}

		// delete workload
		workload := workloadConfig.Workload
		workload.WorkloadConfigPath = deleteWorkloadConfigPath
		_, _, err = workload.Delete(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to delete workload", err)
			os.Exit(1)
		}

		cli.Info(fmt.Sprintf("workload definition %s deleted", workload.Name))
		cli.Info(fmt.Sprintf("workload instance %s deleted", workload.Name))
		cli.Complete(fmt.Sprintf("workload %s deleted", workloadConfig.Workload.Name))
	},
	Short:        "Delete an existing workload",
	SilenceUsage: true,
	Use:          "workload",
}

func init() {
	DeleteCmd.AddCommand(DeleteWorkloadCmd)

	DeleteWorkloadCmd.Flags().StringVarP(
		&deleteWorkloadConfigPath,
		"config", "c", "", "Path to file with workload config.",
	)
	DeleteWorkloadCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
}

///////////////////////////////////////////////////////////////////////////////
// WorkloadDefinition
///////////////////////////////////////////////////////////////////////////////

// GetWorkloadDefinitionsCmd represents the workload-definition command
var GetWorkloadDefinitionsCmd = &cobra.Command{
	Example: "  tptctl get workload-definitions",
	Long:    "Get workload definitions from the system.",
	PreRun:  commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, requestedControlPlane := getClientContext(cmd)

		// get workload definitions
		workloadDefinitions, err := client.GetWorkloadDefinitions(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to retrieve workload definitions", err)
			os.Exit(1)
		}

		// write the output
		if len(*workloadDefinitions) == 0 {
			cli.Info(fmt.Sprintf(
				"No workload definitions currently managed by %s threeport control plane",
				requestedControlPlane,
			))
			os.Exit(0)
		}
		if err := outputGetWorkloadDefinitionsCmd(
			workloadDefinitions,
			apiClient,
			apiEndpoint,
		); err != nil {
			cli.Error("failed to produce output", err)
			os.Exit(0)
		}
	},
	Short:        "Get workload definitions from the system",
	SilenceUsage: true,
	Use:          "workload-definitions",
}

func init() {
	GetCmd.AddCommand(GetWorkloadDefinitionsCmd)

	GetWorkloadDefinitionsCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
}

var createWorkloadDefinitionConfigPath string

// CreateWorkloadDefinitionCmd represents the workload-definition command
var CreateWorkloadDefinitionCmd = &cobra.Command{
	Example: "  tptctl create workload-definition --config path/to/config.yaml",
	Long:    "Create a new workload definition.",
	PreRun:  commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, _ := getClientContext(cmd)

		// load workload definition config
		configContent, err := os.ReadFile(createWorkloadDefinitionConfigPath)
		if err != nil {
			cli.Error("failed to read config file", err)
			os.Exit(1)
		}
		var workloadDefinitionConfig config.WorkloadDefinitionConfig
		if err := yaml.UnmarshalStrict(configContent, &workloadDefinitionConfig); err != nil {
			cli.Error("failed to unmarshal config file yaml content", err)
			os.Exit(1)
		}

		// create workload definition
		workloadDefinition := workloadDefinitionConfig.WorkloadDefinition
		workloadDefinition.WorkloadConfigPath = createWorkloadDefinitionConfigPath
		createdWorkloadDefinition, err := workloadDefinition.Create(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to create workload definition", err)
			os.Exit(1)
		}

		cli.Complete(fmt.Sprintf("workload definition %s created", *createdWorkloadDefinition.Name))
	},
	Short:        "Create a new workload definition",
	SilenceUsage: true,
	Use:          "workload-definition",
}

func init() {
	CreateCmd.AddCommand(CreateWorkloadDefinitionCmd)

	CreateWorkloadDefinitionCmd.Flags().StringVarP(
		&createWorkloadDefinitionConfigPath,
		"config", "c", "", "Path to file with workload definition config.",
	)
	CreateWorkloadDefinitionCmd.MarkFlagRequired("config")
	CreateWorkloadDefinitionCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
}

var (
	deleteWorkloadDefinitionConfigPath string
	deleteWorkloadDefinitionName       string
)

// DeleteWorkloadDefinitionCmd represents the workload-definition command
var DeleteWorkloadDefinitionCmd = &cobra.Command{
	Example: "  # delete based on config file\n  tptctl delete workload-definition --config path/to/config.yaml\n\n  # delete based on name\n  tptctl delete workload-definition --name some-workload-definition",
	Long:    "Delete an existing workload definition.",
	PreRun:  commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, _ := getClientContext(cmd)

		// flag validation
		if err := cli.ValidateConfigNameFlags(
			deleteWorkloadDefinitionConfigPath,
			deleteWorkloadDefinitionName,
			"workload definition",
		); err != nil {
			cli.Error("flag validation failed", err)
			os.Exit(1)
		}

		var workloadDefinitionConfig config.WorkloadDefinitionConfig
		if deleteWorkloadDefinitionConfigPath != "" {
			// load workload definition config
			configContent, err := os.ReadFile(deleteWorkloadDefinitionConfigPath)
			if err != nil {
				cli.Error("failed to read config file", err)
				os.Exit(1)
			}
			if err := yaml.UnmarshalStrict(configContent, &workloadDefinitionConfig); err != nil {
				cli.Error("failed to unmarshal config file yaml content", err)
				os.Exit(1)
			}
		} else {
			workloadDefinitionConfig = config.WorkloadDefinitionConfig{
				WorkloadDefinition: config.WorkloadDefinitionValues{
					Name: deleteWorkloadDefinitionName,
				},
			}
		}

		// delete workload definition
		workloadDefinition := workloadDefinitionConfig.WorkloadDefinition
		workloadDefinition.WorkloadConfigPath = deleteWorkloadDefinitionConfigPath
		deletedWorkloadDefinition, err := workloadDefinition.Delete(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to delete workload definition", err)
			os.Exit(1)
		}

		cli.Complete(fmt.Sprintf("workload definition %s deleted", *deletedWorkloadDefinition.Name))
	},
	Short:        "Delete an existing workload definition",
	SilenceUsage: true,
	Use:          "workload-definition",
}

func init() {
	DeleteCmd.AddCommand(DeleteWorkloadDefinitionCmd)

	DeleteWorkloadDefinitionCmd.Flags().StringVarP(
		&deleteWorkloadDefinitionConfigPath,
		"config", "c", "", "Path to file with workload definition config.",
	)
	DeleteWorkloadDefinitionCmd.Flags().StringVarP(
		&deleteWorkloadDefinitionName,
		"name", "n", "", "Name of workload definition.",
	)
	DeleteWorkloadDefinitionCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
}

var (
	describeWorkloadDefinitionConfigPath string
	describeWorkloadDefinitionName       string
	describeWorkloadDefinitionField      string
	describeWorkloadDefinitionOutput     string
)

// DescribeWorkloadDefinitionCmd representes the workload-definition command
var DescribeWorkloadDefinitionCmd = &cobra.Command{
	Example: "  # Get the plain output description for a workload definition\n  tptctl describe workload-definition -n some-workload-definition\n\n  # Get JSON output for a workload definition\n  tptctl describe workload-definition -n some-workload-definition -o json\n\n  # Get the value of the Name field for a workload definition\n  tptctl describe workload-definition -n some-workload-definition -f Name ",
	Long:    "Describe a workload definition.  This command can give you a plain output description, output all fields in JSON or YAML format, or provide the value of any specific field.\n\nNote: any values that are encrypted in the database will be redacted unless the field is specifically requested with the --field flag.",
	PreRun:  commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, _ := getClientContext(cmd)

		// flag validation
		if err := cli.ValidateConfigNameFlags(
			describeWorkloadDefinitionConfigPath,
			describeWorkloadDefinitionName,
			"workload definition",
		); err != nil {
			cli.Error("flag validation failed", err)
			os.Exit(1)
		}

		if err := cli.ValidateDescribeOutputFlag(
			describeWorkloadDefinitionOutput,
			"workload definition",
		); err != nil {
			cli.Error("flag validation failed", err)
			os.Exit(1)
		}

		// load workload definition config by name or config file
		var workloadDefinitionConfig config.WorkloadDefinitionConfig
		if describeWorkloadDefinitionConfigPath != "" {
			configContent, err := os.ReadFile(describeWorkloadDefinitionConfigPath)
			if err != nil {
				cli.Error("failed to read config file", err)
				os.Exit(1)
			}
			if err := yaml.UnmarshalStrict(configContent, &workloadDefinitionConfig); err != nil {
				cli.Error("failed to unmarshal config file yaml content", err)
				os.Exit(1)
			}
		} else {
			workloadDefinitionConfig = config.WorkloadDefinitionConfig{
				WorkloadDefinition: config.WorkloadDefinitionValues{
					Name: describeWorkloadDefinitionName,
				},
			}
		}

		// get workload definition
		workloadDefinition, err := client.GetWorkloadDefinitionByName(
			apiClient,
			apiEndpoint,
			workloadDefinitionConfig.WorkloadDefinition.Name,
		)
		if err != nil {
			cli.Error("failed to retrieve workload definition details", err)
			os.Exit(1)
		}

		// return field value if specified
		if describeWorkloadDefinitionField != "" {
			fieldVal, err := util.GetObjectFieldValue(
				workloadDefinition,
				describeWorkloadDefinitionField,
			)
			if err != nil {
				cli.Error("failed to get field value from workload definition", err)
				os.Exit(1)
			}

			// decrypt value as needed
			encrypted, err := encryption.IsEncryptedField(workloadDefinition, describeWorkloadDefinitionField)
			if err != nil {
				cli.Error("", err)
			}
			if encrypted {
				// get encryption key from threeport config
				threeportConfig, requestedControlPlane, err := config.GetThreeportConfig(cliArgs.ControlPlaneName)
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

		switch describeWorkloadDefinitionOutput {
		case "plain":
			// produce plain object description output
			if err := outputDescribeWorkloadDefinitionCmd(
				workloadDefinition,
				&workloadDefinitionConfig,
				apiClient,
				apiEndpoint,
			); err != nil {
				cli.Error("failed to describe workload definition", err)
				os.Exit(1)
			}
		case "json":
			// redact encrypted values
			redactedWorkloadDefinition := encryption.RedactEncryptedValues(workloadDefinition)

			// marshal to JSON then print
			workloadDefinitionJson, err := json.MarshalIndent(redactedWorkloadDefinition, "", "  ")
			if err != nil {
				cli.Error("failed to marshal workload definition into JSON", err)
				os.Exit(1)
			}

			fmt.Println(string(workloadDefinitionJson))
		case "yaml":
			// redact encrypted values
			redactedWorkloadDefinition := encryption.RedactEncryptedValues(workloadDefinition)

			// marshal to JSON then convert to YAML - this results in field
			// names with correct capitalization vs marshalling directly to YAML
			workloadDefinitionJson, err := json.MarshalIndent(redactedWorkloadDefinition, "", "  ")
			if err != nil {
				cli.Error("failed to marshal workload definition into JSON", err)
				os.Exit(1)
			}
			workloadDefinitionYaml, err := ghodss_yaml.JSONToYAML(workloadDefinitionJson)
			if err != nil {
				cli.Error("failed to convert workload definition JSON to YAML", err)
				os.Exit(1)
			}

			fmt.Println(string(workloadDefinitionYaml))
		}
	},
	Short:        "Describe a workload definition",
	SilenceUsage: true,
	Use:          "workload-definition",
}

func init() {
	DescribeCmd.AddCommand(DescribeWorkloadDefinitionCmd)

	DescribeWorkloadDefinitionCmd.Flags().StringVarP(
		&describeWorkloadDefinitionConfigPath,
		"config", "c", "", "Path to file with workload definition config.")
	DescribeWorkloadDefinitionCmd.Flags().StringVarP(
		&describeWorkloadDefinitionName,
		"name", "n", "", "Name of workload definition.")
	DescribeWorkloadDefinitionCmd.Flags().StringVarP(
		&describeWorkloadDefinitionOutput,
		"output", "o", "plain", "Output format for object description. One of 'plain','json','yaml'.  Will be ignored if the --field flag is also used.  Plain output produces select details about the object.  JSON and YAML output formats include all direct attributes of the object")
	DescribeWorkloadDefinitionCmd.Flags().StringVarP(
		&describeWorkloadDefinitionField,
		"field", "f", "", "Object field to get value for. If used, --output flag will be ignored.  *Only* the value of the desired field will be returned.  Will not return information on related objects, only direct attributes of the object itself.")
	DescribeWorkloadDefinitionCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
}

///////////////////////////////////////////////////////////////////////////////
// WorkloadInstance
///////////////////////////////////////////////////////////////////////////////

// GetWorkloadInstancesCmd represents the workload-instance command
var GetWorkloadInstancesCmd = &cobra.Command{
	Example: "  tptctl get workload-instances",
	Long:    "Get workload instances from the system.",
	PreRun:  commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, requestedControlPlane := getClientContext(cmd)

		// get workload instances
		workloadInstances, err := client.GetWorkloadInstances(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to retrieve workload instances", err)
			os.Exit(1)
		}

		// write the output
		if len(*workloadInstances) == 0 {
			cli.Info(fmt.Sprintf(
				"No workload instances currently managed by %s threeport control plane",
				requestedControlPlane,
			))
			os.Exit(0)
		}
		if err := outputGetWorkloadInstancesCmd(
			workloadInstances,
			apiClient,
			apiEndpoint,
		); err != nil {
			cli.Error("failed to produce output", err)
			os.Exit(0)
		}
	},
	Short:        "Get workload instances from the system",
	SilenceUsage: true,
	Use:          "workload-instances",
}

func init() {
	GetCmd.AddCommand(GetWorkloadInstancesCmd)

	GetWorkloadInstancesCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
}

var createWorkloadInstanceConfigPath string

// CreateWorkloadInstanceCmd represents the workload-instance command
var CreateWorkloadInstanceCmd = &cobra.Command{
	Example: "  tptctl create workload-instance --config path/to/config.yaml",
	Long:    "Create a new workload instance.",
	PreRun:  commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, _ := getClientContext(cmd)

		// load workload instance config
		configContent, err := os.ReadFile(createWorkloadInstanceConfigPath)
		if err != nil {
			cli.Error("failed to read config file", err)
			os.Exit(1)
		}
		var workloadInstanceConfig config.WorkloadInstanceConfig
		if err := yaml.UnmarshalStrict(configContent, &workloadInstanceConfig); err != nil {
			cli.Error("failed to unmarshal config file yaml content", err)
			os.Exit(1)
		}

		// create workload instance
		workloadInstance := workloadInstanceConfig.WorkloadInstance
		createdWorkloadInstance, err := workloadInstance.Create(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to create workload instance", err)
			os.Exit(1)
		}

		cli.Complete(fmt.Sprintf("workload instance %s created", *createdWorkloadInstance.Name))
	},
	Short:        "Create a new workload instance",
	SilenceUsage: true,
	Use:          "workload-instance",
}

func init() {
	CreateCmd.AddCommand(CreateWorkloadInstanceCmd)

	CreateWorkloadInstanceCmd.Flags().StringVarP(
		&createWorkloadInstanceConfigPath,
		"config", "c", "", "Path to file with workload instance config.",
	)
	CreateWorkloadInstanceCmd.MarkFlagRequired("config")
	CreateWorkloadInstanceCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
}

var (
	deleteWorkloadInstanceConfigPath string
	deleteWorkloadInstanceName       string
)

// DeleteWorkloadInstanceCmd represents the workload-instance command
var DeleteWorkloadInstanceCmd = &cobra.Command{
	Example: "  # delete based on config file\n  tptctl delete workload-instance --config path/to/config.yaml\n\n  # delete based on name\n  tptctl delete workload-instance --name some-workload-instance",
	Long:    "Delete an existing workload instance.",
	PreRun:  commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, _ := getClientContext(cmd)

		// flag validation
		if err := cli.ValidateConfigNameFlags(
			deleteWorkloadInstanceConfigPath,
			deleteWorkloadInstanceName,
			"workload instance",
		); err != nil {
			cli.Error("flag validation failed", err)
			os.Exit(1)
		}

		var workloadInstanceConfig config.WorkloadInstanceConfig
		if deleteWorkloadInstanceConfigPath != "" {
			// load workload instance config
			configContent, err := os.ReadFile(deleteWorkloadInstanceConfigPath)
			if err != nil {
				cli.Error("failed to read config file", err)
				os.Exit(1)
			}
			if err := yaml.UnmarshalStrict(configContent, &workloadInstanceConfig); err != nil {
				cli.Error("failed to unmarshal config file yaml content", err)
				os.Exit(1)
			}
		} else {
			workloadInstanceConfig = config.WorkloadInstanceConfig{
				WorkloadInstance: config.WorkloadInstanceValues{
					Name: deleteWorkloadInstanceName,
				},
			}
		}

		// delete workload instance
		workloadInstance := workloadInstanceConfig.WorkloadInstance
		deletedWorkloadInstance, err := workloadInstance.Delete(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to delete workload instance", err)
			os.Exit(1)
		}

		cli.Complete(fmt.Sprintf("workload instance %s deleted", *deletedWorkloadInstance.Name))
	},
	Short:        "Delete an existing workload instance",
	SilenceUsage: true,
	Use:          "workload-instance",
}

func init() {
	DeleteCmd.AddCommand(DeleteWorkloadInstanceCmd)

	DeleteWorkloadInstanceCmd.Flags().StringVarP(
		&deleteWorkloadInstanceConfigPath,
		"config", "c", "", "Path to file with workload instance config.",
	)
	DeleteWorkloadInstanceCmd.Flags().StringVarP(
		&deleteWorkloadInstanceName,
		"name", "n", "", "Name of workload instance.",
	)
	DeleteWorkloadInstanceCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
}

var (
	describeWorkloadInstanceConfigPath string
	describeWorkloadInstanceName       string
	describeWorkloadInstanceField      string
	describeWorkloadInstanceOutput     string
)

// DescribeWorkloadInstanceCmd representes the workload-instance command
var DescribeWorkloadInstanceCmd = &cobra.Command{
	Example: "  # Get the plain output description for a workload instance\n  tptctl describe workload-instance -n some-workload-instance\n\n  # Get JSON output for a workload instance\n  tptctl describe workload-instance -n some-workload-instance -o json\n\n  # Get the value of the Name field for a workload instance\n  tptctl describe workload-instance -n some-workload-instance -f Name ",
	Long:    "Describe a workload instance.  This command can give you a plain output description, output all fields in JSON or YAML format, or provide the value of any specific field.\n\nNote: any values that are encrypted in the database will be redacted unless the field is specifically requested with the --field flag.",
	PreRun:  commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, _ := getClientContext(cmd)

		// flag validation
		if err := cli.ValidateConfigNameFlags(
			describeWorkloadInstanceConfigPath,
			describeWorkloadInstanceName,
			"workload instance",
		); err != nil {
			cli.Error("flag validation failed", err)
			os.Exit(1)
		}

		if err := cli.ValidateDescribeOutputFlag(
			describeWorkloadInstanceOutput,
			"workload instance",
		); err != nil {
			cli.Error("flag validation failed", err)
			os.Exit(1)
		}

		// load workload instance config by name or config file
		var workloadInstanceConfig config.WorkloadInstanceConfig
		if describeWorkloadInstanceConfigPath != "" {
			configContent, err := os.ReadFile(describeWorkloadInstanceConfigPath)
			if err != nil {
				cli.Error("failed to read config file", err)
				os.Exit(1)
			}
			if err := yaml.UnmarshalStrict(configContent, &workloadInstanceConfig); err != nil {
				cli.Error("failed to unmarshal config file yaml content", err)
				os.Exit(1)
			}
		} else {
			workloadInstanceConfig = config.WorkloadInstanceConfig{
				WorkloadInstance: config.WorkloadInstanceValues{
					Name: describeWorkloadInstanceName,
				},
			}
		}

		// get workload instance
		workloadInstance, err := client.GetWorkloadInstanceByName(
			apiClient,
			apiEndpoint,
			workloadInstanceConfig.WorkloadInstance.Name,
		)
		if err != nil {
			cli.Error("failed to retrieve workload instance details", err)
			os.Exit(1)
		}

		// return field value if specified
		if describeWorkloadInstanceField != "" {
			fieldVal, err := util.GetObjectFieldValue(
				workloadInstance,
				describeWorkloadInstanceField,
			)
			if err != nil {
				cli.Error("failed to get field value from workload instance", err)
				os.Exit(1)
			}

			// decrypt value as needed
			encrypted, err := encryption.IsEncryptedField(workloadInstance, describeWorkloadInstanceField)
			if err != nil {
				cli.Error("", err)
			}
			if encrypted {
				// get encryption key from threeport config
				threeportConfig, requestedControlPlane, err := config.GetThreeportConfig(cliArgs.ControlPlaneName)
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

		switch describeWorkloadInstanceOutput {
		case "plain":
			// produce plain object description output
			if err := outputDescribeWorkloadInstanceCmd(
				workloadInstance,
				&workloadInstanceConfig,
				apiClient,
				apiEndpoint,
			); err != nil {
				cli.Error("failed to describe workload instance", err)
				os.Exit(1)
			}
		case "json":
			// redact encrypted values
			redactedWorkloadInstance := encryption.RedactEncryptedValues(workloadInstance)

			// marshal to JSON then print
			workloadInstanceJson, err := json.MarshalIndent(redactedWorkloadInstance, "", "  ")
			if err != nil {
				cli.Error("failed to marshal workload instance into JSON", err)
				os.Exit(1)
			}

			fmt.Println(string(workloadInstanceJson))
		case "yaml":
			// redact encrypted values
			redactedWorkloadInstance := encryption.RedactEncryptedValues(workloadInstance)

			// marshal to JSON then convert to YAML - this results in field
			// names with correct capitalization vs marshalling directly to YAML
			workloadInstanceJson, err := json.MarshalIndent(redactedWorkloadInstance, "", "  ")
			if err != nil {
				cli.Error("failed to marshal workload instance into JSON", err)
				os.Exit(1)
			}
			workloadInstanceYaml, err := ghodss_yaml.JSONToYAML(workloadInstanceJson)
			if err != nil {
				cli.Error("failed to convert workload instance JSON to YAML", err)
				os.Exit(1)
			}

			fmt.Println(string(workloadInstanceYaml))
		}
	},
	Short:        "Describe a workload instance",
	SilenceUsage: true,
	Use:          "workload-instance",
}

func init() {
	DescribeCmd.AddCommand(DescribeWorkloadInstanceCmd)

	DescribeWorkloadInstanceCmd.Flags().StringVarP(
		&describeWorkloadInstanceConfigPath,
		"config", "c", "", "Path to file with workload instance config.")
	DescribeWorkloadInstanceCmd.Flags().StringVarP(
		&describeWorkloadInstanceName,
		"name", "n", "", "Name of workload instance.")
	DescribeWorkloadInstanceCmd.Flags().StringVarP(
		&describeWorkloadInstanceOutput,
		"output", "o", "plain", "Output format for object description. One of 'plain','json','yaml'.  Will be ignored if the --field flag is also used.  Plain output produces select details about the object.  JSON and YAML output formats include all direct attributes of the object")
	DescribeWorkloadInstanceCmd.Flags().StringVarP(
		&describeWorkloadInstanceField,
		"field", "f", "", "Object field to get value for. If used, --output flag will be ignored.  *Only* the value of the desired field will be returned.  Will not return information on related objects, only direct attributes of the object itself.")
	DescribeWorkloadInstanceCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
}
