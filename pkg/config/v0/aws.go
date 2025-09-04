package v0

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"gopkg.in/ini.v1"

	"github.com/threeport/threeport/internal/aws/status"
	"github.com/threeport/threeport/internal/kubernetes-runtime/mapping"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// AwsAccountConfig contains the config for an AWS account.
type AwsAccountConfig struct {
	AwsAccount AwsAccountValues `yaml:"AwsAccount"`
}

// AwsAccountValues contains the attributes needed to manage an AWS account.
type AwsAccountValues struct {
	Name             *string `yaml:"Name"`
	AccountID        *string `yaml:"AccountID"`
	DefaultAccount   *bool   `yaml:"DefaultAccount"`
	DefaultRegion    *string `yaml:"DefaultRegion"`
	AccessKeyID      *string `yaml:"AccessKeyID"`
	SecretAccessKey  *string `yaml:"SecretAccessKey"`
	RoleArn          *string `yaml:"RoleArn"`
	LocalConfig      *string `yaml:"LocalConfig"`
	LocalCredentials *string `yaml:"LocalCredentials"`
	LocalProfile     *string `yaml:"LocalProfile"`
}

// AwsEksKubernetesRuntimeConfig contains the config for an AWS EKS
// kubernetes runtime which is an abstraction of an AWS EKS kubernetes runtime
// definition and instance.
type AwsEksKubernetesRuntimeConfig struct {
	AwsEksKubernetesRuntime AwsEksKubernetesRuntimeValues `yaml:"AwsEksKubernetesRuntime"`
}

// AwsEksKubernetesRuntimeValues contains the attributes needed to
// manage an AWS EKS kubernetes runtime definition and instance.
type AwsEksKubernetesRuntimeValues struct {
	Name                         *string `yaml:"Name"`
	AwsAccountName               *string `yaml:"AwsAccountName"`
	ZoneCount                    *int    `yaml:"ZoneCount"`
	DefaultNodeGroupInstanceType *string `yaml:"DefaultNodeGroupInstanceType"`
	DefaultNodeGroupInitialSize  *int    `yaml:"DefaultNodeGroupInitialSize"`
	DefaultNodeGroupMinimumSize  *int    `yaml:"DefaultNodeGroupMinimumSize"`
	DefaultNodeGroupMaximumSize  *int    `yaml:"DefaultNodeGroupMaximumSize"`
	Region                       *string `yaml:"Region"`
}

// AwsEksKubernetesRuntimeDefinitionConfig contains the config for an AWS EKS
// kubernetes runtime definition.
type AwsEksKubernetesRuntimeDefinitionConfig struct {
	AwsEksKubernetesRuntimeDefinition AwsEksKubernetesRuntimeDefinitionValues `yaml:"AwsEksKubernetesRuntimeDefinition"`
}

// AwsEksKubernetesRuntimeDefinitionValues contains the attributes needed to
// manage an AWS EKS kubernetes runtime definition.
type AwsEksKubernetesRuntimeDefinitionValues struct {
	Name                         *string `yaml:"Name"`
	AwsAccountName               *string `yaml:"AwsAccountName"`
	ZoneCount                    *int    `yaml:"ZoneCount"`
	DefaultNodeGroupInstanceType *string `yaml:"DefaultNodeGroupInstanceType"`
	DefaultNodeGroupInitialSize  *int    `yaml:"DefaultNodeGroupInitialSize"`
	DefaultNodeGroupMinimumSize  *int    `yaml:"DefaultNodeGroupMinimumSize"`
	DefaultNodeGroupMaximumSize  *int    `yaml:"DefaultNodeGroupMaximumSize"`
}

// AwsEksKubernetesRuntimeInstanceConfig contains the config for an AWS EKS
// kubernetes runtime instance.
type AwsEksKubernetesRuntimeInstanceConfig struct {
	AwsEksKubernetesRuntimeInstance AwsEksKubernetesRuntimeInstanceValues `yaml:"AwsEksKubernetesRuntimeInstance"`
}

