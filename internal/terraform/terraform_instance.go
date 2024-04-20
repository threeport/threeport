package terraform

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/go-logr/logr"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	encryption "github.com/threeport/threeport/pkg/encryption/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// terraformInstanceCreated reconciles state for a new terraform instance.
func terraformInstanceCreated(
	r *controller.Reconciler,
	terraformInstance *v0.TerraformInstance,
	log *logr.Logger,
) (int64, error) {
	// set up terraform
	tfDirName, awsConfig, err := setupTerraform(
		r,
		terraformInstance,
		log,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to set up terraform: %w", err)
	}

	c := &TerraformInstanceConfig{
		r:                 r,
		terraformInstance: terraformInstance,
		log:               log,
		awsConfig:         awsConfig,
		tfDirName:         tfDirName,
	}

	// execute terraform instance create
	if err := c.getTerraformInstanceOperations().Create(); err != nil {
		return 0, fmt.Errorf("failed to execute terraform instance create operations: %w", err)
	}

	// update the terraform instance
	terraformInstance.Reconciled = util.Ptr(true)
	terraformInstance.StateDocument = &c.tfState
	terraformInstance.Outputs = &c.tfOutput
	if _, err := client.UpdateTerraformInstance(
		r.APIClient,
		r.APIServer,
		terraformInstance,
	); err != nil {
		return 0, fmt.Errorf("failed to update terraform instance: %w", err)
	}

	// clean up local files
	if err := os.RemoveAll(c.tfDirName); err != nil {
		// logging err but not returning it as it is non-critical and we do not
		// want to re-queue reconciliation
		log.Error(err, "failed to remove terraform files written to disk")
	}

	return 0, nil
}

// terraformInstanceUpadated reconciles state for a terraform instance when it is
// changed.
func terraformInstanceUpdated(
	r *controller.Reconciler,
	terraformInstance *v0.TerraformInstance,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}

// terraformInstanceDeleted reconciles state for a terraform instance when it is
// removed.
func terraformInstanceDeleted(
	r *controller.Reconciler,
	terraformInstance *v0.TerraformInstance,
	log *logr.Logger,
) (int64, error) {
	// set up terraform
	tfDirName, awsConfig, err := setupTerraform(
		r,
		terraformInstance,
		log,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to set up terraform: %w", err)
	}

	c := &TerraformInstanceConfig{
		r:                 r,
		terraformInstance: terraformInstance,
		log:               log,
		awsConfig:         awsConfig,
		tfDirName:         tfDirName,
	}

	// execute terraform instance create
	if err := c.getTerraformInstanceOperations().Delete(); err != nil {
		return 0, fmt.Errorf("failed to execute terraform instance delete operations: %w", err)
	}

	// clean up local files
	if err := os.RemoveAll(c.tfDirName); err != nil {
		// logging err but not returning it as it is non-critical and we do not
		// want to re-queue reconciliation
		log.Error(err, "failed to remove terraform files written to disk")
	}

	return 0, nil
}

// setupTerraform sets up a terraform config directory, initializes terraform
// and returns the config directory and AWS credentials.
func setupTerraform(
	r *controller.Reconciler,
	terraformInstance *v0.TerraformInstance,
	log *logr.Logger,
) (string, *aws.Config, error) {
	// get terraform definition
	terraformDefinition, err := client.GetTerraformDefinitionByID(
		r.APIClient,
		r.APIServer,
		*terraformInstance.TerraformDefinitionID,
	)
	if err != nil {
		return "", nil, fmt.Errorf("failed to get terraform definition: %w", err)
	}

	// write terraform config to disk
	tfDirName := fmt.Sprintf("/tmp/%d", *terraformInstance.ID)
	_, err = os.Stat(tfDirName)
	if os.IsNotExist(err) {
		if err := os.Mkdir(tfDirName, os.ModePerm); err != nil {
			return "", nil, fmt.Errorf("failed to create directory for terraform config: %w", err)
		}
	}
	tfFilepath := fmt.Sprintf("%s/terraform.tf", tfDirName)
	if err := os.WriteFile(tfFilepath, []byte(*terraformDefinition.ConfigDir), 0644); err != nil {
		return "", nil, fmt.Errorf("failed to write terraform config to file: %w", err)
	}

	// execute `terraform init`
	initCmd := exec.Command(
		"terraform",
		fmt.Sprintf("-chdir=%s", tfDirName),
		"init",
		"-no-color",
	)
	initOut, err := initCmd.CombinedOutput()
	if err != nil {
		return "", nil, fmt.Errorf("failed to initialize terrform with output '%s': %w", string(initOut), err)
	}
	log.V(0).Info(
		"terraform init command executed",
		"output", string(initOut),
	)

	// get AWS credentials
	awsAccount, err := client.GetAwsAccountByID(
		r.APIClient,
		r.APIServer,
		*terraformInstance.AwsAccountID,
	)
	if err != nil {
		return "", nil, fmt.Errorf("failed to get AWS account to use for terraform resource management: %w", err)
	}
	awsConfig, err := kube.GetAwsConfigFromAwsAccount(
		r.EncryptionKey,
		*awsAccount.DefaultRegion,
		awsAccount,
	)
	if err != nil {
		return "", nil, fmt.Errorf("failed to get AWS config from AWS account to use for terraform resource management: %w", err)
	}

	// decrypt encrypted values
	if terraformInstance.VarsDocument != nil {
		terraformVarsDoc, err := encryption.Decrypt(r.EncryptionKey, *terraformInstance.VarsDocument)
		if err != nil {
			return "", nil, fmt.Errorf("failed to decrypt terraform vars document: %w", err)
		}
		terraformInstance.VarsDocument = &terraformVarsDoc
	}
	if terraformInstance.StateDocument != nil {
		terraformStateDoc, err := encryption.Decrypt(r.EncryptionKey, *terraformInstance.StateDocument)
		if err != nil {
			return "", nil, fmt.Errorf("failed to decrypt terraform state document: %w", err)
		}
		terraformInstance.StateDocument = &terraformStateDoc
	}
	if terraformInstance.Outputs != nil {
		terraformOutputs, err := encryption.Decrypt(r.EncryptionKey, *terraformInstance.Outputs)
		if err != nil {
			return "", nil, fmt.Errorf("failed to decrypt terraform outputs: %w", err)
		}
		terraformInstance.Outputs = &terraformOutputs
	}

	return tfDirName, awsConfig, nil
}
