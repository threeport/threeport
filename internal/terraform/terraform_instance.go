package terraform

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/go-logr/logr"
	"gorm.io/datatypes"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	"github.com/threeport/threeport/pkg/encryption/v0"
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

	// write terraform vars if applicable
	tfVarsFilepath := fmt.Sprintf("%s/terraform.tfvars", tfDirName)
	if terraformInstance.TerraformVarsDocument != nil {
		if err := os.WriteFile(tfVarsFilepath, []byte(*terraformInstance.TerraformVarsDocument), 0644); err != nil {
			return 0, fmt.Errorf("failed to write terraform vars to file: %w", err)
		}
	}

	// execute 'terrform apply'
	applyCmd := exec.Command(
		"terraform",
		fmt.Sprintf("-chdir=%s", tfDirName),
		"apply",
		"-auto-approve",
	)
	applyCmd.Env = append(
		applyCmd.Environ(),
		fmt.Sprintf("AWS_ACCESS_KEY_ID=%s", accessKeyId),
		fmt.Sprintf("AWS_SECRET_ACCESS_KEY=%s", secretAccessKey),
	)
	applyOut, err := applyCmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("failed to apply terrform config with output '%s': %w", string(applyOut), err)
	}
	log.V(0).Info(
		"terraform apply command executed",
		"output", string(applyOut),
	)

	// capture the terraform state and store
	tfStateContent, err := os.ReadFile(fmt.Sprintf("%s/terraform.tfstate", tfDirName))
	if err != nil {
		return 0, fmt.Errorf("failed to read terraform state file: %w", err)
	}
	tfStateJson := datatypes.JSON(tfStateContent)
	terraformInstance.TerraformStateDocument = &tfStateJson
	_, err = client.UpdateTerraformInstance(
		r.APIClient,
		r.APIServer,
		terraformInstance,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to update terraform state for terraform instance: %w", err)
	}

	return 0, nil
}

// terraformInstanceCreated reconciles state for a terraform instance when it is
// changed.
func terraformInstanceUpdated(
	r *controller.Reconciler,
	terraformInstance *v0.TerraformInstance,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}

// terraformInstanceCreated reconciles state for a terraform instance when it is
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

	// write terraform state file
	_, err = os.Stat(tfDirName)
	if os.IsNotExist(err) {
		if err := os.Mkdir(tfDirName, os.ModePerm); err != nil {
			return 0, fmt.Errorf("failed to create directory for terraform config: %w", err)
		}
	}
	tfStateFilepath := fmt.Sprintf("%s/terraform.tfstate", tfDirName)
	if err := os.WriteFile(tfStateFilepath, []byte(*terraformInstance.TerraformStateDocument), 0644); err != nil {
		return 0, fmt.Errorf("failed to write terraform state to file: %w", err)
	}

	// execute 'terrform destroy'
	destroyCmd := exec.Command(
		"terraform",
		fmt.Sprintf("-chdir=%s", tfDirName),
		"destroy",
		"-auto-approve",
	)
	destroyCmd.Env = append(
		destroyCmd.Environ(),
		fmt.Sprintf("AWS_ACCESS_KEY_ID=%s", accessKeyId),
		fmt.Sprintf("AWS_SECRET_ACCESS_KEY=%s", secretAccessKey),
	)
	destroyOut, err := destroyCmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("failed to destroy terrform config with output '%s': %w", string(destroyOut), err)
	}

	log.V(0).Info(
		"terraform destroy command executed",
		"output", string(destroyOut),
	)

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

	return tfDirName, accessKeyId, secretAccessKey, nil
}