// AwsEksKubernetesRuntimeInstanceValues contains the attributes needed to
// manage an AWS EKS kubernetes runtime instance.
type AwsEksKubernetesRuntimeInstanceValues struct {
	Name                              *string                                  `yaml:"Name"`
	Region                            *string                                  `yaml:"Region"`
	AwsEksKubernetesRuntimeDefinition *AwsEksKubernetesRuntimeDefinitionValues `yaml:"AwsEksKubernetesRuntimeDefinition"`
}

// Create creates an AWS account in the Threeport API.
func (a *AwsAccountValues) Create(apiClient *http.Client, apiEndpoint string) (*v0.AwsAccount, error) {
	// validate required fields
	if a.Name == nil || a.AccountID == nil {
		return nil, errors.New("missing required field/s in config - required fields: Name, AccountID")
	}

	// validate config and credentials properly provided
	explain := `
In order to configure an AWS account provide the fields:
DefaultRegion, AccessKeyID and SecretAccessKey
OR
LocalConfig, LocalCredentials and LocalProfile
`
	localConfig := false
	explicitConfig := false
	if a.LocalConfig != nil && a.LocalCredentials != nil && a.LocalProfile != nil {
		localConfig = true
	}
	if a.DefaultRegion != nil && a.AccessKeyID != nil && a.SecretAccessKey != nil {
		explicitConfig = true
	}
	switch {
	case localConfig && explicitConfig:
		msg := fmt.Sprintf("local and explicit configurations provided %s", explain)
		return nil, errors.New(msg)
	case !localConfig && !explicitConfig:
		msg := fmt.Sprintf("neither local nor explicit configurations provided %s", explain)
		return nil, errors.New(msg)
	}

	// validate that no other default AWS account exists
	if a.DefaultAccount != nil && *a.DefaultAccount {
		existingAccounts, err := client.GetAwsAccounts(apiClient, apiEndpoint)
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve existing AWS accounts to check default accounts: %w", err)
		}
		for _, existing := range *existingAccounts {
			if existing.DefaultAccount != nil && *existing.DefaultAccount {
				msg := fmt.Sprintf("cannot designate new account as default account - %s is already the default account", *existing.Name)
				return nil, errors.New(msg)
			}
		}
	}

	// establish default region from explicit declaration in config or AWS config file
	var region string
	if a.DefaultRegion == nil {
		awsConfig, err := ini.Load(*a.LocalConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to load aws config: %w", err)
		}
		if awsConfig.Section(*a.LocalProfile).HasKey("region") {
			region = awsConfig.Section(*a.LocalProfile).Key("region").String()
		} else {
			return nil, errors.New(
				fmt.Sprintf("profile %s not found in aws config %s", *a.LocalProfile, *a.LocalConfig),
			)
		}
	} else {
		region = *a.DefaultRegion
	}

	// retrieve access key ID and secret access key if needed
	var accessKeyID string
	var secretAccessKey string
	if a.AccessKeyID == nil && a.SecretAccessKey == nil {
		awsCredentials, err := ini.Load(*a.LocalCredentials)
		if err != nil {
			return nil, fmt.Errorf("failed to load aws credentials: %w", err)
		}
		if awsCredentials.Section(*a.LocalProfile).HasKey("aws_access_key_id") &&
			awsCredentials.Section(*a.LocalProfile).HasKey("aws_secret_access_key") {
			accessKeyID = awsCredentials.Section(*a.LocalProfile).Key("aws_access_key_id").String()
			secretAccessKey = awsCredentials.Section(*a.LocalProfile).Key("aws_secret_access_key").String()
		}
	} else {
		accessKeyID = *a.AccessKeyID
		secretAccessKey = *a.SecretAccessKey
	}

	// construct AWS account object
	awsAccount := v0.AwsAccount{
		Name:            a.Name,
		DefaultAccount:  a.DefaultAccount,
		DefaultRegion:   &region,
		AccountID:       a.AccountID,
		AccessKeyID:     &accessKeyID,
		SecretAccessKey: &secretAccessKey,
		RoleArn:         a.RoleArn,
	}

	// create AWS account
	createdAwsAccount, err := client.CreateAwsAccount(apiClient, apiEndpoint, &awsAccount)
	if err != nil {
		return nil, fmt.Errorf("failed to create aws account in threeport API: %w", err)
	}

	return createdAwsAccount, nil
}

