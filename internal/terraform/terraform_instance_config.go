package terraform

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/go-logr/logr"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TerraformInstanceConfig contains the configuration for terraform instance
// reconciliation.
type TerraformInstanceConfig struct {
	r                   *controller.Reconciler
	terraformInstance   *v0.TerraformInstance
	terraformDefinition *v0.TerraformDefinition
	log                 *logr.Logger
	tfDirName           string
	accessKeyId         string
	secretAccessKey     string
	tfState             string
	tfOutput            string
}

// getTerraformInstanceOperations returns a list of operations for a terraform instance.
func getTerraformInstanceOperations(c *TerraformInstanceConfig) *util.Operations {
	operations := util.Operations{}

	operations.AppendOperation(util.Operation{
		Name:   "terraformInstance",
		Create: c.createTerraformInstance,
		Delete: c.deleteTerraformInstance,
	})

	return &operations
}

// createTerraformInstance runs the 'terraform apply' command to create
// terraform-defined resources.
func (c *TerraformInstanceConfig) createTerraformInstance() error {
	// write terraform vars if applicable
	tfVarsFilepath := fmt.Sprintf("%s/terraform.tfvars", c.tfDirName)
	if c.terraformInstance.TerraformVarsDocument != nil {
		if err := os.WriteFile(tfVarsFilepath, []byte(*c.terraformInstance.TerraformVarsDocument), 0644); err != nil {
			return fmt.Errorf("failed to write terraform vars to file: %w", err)
		}
	}

	// execute 'terrform apply'
	applyCmd := exec.Command(
		"terraform",
		fmt.Sprintf("-chdir=%s", c.tfDirName),
		"apply",
		"-auto-approve",
		"-no-color",
	)
	applyCmd.Env = append(
		applyCmd.Environ(),
		fmt.Sprintf("AWS_ACCESS_KEY_ID=%s", c.accessKeyId),
		fmt.Sprintf("AWS_SECRET_ACCESS_KEY=%s", c.secretAccessKey),
	)
	applyOut, err := applyCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to apply terrform config with output '%s': %w", string(applyOut), err)
	}
	c.log.V(0).Info(
		"terraform apply command executed",
		"output", string(applyOut),
	)

	// capture the terraform state
	tfStateContent, err := os.ReadFile(fmt.Sprintf("%s/terraform.tfstate", c.tfDirName))
	if err != nil {
		return fmt.Errorf("failed to read terraform state file: %w", err)
	}
	c.tfState = string(tfStateContent)

	// capture the terraform outputs by executing 'terraform output'
	outputCmd := exec.Command(
		"terraform",
		fmt.Sprintf("-chdir=%s", c.tfDirName),
		"output",
		"-json",
		"-no-color",
	)
	outputOut, err := outputCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to retrieve terrform output with command output '%s': %w", string(outputOut), err)
	}
	c.log.V(0).Info(
		"terraform output command executed",
		"output", string(outputOut),
	)
	c.tfOutput = string(outputOut)

	return nil
}

// deleteTerraformInstance deletes the terraform resources recoreded in the
// terraform state file with the 'terraform destroy' command.
func (c *TerraformInstanceConfig) deleteTerraformInstance() error {
	// write terraform vars if applicable
	tfVarsFilepath := fmt.Sprintf("%s/terraform.tfvars", c.tfDirName)
	if c.terraformInstance.TerraformVarsDocument != nil {
		if err := os.WriteFile(tfVarsFilepath, []byte(*c.terraformInstance.TerraformVarsDocument), 0644); err != nil {
			return fmt.Errorf("failed to write terraform vars to file: %w", err)
		}
	}

	// write terraform state file
	_, err := os.Stat(c.tfDirName)
	if os.IsNotExist(err) {
		if err := os.Mkdir(c.tfDirName, os.ModePerm); err != nil {
			return fmt.Errorf("failed to create directory for terraform config: %w", err)
		}
	}

	// check to see if state file is present and delete if it is
	// if no state file is present, we can only assume that creation failed and
	// there are no terraform resources to destroy
	if c.terraformInstance.TerraformStateDocument != nil {
		tfStateFilepath := fmt.Sprintf("%s/terraform.tfstate", c.tfDirName)
		if err := os.WriteFile(tfStateFilepath, []byte(*c.terraformInstance.TerraformStateDocument), 0644); err != nil {
			return fmt.Errorf("failed to write terraform state to file: %w", err)
		}

		// execute 'terrform destroy'
		destroyCmd := exec.Command(
			"terraform",
			fmt.Sprintf("-chdir=%s", c.tfDirName),
			"destroy",
			"-auto-approve",
			"-no-color",
		)
		destroyCmd.Env = append(
			destroyCmd.Environ(),
			fmt.Sprintf("AWS_ACCESS_KEY_ID=%s", c.accessKeyId),
			fmt.Sprintf("AWS_SECRET_ACCESS_KEY=%s", c.secretAccessKey),
		)
		destroyOut, err := destroyCmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to destroy terrform config with output '%s': %w", string(destroyOut), err)
		}
	}

	return nil
}
