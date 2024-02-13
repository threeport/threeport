package v0

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TerraformConfig contains the config for a terraform which is an abstraction of
// a terraform definition and terraform instance.
type TerraformConfig struct {
	Terraform TerraformValues `yaml:"Terraform"`
}

// TerraformValues contains the attributes needed to manage a terraform
// definition and terraform instance.
type TerraformValues struct {
	Name                string           `yaml:"Name"`
	ConfigDir           string           `yaml:"ConfigDir"`
	AwsAccount          AwsAccountValues `yaml:"AwsAccount"`
	VarsDocument        string           `yaml:"VarsDocument"`
	TerraformConfigPath string           `yaml:"TerraformConfigPath"`
}

// TerraformDefinitionConfig contains the config for a terraform definition.
type TerraformDefinitionConfig struct {
	TerraformDefinition TerraformDefinitionValues `yaml:"TerraformDefinition"`
}

// TerraformDefinitionValues contains the attributes needed to manage a terraform
// definition.
type TerraformDefinitionValues struct {
	Name                string `yaml:"Name"`
	ConfigDir           string `yaml:"ConfigDir"`
	TerraformConfigPath string `yaml:"TerraformConfigPath"`
}

// TerraformInstanceConfig contains the config for a terraform instance.
type TerraformInstanceConfig struct {
	TerraformInstance TerraformInstanceValues `yaml:"TerraformInstance"`
}

// TerraformInstanceValues contains the attributes needed to manage a terraform
// instance.
type TerraformInstanceValues struct {
	Name string `yaml:"Name"`
	//AwsAccountName        string                    `yaml:"AwsAccountName"`
	AwsAccount          AwsAccountValues          `yaml:"AwsAccount"`
	VarsDocument        string                    `yaml:"VarsDocument"`
	TerraformDefinition TerraformDefinitionValues `yaml:"TerraformDefinition"`
	TerraformConfigPath string                    `yaml:"TerraformConfigPath"`
}

// Create creates a terraform definition and instance in the Threeport API.
func (t *TerraformValues) Create(apiClient *http.Client, apiEndpoint string) (*v0.TerraformDefinition, *v0.TerraformInstance, error) {

	// get operations
	operations, createdTerraformDefinition, createdTerraformInstance := t.GetOperations(apiClient, apiEndpoint)

	// execute create operations
	if err := operations.Create(); err != nil {
		return nil, nil, err
	}

	return createdTerraformDefinition, createdTerraformInstance, nil
}

// Delete deletes a terraform definition, terraform instance,
// domain name definition, domain name instance,
// gateway definition, and gateway instance from the Threeport API.
func (t *TerraformValues) Delete(apiClient *http.Client, apiEndpoint string) (*v0.TerraformDefinition, *v0.TerraformInstance, error) {

	// get operation
	operations, _, _ := t.GetOperations(apiClient, apiEndpoint)

	// execute delete operations
	if err := operations.Delete(); err != nil {
		return nil, nil, err
	}

	return nil, nil, nil
}

// Validate validates inputs to create terraform definitions.
func (t *TerraformDefinitionValues) Validate() error {
	multiError := util.MultiError{}

	// ensure name is set
	if t.Name == "" {
		multiError.AppendError(errors.New("missing required field in config: Name"))
	}

	// ensure terraform config dir is set
	if t.ConfigDir == "" {
		multiError.AppendError(errors.New("missing required field in config: ConfigDir"))
	}

	return multiError.Error()
}

// Create creates a terraform definition in the Threeport API.
func (t *TerraformDefinitionValues) Create(apiClient *http.Client, apiEndpoint string) (*v0.TerraformDefinition, error) {
	// validate inputs
	if err := t.Validate(); err != nil {
		return nil, err
	}

	// build the path to the terraform config dir relative to the user's working
	// directory
	configDir := filepath.Dir(t.TerraformConfigPath)
	relativeTerraformConfigPath := filepath.Join(configDir, t.ConfigDir)

	// collect all the terraform config files
	var terraformConfigFiles []string
	if err := filepath.Walk(relativeTerraformConfigPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && strings.HasSuffix(info.Name(), ".tf") {
			terraformConfigFiles = append(terraformConfigFiles, path)
		}

		return nil
	}); err != nil {
		return nil, fmt.Errorf("failed to find terraform config files in provided config dir: %w", err)
	}
	if len(terraformConfigFiles) == 0 {
		return nil, fmt.Errorf("no terraform config files with '.tf' file extension found in provided config dir: %s", t.ConfigDir)
	}

	// load terraform configs
	var concatConfig string
	for _, configFile := range terraformConfigFiles {
		configContent, err := os.ReadFile(configFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read terraform config file %s: %w", configFile, err)
		}
		concatConfig += string(configContent)
		concatConfig += "\n"
	}

	// construct terraform definition object
	terraformDefinition := v0.TerraformDefinition{
		Definition: v0.Definition{
			Name: &t.Name,
		},
		ConfigDir: &concatConfig,
	}

	// create terraform definition
	createdTerraformDefinition, err := client.CreateTerraformDefinition(apiClient, apiEndpoint, &terraformDefinition)
	if err != nil {
		return nil, fmt.Errorf("failed to create terraform definition in threeport API: %w", err)
	}

	return createdTerraformDefinition, nil
}

// Delete deletes a terraform definition from the Threeport API.
func (t *TerraformDefinitionValues) Delete(apiClient *http.Client, apiEndpoint string) (*v0.TerraformDefinition, error) {
	// get terraform definition by name
	terraformDefinition, err := client.GetTerraformDefinitionByName(apiClient, apiEndpoint, t.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to find terraform definition with name %s: %w", t.Name, err)
	}

	// delete terraform definition
	deletedTerraformDefinition, err := client.DeleteTerraformDefinition(apiClient, apiEndpoint, *terraformDefinition.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to delete terraform definition from threeport API: %w", err)
	}

	return deletedTerraformDefinition, nil
}