// Describe returns details related to an AWS account.
func (a *AwsAccountValues) Describe(apiClient *http.Client, apiEndpoint string) (*status.AwsAccountStatusDetail, error) {
	// validate
	if a.Name == nil {
		return nil, errors.New("missing required field: Name")
	}

	// get AWS account by name
	awsAccount, err := client.GetAwsAccountByName(apiClient, apiEndpoint, *a.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to find AWS Account with name %s: %w", *a.Name, err)
	}

	// get AWS account status
	statusDetail, err := status.GetAwsAccountStatus(
		apiClient,
		apiEndpoint,
		*awsAccount.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get status for AWS account with name %s: %w", *a.Name, err)
	}

	return statusDetail, nil
}

// Delete deletes an AWS account from the Threeport API.
func (a *AwsAccountValues) Delete(apiClient *http.Client, apiEndpoint string) (*v0.AwsAccount, error) {
	//validate
	if a.Name == nil {
		return nil, errors.New("missing required field: Name")
	}

	// get AWS account by name
	awsAccount, err := client.GetAwsAccountByName(apiClient, apiEndpoint, *a.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to find AWS account with name %s: %w", *a.Name, err)
	}

	// delete AWS account
	deletedAwsAccount, err := client.DeleteAwsAccount(apiClient, apiEndpoint, *awsAccount.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to delete AWS account from threeport API: %w", err)
	}

	return deletedAwsAccount, nil
}

// Create creates a AWS EKS kubernetes runtime definition and instance in the Threeport API.
func (w *AwsEksKubernetesRuntimeValues) Create(
	apiClient *http.Client,
	apiEndpoint string,
) (*v0.AwsEksKubernetesRuntimeDefinition, *v0.AwsEksKubernetesRuntimeInstance, error) {

	// get operations
	operations, createdAwsEksKubernetesRuntimeDefinition, createdAwsEksKubernetesRuntimeInstance := w.GetOperations(
		apiClient,
		apiEndpoint,
	)

	// execute create operations
	if err := operations.Create(); err != nil {
		return nil, nil, fmt.Errorf(
			"failed to execute create operations for AWS EKS kubernetes runtime defined instance with name %s: %w",
			*w.Name,
			err,
		)
	}

	return createdAwsEksKubernetesRuntimeDefinition, createdAwsEksKubernetesRuntimeInstance, nil
}

// Delete deletes a AWS EKS kubernetes runtime definition and AWS EKS
// kubernetes runtime instance.
func (w *AwsEksKubernetesRuntimeValues) Delete(
	apiClient *http.Client,
	apiEndpoint string,
) (*v0.AwsEksKubernetesRuntimeDefinition, *v0.AwsEksKubernetesRuntimeInstance, error) {

	// get operation
	operations, _, _ := w.GetOperations(apiClient, apiEndpoint)

	// execute delete operations
	if err := operations.Delete(); err != nil {
		return nil, nil, fmt.Errorf(
			"failed to execute delete operations for AWS EKS kubernetes runtime defined instance %s: %w",
			*w.Name,
			err,
		)
	}

	return nil, nil, nil
}

