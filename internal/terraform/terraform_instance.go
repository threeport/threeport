package terraform

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/go-logr/logr"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	"github.com/threeport/threeport/pkg/encryption/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// terraformInstanceCreated reconciles state for a new terraform instance.
func terraformInstanceCreated(
	r *controller.Reconciler,
	terraformInstance *v0.TerraformInstance,
	log *logr.Logger,
) (int64, error) {
	// set up terraform
	tfDirName, accessKeyId, secretAccessKey, err := setupTerraform(
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
		tfDirName:         tfDirName,
		accessKeyId:       accessKeyId,
		secretAccessKey:   secretAccessKey,
	}

	// get terraform instance operations
	operations := getTerraformInstanceOperations(c)

	// execute terraform instance create
	if err := operations.Create(); err != nil {
		return 0, fmt.Errorf("failed to execute terraform instance create operations: %w", err)
	}

	// update the terraform instance
	terraformInstance.Reconciled = util.BoolPtr(true)
	terraformInstance.TerraformStateDocument = &c.tfState
	terraformInstance.TerraformOutputs = &c.tfOutput
	if _, err := client.UpdateTerraformInstance(
		r.APIClient,
		r.APIServer,
		terraformInstance,
	); err != nil {
		return 0, fmt.Errorf("failed to update terraform instance: %w", err)
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
	tfDirName, accessKeyId, secretAccessKey, err := setupTerraform(
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
		tfDirName:         tfDirName,
		accessKeyId:       accessKeyId,
		secretAccessKey:   secretAccessKey,
	}

	// get terraform instance operations
	operations := getTerraformInstanceOperations(c)

	// execute terraform instance create
	if err := operations.Delete(); err != nil {
		return 0, fmt.Errorf("failed to execute terraform instance delete operations: %w", err)
	}

	return 0, nil
}

// setupTerraform sets up a terraform config directory, initializes terraform
// and returns the config directory and AWS credentials.
func setupTerraform(
	r *controller.Reconciler,
	terraformInstance *v0.TerraformInstance,
	log *logr.Logger,
) (string, string, string, error) {
	// get terraform definition
	terraformDefinition, err := client.GetTerraformDefinitionByID(
		r.APIClient,
		r.APIServer,
		*terraformInstance.TerraformDefinitionID,
	)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to get terraform definition: %w", err)
	}

	// write terraform config to disk
	tfDirName := fmt.Sprintf("/tmp/%d", *terraformInstance.ID)
	_, err = os.Stat(tfDirName)
	if os.IsNotExist(err) {
		if err := os.Mkdir(tfDirName, os.ModePerm); err != nil {
			return "", "", "", fmt.Errorf("failed to create directory for terraform config: %w", err)
		}
	}
	tfFilepath := fmt.Sprintf("%s/terraform.tf", tfDirName)
	if err := os.WriteFile(tfFilepath, []byte(*terraformDefinition.TerraformConfigDir), 0644); err != nil {
		return "", "", "", fmt.Errorf("failed to write terraform config to file: %w", err)
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
		return "", "", "", fmt.Errorf("failed to initialize terrform with output '%s': %w", string(initOut), err)
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
		return "", "", "", fmt.Errorf("failed to get AWS account to use for terraform resource deployment: %w", err)
	}
	accessKeyId, err := encryption.Decrypt(r.EncryptionKey, *awsAccount.AccessKeyID)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to decrypt AWS account access key ID: %w", err)
	}
	secretAccessKey, err := encryption.Decrypt(r.EncryptionKey, *awsAccount.SecretAccessKey)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to decrypt AWS account secret access key: %w", err)
	}

	// decrypt encrypted values
	if terraformInstance.TerraformVarsDocument != nil {
		terraformVarsDoc, err := encryption.Decrypt(r.EncryptionKey, *terraformInstance.TerraformVarsDocument)
		if err != nil {
			return "", "", "", fmt.Errorf("failed to decrypt terraform vars document: %w", err)
		}
		terraformInstance.TerraformVarsDocument = &terraformVarsDoc
	}
	if terraformInstance.TerraformStateDocument != nil {
		terraformStateDoc, err := encryption.Decrypt(r.EncryptionKey, *terraformInstance.TerraformStateDocument)
		if err != nil {
			return "", "", "", fmt.Errorf("failed to decrypt terraform state document: %w", err)
		}
		terraformInstance.TerraformStateDocument = &terraformStateDoc
	}
	if terraformInstance.TerraformOutputs != nil {
		terraformOutputs, err := encryption.Decrypt(r.EncryptionKey, *terraformInstance.TerraformOutputs)
		if err != nil {
			return "", "", "", fmt.Errorf("failed to decrypt terraform outputs: %w", err)
		}
		terraformInstance.TerraformOutputs = &terraformOutputs
	}

	return tfDirName, accessKeyId, secretAccessKey, nil
}