// Validate validates inputs to create terraform instances.
func (t *TerraformInstanceValues) Validate() error {
	multiError := util.MultiError{}

	// ensure name is set
	if t.Name == "" {
		multiError.AppendError(errors.New("missing required field in config: Name"))
	}

	// ensure AWS account name is set
	if t.AwsAccount.Name == "" {
		multiError.AppendError(errors.New("missing required field in config: AwsAccount.Name"))
	}

	// ensure the terraform definition name is set
	if t.TerraformDefinition.Name == "" {
		multiError.AppendError(errors.New("missing required field in config: TerraformDefinition.Name"))
	}

	return multiError.Error()
}

// Create creates a terraform instance in the Threeport API.
func (t *TerraformInstanceValues) Create(apiClient *http.Client, apiEndpoint string) (*v0.TerraformInstance, error) {
	// validate inputs
	if err := t.Validate(); err != nil {
		return nil, err
	}

	// get terraform definition by name
	terraformDefinition, err := client.GetTerraformDefinitionByName(
		apiClient,
		apiEndpoint,
		t.TerraformDefinition.Name,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to find terraform definition with name %s: %w", t.TerraformDefinition.Name, err)
	}

	// get AWS Account by name
	awsAccount, err := client.GetAwsAccountByName(
		apiClient,
		apiEndpoint,
		t.AwsAccount.Name,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to find AWS account with name %s: %w", t.AwsAccount.Name, err)
	}

	// construct terraform instance object
	terraformInstance := v0.TerraformInstance{
		Instance: v0.Instance{
			Name: &t.Name,
		},
		AwsAccountID:          awsAccount.ID,
		TerraformDefinitionID: terraformDefinition.ID,
	}

	// add terraform vars if supplied
	if t.VarsDocument != "" {
		// build the path to the terraform config dir relative to the user's working
		// directory
		configDir := filepath.Dir(t.TerraformConfigPath)
		varsDoc := filepath.Join(configDir, t.VarsDocument)
		varsContent, err := os.ReadFile(varsDoc)
		if err != nil {
			return nil, fmt.Errorf("failed to read terraform vars file %s: %w", t.VarsDocument, err)
		}

		// add the terraform vars to the terraform instance object
		varsContentStr := string(varsContent)
		terraformInstance.VarsDocument = &varsContentStr
	}

	// create terraform instance
	createdTerraformInstance, err := client.CreateTerraformInstance(apiClient, apiEndpoint, &terraformInstance)
	if err != nil {
		return nil, fmt.Errorf("failed to create terraform instance in threeport API: %w", err)
	}

	return createdTerraformInstance, nil
}

// Delete deletes a terraform instance from the Threeport API.
func (t *TerraformInstanceValues) Delete(apiClient *http.Client, apiEndpoint string) (*v0.TerraformInstance, error) {
	// get terraform instance by name
	terraformInstance, err := client.GetTerraformInstanceByName(apiClient, apiEndpoint, t.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to find terraform instance with name %s: %w", t.Name, err)
	}

	// delete terraform instance
	deletedTerraformInstance, err := client.DeleteTerraformInstance(apiClient, apiEndpoint, *terraformInstance.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to delete terraform instance from threeport API: %w", err)
	}

	// wait for terraform instance to be deleted
	util.Retry(120, 10, func() error {
		if _, err := client.GetTerraformInstanceByName(apiClient, apiEndpoint, t.Name); err == nil {
			return errors.New("terraform instance not deleted")
		}
		return nil
	})

	return deletedTerraformInstance, nil
}

// GetOperations returns a slice of operations used to
// create, update, or delete a terraform.
func (t *TerraformValues) GetOperations(apiClient *http.Client, apiEndpoint string) (*util.Operations, *v0.TerraformDefinition, *v0.TerraformInstance) {

	var err error
	var createdTerraformInstance v0.TerraformInstance
	var createdTerraformDefinition v0.TerraformDefinition

	operations := util.Operations{}

	// add terraform definition operation
	terraformDefinitionValues := TerraformDefinitionValues{
		Name:                t.Name,
		ConfigDir:           t.ConfigDir,
		TerraformConfigPath: t.TerraformConfigPath,
	}
	operations.AppendOperation(util.Operation{
		Name: "terraform definition",
		Create: func() error {
			terraformDefinition, err := terraformDefinitionValues.Create(apiClient, apiEndpoint)
			if err != nil {
				return err
			}
			createdTerraformDefinition = *terraformDefinition
			return nil
		},
		Delete: func() error {
			_, err = terraformDefinitionValues.Delete(apiClient, apiEndpoint)
			return err
		},
	})

	// add terraform instance operation
	terraformInstanceValues := TerraformInstanceValues{
		Name:                t.Name,
		VarsDocument:        t.VarsDocument,
		TerraformConfigPath: t.TerraformConfigPath,
		AwsAccount: AwsAccountValues{
			Name: t.AwsAccount.Name,
		},
		TerraformDefinition: TerraformDefinitionValues{
			Name: t.Name,
		},
	}
	operations.AppendOperation(util.Operation{
		Name: "terraform instance",
		Create: func() error {
			terraformInstance, err := terraformInstanceValues.Create(apiClient, apiEndpoint)
			if err != nil {
				return err
			}
			createdTerraformInstance = *terraformInstance
			return nil
		},
		Delete: func() error {
			_, err = terraformInstanceValues.Delete(apiClient, apiEndpoint)
			return err
		},
	})

	return &operations, &createdTerraformDefinition, &createdTerraformInstance
}