// Create creates a AWS EKS kubernetes runtime definition in the Threeport API.
func (e *AwsEksKubernetesRuntimeDefinitionValues) Create(apiClient *http.Client, apiEndpoint string) (*v0.AwsEksKubernetesRuntimeDefinition, error) {
	// validate required fields
	if e.Name == nil || e.AwsAccountName == nil || e.ZoneCount == nil ||
		e.DefaultNodeGroupInstanceType == nil || e.DefaultNodeGroupInitialSize == nil ||
		e.DefaultNodeGroupMinimumSize == nil || e.DefaultNodeGroupMaximumSize == nil {
		return nil, errors.New("missing required field/s in config - required fields: Name, AwsAccountName, ZoneCount, DefaultNodeGroupInstanceType, DefaultNodeGroupInitialSize, DefaultNodeGroupMinimumSize, DefaultNodeGroupMaximumSize")
	}

	// look up AWS account by name
	awsAccount, err := client.GetAwsAccountByName(apiClient, apiEndpoint, *e.AwsAccountName)
	if err != nil {
		return nil, fmt.Errorf("failed to find AWS account with name %s: %w", *e.AwsAccountName, err)
	}

	// construct kubernetes runtime definition
	infraProvider := v0.KubernetesRuntimeInfraProviderEKS
	kubernetesRuntimeDefinition := v0.KubernetesRuntimeDefinition{
		Definition: v0.Definition{
			Name: e.Name,
		},
		Reconciliation: v0.Reconciliation{
			Reconciled: util.Ptr(true),
		},
		InfraProvider:            &infraProvider,
		InfraProviderAccountName: awsAccount.Name,
	}

	// create kubernetes runtime definition
	createdKubernetesRuntimeDefinition, err := client.CreateKubernetesRuntimeDefinition(apiClient, apiEndpoint, &kubernetesRuntimeDefinition)
	if err != nil {
		return nil, fmt.Errorf("failed to create new kubernetes runtime definition for AWS EKS definition: %w", err)
	}

	// construct AWS EKS kubernetes runtime definition object
	awsEksKubernetesRuntimeDefinition := v0.AwsEksKubernetesRuntimeDefinition{
		Definition: v0.Definition{
			Name: e.Name,
		},
		AwsAccountID:                  awsAccount.ID,
		ZoneCount:                     e.ZoneCount,
		DefaultNodeGroupInstanceType:  e.DefaultNodeGroupInstanceType,
		DefaultNodeGroupInitialSize:   e.DefaultNodeGroupInitialSize,
		DefaultNodeGroupMinimumSize:   e.DefaultNodeGroupMinimumSize,
		DefaultNodeGroupMaximumSize:   e.DefaultNodeGroupMaximumSize,
		KubernetesRuntimeDefinitionID: createdKubernetesRuntimeDefinition.ID,
	}

	// create AWS EKS kubernetes definition
	createdAwsEksKubernetesRuntimeDefinition, err := client.CreateAwsEksKubernetesRuntimeDefinition(apiClient, apiEndpoint, &awsEksKubernetesRuntimeDefinition)
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS EKS kubernetes runtime definition in threeport API: %w", err)
	}

	return createdAwsEksKubernetesRuntimeDefinition, nil
}

// Describe returns details related to a AWS EKS kubernetes runtime definition.
func (e *AwsEksKubernetesRuntimeDefinitionValues) Describe(
	apiClient *http.Client,
	apiEndpoint string,
) (*status.AwsEksKubernetesRuntimeDefinitionStatusDetail, error) {
	// validate
	if e.Name == nil {
		return nil, errors.New("missing required field: Name")
	}

	// get AWS EKS kubernetes runtime definition by name
	awsEksKubernetesRuntimeDefinition, err := client.GetAwsEksKubernetesRuntimeDefinitionByName(
		apiClient,
		apiEndpoint,
		*e.Name,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to find AWS EKS kubernetes runtime definition with name %s: %w", *e.Name, err)
	}

	// get AWS EKS kubernetes runtime definition status
	statusDetail, err := status.GetAwsEksKubernetesRuntimeDefinitionStatus(
		apiClient,
		apiEndpoint,
		awsEksKubernetesRuntimeDefinition,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get status for AWS EKS kubernetes runtime definition with name %s: %w", *e.Name, err)
	}

	return statusDetail, nil
}

