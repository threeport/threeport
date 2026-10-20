package terraform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/go-logr/logr"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
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
	awsConfig           *aws.Config
	tfDirName           string
	tfState             string
	tfOutput            string

	// applyRan records whether terraform apply ran, since only an apply can
	// leave a partial state worth saving
	applyRan bool
}

// getTerraformInstanceOperations returns a list of operations for a terraform instance.
func (c *TerraformInstanceConfig) getTerraformInstanceOperations() *util.Operations {
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
	if c.terraformInstance.VarsDocument != nil {
		if err := os.WriteFile(tfVarsFilepath, []byte(*c.terraformInstance.VarsDocument), 0644); err != nil {
			return fmt.Errorf("failed to write terraform vars to file: %w", err)
		}
	}

	// restore the state a failed attempt saved so terraform picks up the
	// resources it already created, whichever replica made that attempt
	if err := c.restoreTerraformState(); err != nil {
		return err
	}

	// get AWS credentials
	awsCreds, err := c.awsConfig.Credentials.Retrieve(context.Background())
	if err != nil {
		return fmt.Errorf("failed to retrieve AWS credentials: %w", err)
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
		fmt.Sprintf("AWS_ACCESS_KEY_ID=%s", awsCreds.AccessKeyID),
		fmt.Sprintf("AWS_SECRET_ACCESS_KEY=%s", awsCreds.SecretAccessKey),
		fmt.Sprintf("AWS_SESSION_TOKEN=%s", awsCreds.SessionToken),
	)
	c.applyRan = true
	applyOut, err := applyCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to apply terrform config with output '%s': %w", string(applyOut), err)
	}
	c.log.V(0).Info(
		"terraform apply command executed",
		"output", string(applyOut),
	)

	// capture the terraform state
	tfStateContent, err := os.ReadFile(c.tfStateFilepath())
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

// tfStateFilepath returns the path of the terraform state file.
func (c *TerraformInstanceConfig) tfStateFilepath() string {
	return fmt.Sprintf("%s/terraform.tfstate", c.tfDirName)
}

// restoreTerraformState writes the state saved in the API to the terraform
// directory, unless the directory already holds a newer state from an
// attempt in this pod whose save to the API failed.
func (c *TerraformInstanceConfig) restoreTerraformState() error {
	if c.terraformInstance.StateDocument == nil {
		return nil
	}

	localState, err := os.ReadFile(c.tfStateFilepath())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to read terraform state file: %w", err)
	}
	if err == nil && stateIsNewer(localState, []byte(*c.terraformInstance.StateDocument)) {
		return nil
	}

	// write to a temporary file and rename it into place, so a write that
	// fails part way never leaves a truncated state file behind
	tmpFilepath := c.tfStateFilepath() + ".restore"
	if err := os.WriteFile(tmpFilepath, []byte(*c.terraformInstance.StateDocument), 0644); err != nil {
		os.Remove(tmpFilepath)
		return fmt.Errorf("failed to write terraform state to file: %w", err)
	}
	if err := os.Rename(tmpFilepath, c.tfStateFilepath()); err != nil {
		os.Remove(tmpFilepath)
		return fmt.Errorf("failed to move terraform state into place: %w", err)
	}

	return nil
}

// savePartialTerraformState saves the state a failed apply left behind to
// the API.  An apply can create some resources before it fails, and a retry
// can run in another replica, which would otherwise create them again.
func (c *TerraformInstanceConfig) savePartialTerraformState() error {
	// a create that failed before apply ran left no state of its own, and
	// whatever is on disk may be a restore that failed part way
	if !c.applyRan {
		return nil
	}

	tfStateContent, err := os.ReadFile(c.tfStateFilepath())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to read terraform state file: %w", err)
	}

	tfState := string(tfStateContent)
	if _, err := client.UpdateTerraformInstance(
		c.r.APIClient,
		c.r.APIServer,
		&v0.TerraformInstance{
			Common:        v0.Common{ID: c.terraformInstance.ID},
			StateDocument: &tfState,
		},
	); err != nil {
		return fmt.Errorf("failed to save partial terraform state: %w", err)
	}

	return nil
}

// stateIsNewer reports whether candidate is a later version of the same
// terraform state as current, going by the lineage and serial terraform
// records in every state file.
func stateIsNewer(candidate, current []byte) bool {
	type stateVersion struct {
		Lineage string `json:"lineage"`
		Serial  int64  `json:"serial"`
	}
	var candidateVersion, currentVersion stateVersion
	if err := json.Unmarshal(candidate, &candidateVersion); err != nil {
		return false
	}
	if err := json.Unmarshal(current, &currentVersion); err != nil {
		return false
	}

	return candidateVersion.Lineage != "" &&
		candidateVersion.Lineage == currentVersion.Lineage &&
		candidateVersion.Serial > currentVersion.Serial
}

// deleteTerraformInstance deletes the terraform resources recoreded in the
// terraform state file with the 'terraform destroy' command.
func (c *TerraformInstanceConfig) deleteTerraformInstance() error {
	// write terraform vars if applicable
	tfVarsFilepath := fmt.Sprintf("%s/terraform.tfvars", c.tfDirName)
	if c.terraformInstance.VarsDocument != nil {
		if err := os.WriteFile(tfVarsFilepath, []byte(*c.terraformInstance.VarsDocument), 0644); err != nil {
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
	if c.terraformInstance.StateDocument != nil {
		tfStateFilepath := fmt.Sprintf("%s/terraform.tfstate", c.tfDirName)
		if err := os.WriteFile(tfStateFilepath, []byte(*c.terraformInstance.StateDocument), 0644); err != nil {
			return fmt.Errorf("failed to write terraform state to file: %w", err)
		}

		// get AWS credentials
		awsCreds, err := c.awsConfig.Credentials.Retrieve(context.Background())
		if err != nil {
			return fmt.Errorf("failed to retrieve AWS credentials: %w", err)
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
			fmt.Sprintf("AWS_ACCESS_KEY_ID=%s", awsCreds.AccessKeyID),
			fmt.Sprintf("AWS_SECRET_ACCESS_KEY=%s", awsCreds.SecretAccessKey),
			fmt.Sprintf("AWS_SESSION_TOKEN=%s", awsCreds.SessionToken),
		)
		destroyOut, err := destroyCmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to destroy terrform config with output '%s': %w", string(destroyOut), err)
		}
	}

	return nil
}