// Delete deletes an AWS EKS kubernetes definition from the Threeport API.
func (e *AwsEksKubernetesRuntimeDefinitionValues) Delete(apiClient *http.Client, apiEndpoint string) (*v0.AwsEksKubernetesRuntimeDefinition, error) {
	// validate
	if e.Name == nil {
		return nil, errors.New("missing required field: Name")
	}

	// get AWS EKS kubernetes definition by name
	awsEksKubernetesRuntimeDefinition, err := client.GetAwsEksKubernetesRuntimeDefinitionByName(apiClient, apiEndpoint, *e.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to find AWS EKS kubernetes definition with name %s: %w", *e.Name, err)
	}

	// delete associated kubernetes runtime definition
	_, err = client.DeleteKubernetesRuntimeDefinition(
		apiClient,
		apiEndpoint,
		*awsEksKubernetesRuntimeDefinition.KubernetesRuntimeDefinitionID,
	)
	if err != nil && !errors.Is(err, client_lib.ErrObjectNotFound) {
		return nil, fmt.Errorf("failed to delete associated kubernetes runtime definition: %w", err)
	}

	// delete AWS EKS kubernetes definition
	deletedAwsEksKubernetesRuntimeDefinition, err := client.DeleteAwsEksKubernetesRuntimeDefinition(apiClient, apiEndpoint, *awsEksKubernetesRuntimeDefinition.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to delete AWS EKS kubernetes definition from threeport API: %w", err)
	}

	return deletedAwsEksKubernetesRuntimeDefinition, nil
}

// Create creates an AWS EKS kubernetes runtime instance in the threeport API.
func (e *AwsEksKubernetesRuntimeInstanceValues) Create(apiClient *http.Client, apiEndpoint string) (*v0.AwsEksKubernetesRuntimeInstance, error) {
	// validate required fields
	if e.Name == nil || e.AwsEksKubernetesRuntimeDefinition.Name == nil {
		return nil, errors.New("missing required field/s in config - required fields: Name, AwsEksKubernetesRuntimeDefinition.Name")
	}

	// look up AWS EKS kubernetes runtime definition by name
	awsEksKubernetesRuntimeDefinition, err := client.GetAwsEksKubernetesRuntimeDefinitionByName(apiClient, apiEndpoint, *e.AwsEksKubernetesRuntimeDefinition.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to find AWS EKS kubernetes runtime definition with name %s: %w", *e.AwsEksKubernetesRuntimeDefinition.Name, err)
	}

	// get location for provider AWS region
	location, err := mapping.GetLocationForAwsRegion(*e.Region)
	if err != nil {
		return nil, fmt.Errorf("failed to get Threeport location for AWS region %s: %w", *e.Region, err)
	}

	// construct kubernetes runtime instance object
	controlPlaneHost := false
	defaultRuntime := false
	kubernetesRuntimeInstance := v0.KubernetesRuntimeInstance{
		Instance: v0.Instance{
			Name: e.Name,
		},
		Reconciliation: v0.Reconciliation{
			Reconciled: util.Ptr(true),
		},
		Location:                      &location,
		ThreeportControlPlaneHost:     &controlPlaneHost,
		DefaultRuntime:                &defaultRuntime,
		KubernetesRuntimeDefinitionID: awsEksKubernetesRuntimeDefinition.KubernetesRuntimeDefinitionID,
	}

	// create kubernetes runtime instance
	createdKubernetesRuntimeInstance, err := client.CreateKubernetesRuntimeInstance(apiClient, apiEndpoint, &kubernetesRuntimeInstance)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes runtime instance for AWS EKS instance: %w", err)
	}

	// construct AWS EKS kubernetes runtime instance object
	awsEksKubernetesRuntimeInstance := v0.AwsEksKubernetesRuntimeInstance{
		Instance: v0.Instance{
			Name: e.Name,
		},
		Region:                              e.Region,
		KubernetesRuntimeInstanceID:         createdKubernetesRuntimeInstance.ID,
		AwsEksKubernetesRuntimeDefinitionID: awsEksKubernetesRuntimeDefinition.ID,
	}

	// create AWS EKS kubernetes runtime instance
	createdAwsEksKubernetesRuntimeInstance, err := client.CreateAwsEksKubernetesRuntimeInstance(apiClient, apiEndpoint, &awsEksKubernetesRuntimeInstance)
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS EKS kubernetes runtime instance in threeport API: %w", err)
	}

	return createdAwsEksKubernetesRuntimeInstance, nil
}

// Describe returns details related to a AWS EKS kubernetes runtime instance.
func (e *AwsEksKubernetesRuntimeInstanceValues) Describe(
	apiClient *http.Client,
	apiEndpoint string,
) (*status.AwsEksKubernetesRuntimeInstanceStatusDetail, error) {
	// validate
	if e.Name == nil {
		return nil, errors.New("missing required field: Name")
	}

	// get AWS EKS kubernetes runtime instance by name
	awsEksKubernetesRuntimeInstance, err := client.GetAwsEksKubernetesRuntimeInstanceByName(
		apiClient,
		apiEndpoint,
		*e.Name,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to find AWS EKS kubernetes runtime instance with name %s: %w", *e.Name, err)
	}

	// get AWS EKS kubernetes runtime instance status
	statusDetail, err := status.GetAwsEksKubernetesRuntimeInstanceStatus(
		apiClient,
		apiEndpoint,
		awsEksKubernetesRuntimeInstance,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get status for AWS EKS kubernetes runtime instance with name %s: %w", *e.Name, err)
	}

	return statusDetail, nil
}

// Delete deletes an AWS EKS kubernetes runtime instance from the Threeport API.
func (e *AwsEksKubernetesRuntimeInstanceValues) Delete(apiClient *http.Client, apiEndpoint string) (*v0.AwsEksKubernetesRuntimeInstance, error) {
	// validate
	if e.Name == nil {
		return nil, errors.New("missing required field: Name")
	}

	// get AWS EKS kubernetes runtime instance by name
	awsEksKubernetesRuntimeInstance, err := client.GetAwsEksKubernetesRuntimeInstanceByName(apiClient, apiEndpoint, *e.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to find AWS EKS kubernetes runtime instance with name %s: %w", *e.Name, err)
	}

	// delete AWS EKS kubernetes runtime instance
	deletedAwsEksKubernetesRuntimeInstance, err := client.DeleteAwsEksKubernetesRuntimeInstance(
		apiClient,
		apiEndpoint,
		*awsEksKubernetesRuntimeInstance.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to delete AWS EKS kubernetes runtime instance from threeport API: %w", err)
	}

	// wait for AWS EKS kubernetes runtime instance to be deleted
	util.Retry(90, 10, func() error {
		if _, err := client.GetAwsEksKubernetesRuntimeInstanceByName(
			apiClient,
			apiEndpoint,
			*awsEksKubernetesRuntimeInstance.Name,
		); err == nil {
			return errors.New("AWS EKS kubernetes runtime instance not deleted")
		}
		return nil
	})

	// get kubernetes runtime instance
	kubernetesRuntimeInstance, err := client.GetKubernetesRuntimeInstanceByID(
		apiClient,
		apiEndpoint,
		*awsEksKubernetesRuntimeInstance.KubernetesRuntimeInstanceID,
	)
	if err != nil {
		// if the kubernetes runtime instance wasn't found, there's no more to
		// do - return the error if something other than 'object not found'
		if !errors.Is(err, client_lib.ErrObjectNotFound) {
			return nil, fmt.Errorf("failed to get associated kubernetes runtime instance: %w", err)
		}
	}
	// if kubernetes runtime found, remove it
	if err == nil {
		// update kubernetes runtime instance to set the deletion confirmed
		// timestamp - this will allow deletion of the k8s runtime object without
		// triggering unnecessary reconciliation
		now := time.Now().UTC()
		kubernetesRuntimeInstance.DeletionConfirmed = &now
		_, err = client.UpdateKubernetesRuntimeInstance(
			apiClient,
			apiEndpoint,
			kubernetesRuntimeInstance,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to update associated kubernetes runtime instance to set deletion confirmed: %w", err)
		}

		// delete kubernetes runtime instance
		_, err = client.DeleteKubernetesRuntimeInstance(
			apiClient,
			apiEndpoint,
			*awsEksKubernetesRuntimeInstance.KubernetesRuntimeInstanceID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to delete associated kubernetes runtime instance: %w", err)
		}

		// wait for kubernetes runtime instance to be deleted
		util.Retry(10, 1, func() error {
			if _, err := client.GetKubernetesRuntimeInstanceByName(
				apiClient,
				apiEndpoint,
				*kubernetesRuntimeInstance.Name,
			); err == nil {
				return errors.New("kubernetes runtime instance not deleted")
			}
			return nil
		})
	}

	return deletedAwsEksKubernetesRuntimeInstance, nil
}

// GetOperations returns a slice of operations used to create or delete an AWS
// EKS kubernetes runtime.
func (e *AwsEksKubernetesRuntimeValues) GetOperations(
	apiClient *http.Client,
	apiEndpoint string,
) (*util.Operations, *v0.AwsEksKubernetesRuntimeDefinition, *v0.AwsEksKubernetesRuntimeInstance) {

	var err error
	var createdAwsEksKubernetesRuntimeInstance v0.AwsEksKubernetesRuntimeInstance
	var createdAwsEksKubernetesRuntimeDefinition v0.AwsEksKubernetesRuntimeDefinition

	operations := util.Operations{}

	// add AWS EKS kubernetes runtime definition operation
	awsEksKubernetesRuntimeDefinitionValues := AwsEksKubernetesRuntimeDefinitionValues{
		Name:                         e.Name,
		AwsAccountName:               e.AwsAccountName,
		ZoneCount:                    e.ZoneCount,
		DefaultNodeGroupInstanceType: e.DefaultNodeGroupInstanceType,
		DefaultNodeGroupInitialSize:  e.DefaultNodeGroupInitialSize,
		DefaultNodeGroupMinimumSize:  e.DefaultNodeGroupMinimumSize,
		DefaultNodeGroupMaximumSize:  e.DefaultNodeGroupMaximumSize,
	}
	operations.AppendOperation(util.Operation{
		Name: "AWS EKS kubernetes runtime definition",
		Create: func() error {
			awsEksKubernetesRuntimeDefinition, err := awsEksKubernetesRuntimeDefinitionValues.Create(
				apiClient,
				apiEndpoint,
			)
			if err != nil {
				return fmt.Errorf(
					"failed to create AWS EKS kubernetes runtime definition with name %s: %w",
					*awsEksKubernetesRuntimeDefinitionValues.Name,
					err,
				)
			}
			createdAwsEksKubernetesRuntimeDefinition = *awsEksKubernetesRuntimeDefinition
			return nil
		},
		Delete: func() error {
			_, err = awsEksKubernetesRuntimeDefinitionValues.Delete(apiClient, apiEndpoint)
			if err != nil {
				return fmt.Errorf(
					"failed to delete AWS EKS kubernetes runtime definition with name %s: %w",
					*awsEksKubernetesRuntimeDefinitionValues.Name,
					err,
				)
			}
			return nil
		},
	})

	// add AWS EKS kubernetes runtime instance operation
	awsEksKubernetesRuntimeInstanceValues := AwsEksKubernetesRuntimeInstanceValues{
		Name:   e.Name,
		Region: e.Region,
		AwsEksKubernetesRuntimeDefinition: &AwsEksKubernetesRuntimeDefinitionValues{
			Name: e.Name,
		},
	}
	operations.AppendOperation(util.Operation{
		Name: "AWS EKS kubernetes runtime instance",
		Create: func() error {
			awsEksKubernetesRuntimeInstance, err := awsEksKubernetesRuntimeInstanceValues.Create(
				apiClient,
				apiEndpoint,
			)
			if err != nil {
				return fmt.Errorf(
					"failed to create AWS EKS kubernetes runtime instance with name %s: %w",
					*awsEksKubernetesRuntimeInstanceValues.Name,
					err,
				)
			}
			createdAwsEksKubernetesRuntimeInstance = *awsEksKubernetesRuntimeInstance
			return nil
		},
		Delete: func() error {
			_, err = awsEksKubernetesRuntimeInstanceValues.Delete(apiClient, apiEndpoint)
			if err != nil {
				return fmt.Errorf(
					"failed to delete AWS EKS kubernetes runtime instance with name %s: %w",
					*awsEksKubernetesRuntimeInstanceValues.Name,
					err,
				)
			}
			return nil
		},
	})

	return &operations, &createdAwsEksKubernetesRuntimeDefinition, &createdAwsEksKubernetesRuntimeInstance
}
