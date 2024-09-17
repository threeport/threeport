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
	Name             string `yaml:"Name"`
	AccountID        string `yaml:"AccountID"`
	DefaultAccount   bool   `yaml:"DefaultAccount"`
	DefaultRegion    string `yaml:"DefaultRegion"`
	AccessKeyID      string `yaml:"AccessKeyID"`
	SecretAccessKey  string `yaml:"SecretAccessKey"`
	RoleArn          string `yaml:"RoleArn"`
	LocalConfig      string `yaml:"LocalConfig"`
	LocalCredentials string `yaml:"LocalCredentials"`
	LocalProfile     string `yaml:"LocalProfile"`
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
	Name                         string `yaml:"Name"`
	AwsAccountName               string `yaml:"AwsAccountName"`
	ZoneCount                    int    `yaml:"ZoneCount"`
	DefaultNodeGroupInstanceType string `yaml:"DefaultNodeGroupInstanceType"`
	DefaultNodeGroupInitialSize  int    `yaml:"DefaultNodeGroupInitialSize"`
	DefaultNodeGroupMinimumSize  int    `yaml:"DefaultNodeGroupMinimumSize"`
	DefaultNodeGroupMaximumSize  int    `yaml:"DefaultNodeGroupMaximumSize"`
	Region                       string `yaml:"Region"`
}

// AwsEksKubernetesRuntimeDefinitionConfig contains the config for an AWS EKS
// kubernetes runtime definition.
type AwsEksKubernetesRuntimeDefinitionConfig struct {
	AwsEksKubernetesRuntimeDefinition AwsEksKubernetesRuntimeDefinitionValues `yaml:"AwsEksKubernetesRuntimeDefinition"`
}

// AwsEksKubernetesRuntimeDefinitionValues contains the attributes needed to
// manage an AWS EKS kubernetes runtime definition.
type AwsEksKubernetesRuntimeDefinitionValues struct {
	Name                         string `yaml:"Name"`
	AwsAccountName               string `yaml:"AwsAccountName"`
	ZoneCount                    int    `yaml:"ZoneCount"`
	DefaultNodeGroupInstanceType string `yaml:"DefaultNodeGroupInstanceType"`
	DefaultNodeGroupInitialSize  int    `yaml:"DefaultNodeGroupInitialSize"`
	DefaultNodeGroupMinimumSize  int    `yaml:"DefaultNodeGroupMinimumSize"`
	DefaultNodeGroupMaximumSize  int    `yaml:"DefaultNodeGroupMaximumSize"`
}

// AwsEksKubernetesRuntimeInstanceConfig contains the config for an AWS EKS
// kubernetes runtime instance.
type AwsEksKubernetesRuntimeInstanceConfig struct {
	AwsEksKubernetesRuntimeInstance AwsEksKubernetesRuntimeInstanceValues `yaml:"AwsEksKubernetesRuntimeInstance"`
}

// AwsEksKubernetesRuntimeInstanceValues contains the attributes needed to
// manage an AWS EKS kubernetes runtime instance.
type AwsEksKubernetesRuntimeInstanceValues struct {
	Name                              string                                  `yaml:"Name"`
	Region                            string                                  `yaml:"Region"`
	AwsEksKubernetesRuntimeDefinition AwsEksKubernetesRuntimeDefinitionValues `yaml:"AwsEksKubernetesRuntimeDefinition"`
}

// AwsRelationalDatabaseConfig contains the config for an AWS relational
// database.
type AwsRelationalDatabaseConfig struct {
	AwsRelationalDatabase AwsRelationalDatabaseValues `yaml:"AwsRelationalDatabase"`
}

// AwsRelationalDatabaseConfig contains the config for an abstraction of an RDS
// instance and definition.
type AwsRelationalDatabaseValues struct {
	Name               string                  `yaml:"Name"`
	AwsAccountName     string                  `yaml:"AwsAccountName"`
	Engine             string                  `yaml:"Engine"`
	EngineVersion      string                  `yaml:"EngineVersion"`
	DatabaseName       string                  `yaml:"DatabaseName"`
	DatabasePort       int                     `yaml:"DatabasePort"`
	BackupDays         int                     `yaml:"BackupDays"`
	MachineSize        string                  `yaml:"MachineSize"`
	StorageGb          int                     `yaml:"StorageGb"`
	WorkloadSecretName string                  `yaml:"WorkloadSecretName"`
	WorkloadInstance   *WorkloadInstanceValues `yaml:"WorkloadInstance"`
}

// AwsRelationalDatabaseDefinitionConfig contains the config for an AWS
// relational database definition.
type AwsRelationalDatabaseDefinitionConfig struct {
	AwsRelationalDatabaseDefinition AwsRelationalDatabaseDefinitionValues `yaml:"AwsRelationalDatabaseDefinition"`
}

// AwsRelationalDatabaseDefinitionValues contains the attributes needed to
// configure an AWS RDS instance.
type AwsRelationalDatabaseDefinitionValues struct {
	Name               string `yaml:"Name"`
	AwsAccountName     string `yaml:"AwsAccountName"`
	Engine             string `yaml:"Engine"`
	EngineVersion      string `yaml:"EngineVersion"`
	DatabaseName       string `yaml:"DatabaseName"`
	DatabasePort       int    `yaml:"DatabasePort"`
	BackupDays         int    `yaml:"BackupDays"`
	MachineSize        string `yaml:"MachineSize"`
	StorageGb          int    `yaml:"StorageGb"`
	WorkloadSecretName string `yaml:"WorkloadSecretName"`
}

// AwsRelationalDatabaseInstanceConfig contains the config for an AWS relational
// database instance.
type AwsRelationalDatabaseInstanceConfig struct {
	AwsRelationalDatabaseInstance AwsRelationalDatabaseInstanceValues `yaml:"AwsRelationalDatabaseInstance"`
}

// AwsRelationalDatabaseInstanceValues contains the attributes needed to
// create an AWS RDS instance.
type AwsRelationalDatabaseInstanceValues struct {
	Name                            string                                `yaml:"Name"`
	AwsRelationalDatabaseDefinition AwsRelationalDatabaseDefinitionValues `yaml:"AwsRelationalDatabaseDefinition"`
	WorkloadInstance                WorkloadInstanceValues                `yaml:"WorkloadInstance"`
}

// AwsObjectStorageBucketConfig contains the config for an AWS object storage
// bucket.
type AwsObjectStorageBucketConfig struct {
	AwsObjectStorageBucket AwsObjectStorageBucketValues `yaml:"AwsObjectStorageBucket"`
}

// AwsObjectStorageBucketConfig contains the config for an abstraction of an S3
// bucket instance and definition.
type AwsObjectStorageBucketValues struct {
	Name                       string                  `yaml:"Name"`
	AwsAccountName             string                  `yaml:"AwsAccountName"`
	PublicReadAccess           bool                    `yaml:"PublicReadAccess"`
	WorkloadServiceAccountName string                  `yaml:"WorkloadServiceAccountName"`
	WorkloadBucketEnvVar       string                  `yaml:"WorkloadBucketEnvVar"`
	WorkloadInstance           *WorkloadInstanceValues `yaml:"WorkloadInstance"`
}

// AwsObjectStorageBucketDefinitionConfig contains the config for an AWS
// S3 bucket definition.
type AwsObjectStorageBucketDefinitionConfig struct {
	AwsObjectStorageBucketDefinition AwsObjectStorageBucketDefinitionValues `yaml:"AwsObjectStorageBucketDefinition"`
}

// AwsObjectStorageBucketDefinitionValues contains the attributes needed to
// configure an AWS S3 bucket.
type AwsObjectStorageBucketDefinitionValues struct {
	Name                       string `yaml:"Name"`
	AwsAccountName             string `yaml:"AwsAccountName"`
	PublicReadAccess           bool   `yaml:"PublicReadAccess"`
	WorkloadServiceAccountName string `yaml:"WorkloadServiceAccountName"`
	WorkloadBucketEnvVar       string `yaml:"WorkloadBucketEnvVar"`
}

// AwsObjectStorageBucketInstanceConfig contains the config for an AWS S3 bucket
// instance.
type AwsObjectStorageBucketInstanceConfig struct {
	AwsObjectStorageBucketInstance AwsObjectStorageBucketInstanceValues `yaml:"AwsObjectStorageBucketInstance"`
}

// AwsObjectStorageBucketInstanceValues contains the attributes needed to
// create an AWS S3 instance.
type AwsObjectStorageBucketInstanceValues struct {
	Name                             string                                 `yaml:"Name"`
	AwsObjectStorageBucketDefinition AwsObjectStorageBucketDefinitionValues `yaml:"AwsObjectStorageBucketDefinition"`
	WorkloadInstance                 WorkloadInstanceValues                 `yaml:"WorkloadInstance"`
}

// Create creates an AWS account in the Threeport API.
func (a *AwsAccountValues) Create(apiClient *http.Client, apiEndpoint string) (*v0.AwsAccount, error) {
	// validate required fields
	if a.Name == "" || a.AccountID == "" {
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
	if a.LocalConfig != "" && a.LocalCredentials != "" && a.LocalProfile != "" {
		localConfig = true
	}
	if a.DefaultRegion != "" && a.AccessKeyID != "" && a.SecretAccessKey != "" {
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
	if a.DefaultAccount {
		existingAccounts, err := client.GetAwsAccounts(apiClient, apiEndpoint)
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve existing AWS accounts to check default accounts: %w", err)
		}
		for _, existing := range *existingAccounts {
			if *existing.DefaultAccount {
				msg := fmt.Sprintf("cannot designate new account as default account - %s is already the default account", *existing.Name)
				return nil, errors.New(msg)
			}
		}
	}

	// establish default region from explicit declaration in config or AWS
	// config file
	var region string
	if a.DefaultRegion == "" {
		awsConfig, err := ini.Load(a.LocalConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to load aws config: %w", err)
		}
		if awsConfig.Section(a.LocalProfile).HasKey("region") {
			region = awsConfig.Section(a.LocalProfile).Key("region").String()
		} else {
			return nil, errors.New(
				fmt.Sprintf("profile %s not found in aws config %s", a.LocalProfile, a.LocalConfig),
			)
		}
	} else {
		region = a.DefaultRegion
	}

	// retrieve access key ID and secret access key if needed
	var accessKeyID string
	var secretAccessKey string
	if a.AccessKeyID == "" && a.SecretAccessKey == "" {
		awsCredentials, err := ini.Load(a.LocalCredentials)
		if err != nil {
			return nil, fmt.Errorf("failed to load aws credentials: %w", err)
		}
		if awsCredentials.Section(a.LocalProfile).HasKey("aws_access_key_id") &&
			awsCredentials.Section(a.LocalProfile).HasKey("aws_secret_access_key") {
			accessKeyID = awsCredentials.Section(a.LocalProfile).Key("aws_access_key_id").String()
			secretAccessKey = awsCredentials.Section(a.LocalProfile).Key("aws_secret_access_key").String()
		}
	} else {
		accessKeyID = a.AccessKeyID
		secretAccessKey = a.SecretAccessKey
	}

	// construct AWS account object
	awsAccount := v0.AwsAccount{
		Name:            &a.Name,
		DefaultAccount:  &a.DefaultAccount,
		DefaultRegion:   &region,
		AccountID:       &a.AccountID,
		AccessKeyID:     &accessKeyID,
		SecretAccessKey: &secretAccessKey,
		RoleArn:         &a.RoleArn,
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
	// get AWS account by name
	awsAccount, err := client.GetAwsAccountByName(apiClient, apiEndpoint, a.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to find AWS Account with name %s: %w", a.Name, err)
	}

	// get AWS account status
	statusDetail, err := status.GetAwsAccountStatus(
		apiClient,
		apiEndpoint,
		*awsAccount.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get status for AWS account with name %s: %w", a.Name, err)
	}

	return statusDetail, nil
}

// Delete deletes a AWS account from the Threeport API.
func (a *AwsAccountValues) Delete(apiClient *http.Client, apiEndpoint string) (*v0.AwsAccount, error) {
	// get AWS account by name
	awsAccount, err := client.GetAwsAccountByName(apiClient, apiEndpoint, a.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to find AWS account with name %s: %w", a.Name, err)
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
			w.Name,
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
			w.Name,
			err,
		)
	}

	return nil, nil, nil
}

// Create creates an AWS EKS kubernetes runtime definition in the threeport API.
func (e *AwsEksKubernetesRuntimeDefinitionValues) Create(apiClient *http.Client, apiEndpoint string) (*v0.AwsEksKubernetesRuntimeDefinition, error) {
	// validate required fields
	if e.Name == "" || e.AwsAccountName == "" || e.ZoneCount == 0 ||
		e.DefaultNodeGroupInstanceType == "" || e.DefaultNodeGroupInitialSize == 0 ||
		e.DefaultNodeGroupMinimumSize == 0 || e.DefaultNodeGroupMaximumSize == 0 {
		return nil, errors.New("missing required field/s in config - required fields: Name, AwsAccountName, ZoneCount, DefaultNodeGroupInstanceType, DefaultNodeGroupInitialSize, DefaultNodeGroupMinimumSize, DefaultNodeGroupMaximumSize")
	}

	// look up AWS account by name
	awsAccount, err := client.GetAwsAccountByName(apiClient, apiEndpoint, e.AwsAccountName)
	if err != nil {
		return nil, fmt.Errorf("failed to find AWS account with name %s: %w", e.Name, err)
	}

	// construct kubernetes runtime definition
	infraProvider := v0.KubernetesRuntimeInfraProviderEKS
	kubernetesRuntimeDefinition := v0.KubernetesRuntimeDefinition{
		Definition: v0.Definition{
			Name: &e.Name,
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
			Name: &e.Name,
		},
		AwsAccountID:                  awsAccount.ID,
		ZoneCount:                     &e.ZoneCount,
		DefaultNodeGroupInstanceType:  &e.DefaultNodeGroupInstanceType,
		DefaultNodeGroupInitialSize:   &e.DefaultNodeGroupInitialSize,
		DefaultNodeGroupMinimumSize:   &e.DefaultNodeGroupMinimumSize,
		DefaultNodeGroupMaximumSize:   &e.DefaultNodeGroupMaximumSize,
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
	// get AWS EKS kubernetes runtime definition by name
	awsEksKubernetesRuntimeDefinition, err := client.GetAwsEksKubernetesRuntimeDefinitionByName(
		apiClient,
		apiEndpoint,
		e.Name,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to find AWS EKS kubernetes runtime definition with name %s: %w", e.Name, err)
	}

	// get AWS EKS kubernetes runtime definition status
	statusDetail, err := status.GetAwsEksKubernetesRuntimeDefinitionStatus(
		apiClient,
		apiEndpoint,
		awsEksKubernetesRuntimeDefinition,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get status for AWS EKS kubernetes runtime definition with name %s: %w", e.Name, err)
	}

	return statusDetail, nil
}

// Delete deletes an AWS EKS kubernetes definition from the Threeport API.
func (e *AwsEksKubernetesRuntimeDefinitionValues) Delete(apiClient *http.Client, apiEndpoint string) (*v0.AwsEksKubernetesRuntimeDefinition, error) {
	// get AWS EKS kubernetes definition by name
	awsEksKubernetesRuntimeDefinition, err := client.GetAwsEksKubernetesRuntimeDefinitionByName(apiClient, apiEndpoint, e.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to find AWS EKS kubernetes definition with name %s: %w", e.Name, err)
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
	if e.Name == "" || e.AwsEksKubernetesRuntimeDefinition.Name == "" {
		return nil, errors.New("missing required field/s in config - required fields: Name, AwsEksKubernetesRuntimeDefinitionName")
	}

	// look up AWS EKS kubernetes runtime definition by name
	awsEksKubernetesRuntimeDefinition, err := client.GetAwsEksKubernetesRuntimeDefinitionByName(apiClient, apiEndpoint, e.AwsEksKubernetesRuntimeDefinition.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to find AWS EKS kubernetes runtime definition with name %s: %w", e.AwsEksKubernetesRuntimeDefinition.Name, err)
	}

	// get location for provider AWS region
	location, err := mapping.GetLocationForAwsRegion(e.Region)
	if err != nil {
		return nil, fmt.Errorf("failed to get Threeport location for AWS region %s: %w", e.Region, err)
	}

	// construct kubernetes runtime instance object
	controlPlaneHost := false
	defaultRuntime := false
	kubernetesRuntimeInstance := v0.KubernetesRuntimeInstance{
		Instance: v0.Instance{
			Name: &e.Name,
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
			Name: &e.Name,
		},
		Region:                              &e.Region,
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
	// get AWS EKS kubernetes runtime instance by name
	awsEksKubernetesRuntimeInstance, err := client.GetAwsEksKubernetesRuntimeInstanceByName(
		apiClient,
		apiEndpoint,
		e.Name,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to find AWS EKS kubernetes runtime instance with name %s: %w", e.Name, err)
	}

	// get AWS EKS kubernetes runtime instance status
	statusDetail, err := status.GetAwsEksKubernetesRuntimeInstanceStatus(
		apiClient,
		apiEndpoint,
		awsEksKubernetesRuntimeInstance,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get status for AWS EKS kubernetes runtime instance with name %s: %w", e.Name, err)
	}

	return statusDetail, nil
}

// Delete deletes an AWS EKS kubernetes runtime instance from the Threeport API.
func (e *AwsEksKubernetesRuntimeInstanceValues) Delete(apiClient *http.Client, apiEndpoint string) (*v0.AwsEksKubernetesRuntimeInstance, error) {
	// get AWS EKS kubernetes runtime instance by name
	awsEksKubernetesRuntimeInstance, err := client.GetAwsEksKubernetesRuntimeInstanceByName(apiClient, apiEndpoint, e.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to find AWS EKS kubernetes runtime instance with name %s: %w", e.Name, err)
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
		// triggering unecessary reconciliation
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

// Create creates an AWS relational database definition and instance in the
// threeport API.
func (r *AwsRelationalDatabaseValues) Create(apiClient *http.Client, apiEndpoint string) (*v0.AwsRelationalDatabaseDefinition, *v0.AwsRelationalDatabaseInstance, error) {
	// create the relational database definition
	awsRelationalDatabaseDefinition := AwsRelationalDatabaseDefinitionValues{
		Name:               r.Name,
		AwsAccountName:     r.AwsAccountName,
		Engine:             r.Engine,
		EngineVersion:      r.EngineVersion,
		DatabaseName:       r.DatabaseName,
		DatabasePort:       r.DatabasePort,
		BackupDays:         r.BackupDays,
		MachineSize:        r.MachineSize,
		StorageGb:          r.StorageGb,
		WorkloadSecretName: r.WorkloadSecretName,
	}
	createdAwsRelationalDatabaseDefinition, err := awsRelationalDatabaseDefinition.Create(apiClient, apiEndpoint)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create AWS relational database definition: %w", err)
	}

	// create the relational database instance
	awsRelationalDatabaseInstance := AwsRelationalDatabaseInstanceValues{
		Name: r.Name,
		AwsRelationalDatabaseDefinition: AwsRelationalDatabaseDefinitionValues{
			Name: r.Name,
		},
		WorkloadInstance: *r.WorkloadInstance,
	}
	createdAwsRelationalDatabaseInstance, err := awsRelationalDatabaseInstance.Create(apiClient, apiEndpoint)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create AWS relational database instance: %w", err)
	}

	return createdAwsRelationalDatabaseDefinition, createdAwsRelationalDatabaseInstance, nil
}

// Delete deletes an AWS EKS relational database definition and instance from
// the threeport API.
func (r *AwsRelationalDatabaseValues) Delete(apiClient *http.Client, apiEndpoint string) (*v0.AwsRelationalDatabaseDefinition, *v0.AwsRelationalDatabaseInstance, error) {
	// get AWS relational database definition by name
	awsRelationalDatabaseDefinition, err := client.GetAwsRelationalDatabaseDefinitionByName(apiClient, apiEndpoint, r.Name)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to find AWS relational database definition by name %s: %w", r.Name, err)
	}

	// get AWS relational database instance by name
	awsRelationalDatabaseInstName := r.Name
	awsRelationalDatabaseInstance, err := client.GetAwsRelationalDatabaseInstanceByName(apiClient, apiEndpoint, awsRelationalDatabaseInstName)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to find AWS relational database instance by name %s: %w", r.Name, err)
	}

	// ensure the AWS relational database definition has no more than one
	// associated instance
	queryString := fmt.Sprintf("awsrelationaldatabasedefinitionid=%d", *awsRelationalDatabaseDefinition.ID)
	awsRelationalDatabaseInsts, err := client.GetAwsRelationalDatabaseInstancesByQueryString(apiClient, apiEndpoint, queryString)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get AWS relational database instances by AWS relational database definition with ID: %d: %w", *awsRelationalDatabaseDefinition.ID, err)
	}
	if len(*awsRelationalDatabaseInsts) > 1 {
		err = errors.New("deletion using the AWS relational database abstraction is only permitted when there is a one-to-one AWS relational database defintion and instance relationship")
		return nil, nil, fmt.Errorf("the AWS relational database definition has more than one instance associated: %w", err)
	}

	// delete AWS relational database instance
	deletedAwsRelationalDatabaseInstance, err := client.DeleteAwsRelationalDatabaseInstance(apiClient, apiEndpoint, *awsRelationalDatabaseInstance.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to delete AWS relational database instance from threeport API: %w", err)
	}

	// wait for AWS relational database instance to be reconciled
	deletedCheckAttempts := 0
	deletedCheckAttemptsMax := 60
	deletedCheckDurationSeconds := 15
	awsRelationalDatabaseInstanceDeleted := false
	for deletedCheckAttempts < deletedCheckAttemptsMax {
		_, err := client.GetAwsRelationalDatabaseInstanceByID(apiClient, apiEndpoint, *awsRelationalDatabaseInstance.ID)
		if err != nil {
			if errors.Is(err, client_lib.ErrObjectNotFound) {
				awsRelationalDatabaseInstanceDeleted = true
				break
			} else {
				return nil, nil, fmt.Errorf("failed to get AWS relational database instance from API when checking deletion: %w", err)
			}
		}
		// no error means AWS relational database instance was found - hasn't yet been deleted
		deletedCheckAttempts += 1
		time.Sleep(time.Second * time.Duration(deletedCheckDurationSeconds))
	}
	if !awsRelationalDatabaseInstanceDeleted {
		return nil, nil, errors.New(fmt.Sprintf(
			"AWS relational database instance not deleted after %d seconds",
			deletedCheckAttemptsMax*deletedCheckDurationSeconds,
		))
	}

	// delete AWS relational database definition
	deletedAwsRelationalDatabaseDefinition, err := client.DeleteAwsRelationalDatabaseDefinition(apiClient, apiEndpoint, *awsRelationalDatabaseDefinition.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to delete AWS relational database definition from threeport API: %w", err)
	}

	return deletedAwsRelationalDatabaseDefinition, deletedAwsRelationalDatabaseInstance, nil
}

// Create creates an AWS relational database definition in the threeport API.
func (r *AwsRelationalDatabaseDefinitionValues) Create(apiClient *http.Client, apiEndpoint string) (*v0.AwsRelationalDatabaseDefinition, error) {
	// validate required fields
	if r.Name == "" || r.Engine == "" || r.EngineVersion == "" || r.DatabaseName == "" ||
		r.DatabasePort == 0 || r.MachineSize == "" || r.StorageGb == 0 ||
		r.WorkloadSecretName == "" || r.AwsAccountName == "" {
		return nil, errors.New("missing required field/s in config - required fields: Name, Engine, EngineVersion, DatabaseName, DatabasePort, MachineSize, StorageGb, WorkloadSecretName, AwsAccountName")
	}

	// look up AWS account by name
	awsAccount, err := client.GetAwsAccountByName(apiClient, apiEndpoint, r.AwsAccountName)
	if err != nil {
		return nil, fmt.Errorf("failed to find AWS account with name %s: %w", r.Name, err)
	}

	// construct AWS relational database definition object
	awsRelationalDatabaseDefinition := v0.AwsRelationalDatabaseDefinition{
		Definition: v0.Definition{
			Name: &r.Name,
		},
		Engine:             &r.Engine,
		EngineVersion:      &r.EngineVersion,
		DatabaseName:       &r.DatabaseName,
		DatabasePort:       &r.DatabasePort,
		MachineSize:        &r.MachineSize,
		StorageGb:          &r.StorageGb,
		WorkloadSecretName: &r.WorkloadSecretName,
		AwsAccountID:       awsAccount.ID,
	}

	// create AWS relational database definition
	createdAwsRelationalDatabaseDefinition, err := client.CreateAwsRelationalDatabaseDefinition(apiClient, apiEndpoint, &awsRelationalDatabaseDefinition)
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS relational database definition in threeport API: %w", err)
	}

	return createdAwsRelationalDatabaseDefinition, nil
}

// Delete deletes an AWS relational database definition from the threeport API.
func (r *AwsRelationalDatabaseDefinitionValues) Delete(apiClient *http.Client, apiEndpoint string) (*v0.AwsRelationalDatabaseDefinition, error) {
	// get AWS relational database definition by name
	awsRelationalDatabaseDefinition, err := client.GetAwsRelationalDatabaseDefinitionByName(apiClient, apiEndpoint, r.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to find AWS relational database definition by name %s: %w", r.Name, err)
	}

	// delete AWS relational database definition
	deletedAwsRelationalDatabaseDefinition, err := client.DeleteAwsRelationalDatabaseDefinition(apiClient, apiEndpoint, *awsRelationalDatabaseDefinition.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to delete AWS relational database definition from threeport API: %w", err)
	}

	return deletedAwsRelationalDatabaseDefinition, nil
}

// Create creates an AWS relational database instance in the threeport API.
func (r *AwsRelationalDatabaseInstanceValues) Create(apiClient *http.Client, apiEndpoint string) (*v0.AwsRelationalDatabaseInstance, error) {
	// validate required fields
	if r.Name == "" || r.AwsRelationalDatabaseDefinition.Name == "" || r.WorkloadInstance.Name == "" {
		return nil, errors.New("missing required fields in config - required fields: Name, AwsRelationalDatabaseDefinition.Name, WorkloadInstance.Name")
	}

	// get AWS relational database definition by name
	awsRelationalDatabaseDefinition, err := client.GetAwsRelationalDatabaseDefinitionByName(
		apiClient,
		apiEndpoint,
		r.AwsRelationalDatabaseDefinition.Name,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to find AWS relational database definition by name %s: %w", r.AwsRelationalDatabaseDefinition.Name, err)
	}

	// get workload instance by name
	workloadInstance, err := client.GetWorkloadInstanceByName(
		apiClient,
		apiEndpoint,
		r.WorkloadInstance.Name,
	)
	if err != nil {
		return nil, fmt.Errorf("failed find workload instance by name %s: %w", r.WorkloadInstance.Name, err)
	}

	// construct AWS relational database instance object
	awsRelationalDatabaseInstance := v0.AwsRelationalDatabaseInstance{
		Instance: v0.Instance{
			Name: &r.Name,
		},
		AwsRelationalDatabaseDefinitionID: awsRelationalDatabaseDefinition.ID,
		WorkloadInstanceID:                workloadInstance.ID,
	}

	// create AWS relational database instance
	createdAwsRelationalDatabaseInstance, err := client.CreateAwsRelationalDatabaseInstance(apiClient, apiEndpoint, &awsRelationalDatabaseInstance)
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS relational database instance in threeport API: %w", err)
	}

	return createdAwsRelationalDatabaseInstance, nil
}

// Delete deletes an AWS relational database instance from the threeport API.
func (r *AwsRelationalDatabaseInstanceValues) Delete(apiClient *http.Client, apiEndpoint string) (*v0.AwsRelationalDatabaseInstance, error) {
	// get AWS relational database instance by name
	awsRelationalDatabaseInstance, err := client.GetAwsRelationalDatabaseInstanceByName(apiClient, apiEndpoint, r.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to find AWS relational database instance by name %s: %w", r.Name, err)
	}

	// delete AWS relational database instance
	deletedAwsRelationalDatabaseInstance, err := client.DeleteAwsRelationalDatabaseInstance(apiClient, apiEndpoint, *awsRelationalDatabaseInstance.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to delete AWS relational database instance from threeport API: %w", err)
	}

	return deletedAwsRelationalDatabaseInstance, nil
}

// Create creates an AWS object storage bucket definition and instance in the
// threeport API.
func (o *AwsObjectStorageBucketValues) Create(apiClient *http.Client, apiEndpoint string) (*v0.AwsObjectStorageBucketDefinition, *v0.AwsObjectStorageBucketInstance, error) {
	// create the object storage bucket definition
	awsObjectStorageBucketDefinition := AwsObjectStorageBucketDefinitionValues{
		Name:                       o.Name,
		AwsAccountName:             o.AwsAccountName,
		PublicReadAccess:           o.PublicReadAccess,
		WorkloadServiceAccountName: o.WorkloadServiceAccountName,
		WorkloadBucketEnvVar:       o.WorkloadBucketEnvVar,
	}
	createdAwsObjectStorageBucketDefinition, err := awsObjectStorageBucketDefinition.Create(apiClient, apiEndpoint)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create AWS object storage bucket definition: %w", err)
	}

	// create the object storage bucket instance
	awsObjectStorageBucketInstance := AwsObjectStorageBucketInstanceValues{
		Name: o.Name,
		AwsObjectStorageBucketDefinition: AwsObjectStorageBucketDefinitionValues{
			Name: o.Name,
		},
		WorkloadInstance: *o.WorkloadInstance,
	}
	createdAwsObjectStorageBucketInstance, err := awsObjectStorageBucketInstance.Create(apiClient, apiEndpoint)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create AWS object storage bucket instance: %w", err)
	}

	return createdAwsObjectStorageBucketDefinition, createdAwsObjectStorageBucketInstance, nil
}

// Delete deletes an AWS object storage bucket defintiion and instance from the
// threeport API.
func (o *AwsObjectStorageBucketValues) Delete(apiClient *http.Client, apiEndpoint string) (*v0.AwsObjectStorageBucketDefinition, *v0.AwsObjectStorageBucketInstance, error) {
	// get AWS object storage bucket definition by name
	awsObjectStorageBucketDefinition, err := client.GetAwsObjectStorageBucketDefinitionByName(apiClient, apiEndpoint, o.Name)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to find AWS object storage bucket definition by name %s: %w", o.Name, err)
	}

	// get AWS object storage bucket instance by name
	awsObjectStorageBucketInstName := o.Name
	awsObjectStorageBucketInstance, err := client.GetAwsObjectStorageBucketInstanceByName(apiClient, apiEndpoint, awsObjectStorageBucketInstName)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to find AWS object storage bucket instance by name %s: %w", o.Name, err)
	}

	// ensure the AWS object storage bucket definition has no more than one
	// associated instance
	queryString := fmt.Sprintf("awsobjectstoragebucketdefinitionid=%d", *awsObjectStorageBucketDefinition.ID)
	awsObjectStorageBucketInsts, err := client.GetAwsObjectStorageBucketInstancesByQueryString(apiClient, apiEndpoint, queryString)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get AWS object storage bucket instances by AWS object storage bucket definition with ID: %d: %w", *awsObjectStorageBucketDefinition.ID, err)
	}
	if len(*awsObjectStorageBucketInsts) > 1 {
		err = errors.New("deletion using the AWS object storage bucket abstraction is only permitted when there is a one-to-one AWS object storage bucket defintion and instance relationship")
		return nil, nil, fmt.Errorf("the AWS object storage bucket definition has more than one instance associated: %w", err)
	}

	// delete AWS object storage bucket instance
	deletedAwsObjectStorageBucketInstance, err := client.DeleteAwsObjectStorageBucketInstance(apiClient, apiEndpoint, *awsObjectStorageBucketInstance.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to delete AWS object storage bucket instance from threeport API: %w", err)
	}

	// wait for AWS object storage bucket instance to be reconciled
	deletedCheckAttempts := 0
	deletedCheckAttemptsMax := 12
	deletedCheckDurationSeconds := 5
	awsObjectStorageBucketInstanceDeleted := false
	for deletedCheckAttempts < deletedCheckAttemptsMax {
		_, err := client.GetAwsObjectStorageBucketInstanceByID(apiClient, apiEndpoint, *awsObjectStorageBucketInstance.ID)
		if err != nil {
			if errors.Is(err, client_lib.ErrObjectNotFound) {
				awsObjectStorageBucketInstanceDeleted = true
				break
			} else {
				return nil, nil, fmt.Errorf("failed to get AWS object storage bucket instance from API when checking deletion: %w", err)
			}
		}
		// no error means AWS object storage bucket instance was found - hasn't yet been deleted
		deletedCheckAttempts += 1
		time.Sleep(time.Second * time.Duration(deletedCheckDurationSeconds))
	}
	if !awsObjectStorageBucketInstanceDeleted {
		return nil, nil, errors.New(fmt.Sprintf(
			"AWS object storage bucket instance not deleted after %d seconds",
			deletedCheckAttemptsMax*deletedCheckDurationSeconds,
		))
	}

	// delete AWS object storage bucket definition
	deletedAwsObjectStorageBucketDefinition, err := client.DeleteAwsObjectStorageBucketDefinition(apiClient, apiEndpoint, *awsObjectStorageBucketDefinition.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to delete AWS object storage bucket definition from threeport API: %w", err)
	}

	return deletedAwsObjectStorageBucketDefinition, deletedAwsObjectStorageBucketInstance, nil
}

// Create creates an AWS object storage bucket definition in the threeport API.
func (o *AwsObjectStorageBucketDefinitionValues) Create(apiClient *http.Client, apiEndpoint string) (*v0.AwsObjectStorageBucketDefinition, error) {
	// validate required fields
	if o.Name == "" || o.WorkloadServiceAccountName == "" || o.WorkloadBucketEnvVar == "" || o.AwsAccountName == "" {
		return nil, errors.New("missing required field/s in config - required fields: Name, WorkloadServiceAccountName, WorkloadBucketEnvVar, AwsAccountName")
	}

	// look up AWS account by name
	awsAccount, err := client.GetAwsAccountByName(apiClient, apiEndpoint, o.AwsAccountName)
	if err != nil {
		return nil, fmt.Errorf("failed to find AWS account with name %s: %w", o.Name, err)
	}

	// construct AWS object storage bucket definition object
	awsObjectStorageBucketDefinition := v0.AwsObjectStorageBucketDefinition{
		Definition: v0.Definition{
			Name: &o.Name,
		},
		PublicReadAccess:           &o.PublicReadAccess,
		WorkloadServiceAccountName: &o.WorkloadServiceAccountName,
		WorkloadBucketEnvVar:       &o.WorkloadBucketEnvVar,
		AwsAccountID:               awsAccount.ID,
	}

	// create AWS object storage bucket definition
	createdAwsObjectStorageBucketDefinition, err := client.CreateAwsObjectStorageBucketDefinition(apiClient, apiEndpoint, &awsObjectStorageBucketDefinition)
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS object storage bucket definition in threeport API: %w", err)
	}

	return createdAwsObjectStorageBucketDefinition, nil
}

// Describe returns details related to an AWS object storage bucket definition.
func (e *AwsObjectStorageBucketDefinitionValues) Describe(
	apiClient *http.Client,
	apiEndpoint string,
) (*status.AwsObjectStorageBucketDefinitionStatusDetail, error) {
	// get AWS object storage bucket definition by name
	awsObjectStorageBucketDefinition, err := client.GetAwsObjectStorageBucketDefinitionByName(
		apiClient,
		apiEndpoint,
		e.Name,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to find AWS EKS kubernetes runtime instance with name %s: %w", e.Name, err)
	}

	// get AWS object storage bucket definition status
	statusDetail, err := status.GetAwsObjectStorageBucketDefinitionStatus(
		apiClient,
		apiEndpoint,
		awsObjectStorageBucketDefinition,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get status for AWS EKS kubernetes runtime instance with name %s: %w", e.Name, err)
	}

	return statusDetail, nil
}

// Delete deletes an AWS object storage bucket definition from the threeport
// API.
func (o *AwsObjectStorageBucketDefinitionValues) Delete(apiClient *http.Client, apiEndpoint string) (*v0.AwsObjectStorageBucketDefinition, error) {
	// get AWS object storage bucket definition by name
	awsObjectStorageBucketDefinition, err := client.GetAwsObjectStorageBucketDefinitionByName(apiClient, apiEndpoint, o.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to find AWS object storage bucket definition by name %s: %w", o.Name, err)
	}

	// delete AWS object storage bucket definition
	deletedAwsObjectStorageBucketDefinition, err := client.DeleteAwsObjectStorageBucketDefinition(apiClient, apiEndpoint, *awsObjectStorageBucketDefinition.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to delete AWS object storage bucket definition from threeport API: %w", err)
	}

	return deletedAwsObjectStorageBucketDefinition, nil
}

// Create creates and AWS object storage bucket instance in the threeport API.
func (o *AwsObjectStorageBucketInstanceValues) Create(apiClient *http.Client, apiEndpoint string) (*v0.AwsObjectStorageBucketInstance, error) {
	// validate required fields
	if o.Name == "" || o.AwsObjectStorageBucketDefinition.Name == "" || o.WorkloadInstance.Name == "" {
		return nil, errors.New("missing required fields in config - required fields: Name, AwsObjectStorageBucketDefinition.Name, WorkloadInstance.Name")
	}

	// get AWS object storage bucket definition by name
	awsObjectStorageBucketDefinition, err := client.GetAwsObjectStorageBucketDefinitionByName(
		apiClient,
		apiEndpoint,
		o.AwsObjectStorageBucketDefinition.Name,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to find AWS object storage bucket definition by name %s: %w", o.AwsObjectStorageBucketDefinition.Name, err)
	}

	// get workload instance by name
	workloadInstance, err := client.GetWorkloadInstanceByName(
		apiClient,
		apiEndpoint,
		o.WorkloadInstance.Name,
	)
	if err != nil {
		return nil, fmt.Errorf("failed find workload instance by name %s: %w", o.WorkloadInstance.Name, err)
	}

	// construct AWS object storage bucket instance object
	awsObjectStorageBucketInstance := v0.AwsObjectStorageBucketInstance{
		Instance: v0.Instance{
			Name: &o.Name,
		},
		AwsObjectStorageBucketDefinitionID: awsObjectStorageBucketDefinition.ID,
		WorkloadInstanceID:                 workloadInstance.ID,
	}

	// create AWS object storage bucket instance
	createdAwsObjectStorageBucketInstance, err := client.CreateAwsObjectStorageBucketInstance(apiClient, apiEndpoint, &awsObjectStorageBucketInstance)
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS object storage bucket instance in threeport API: %w", err)
	}

	return createdAwsObjectStorageBucketInstance, nil
}

// Delete deletes an AWS object storage bucket instance from the threeport API.
func (o *AwsObjectStorageBucketInstanceValues) Delete(apiClient *http.Client, apiEndpoint string) (*v0.AwsObjectStorageBucketInstance, error) {
	// get AWS object storage bucket instance by name
	awsObjectStorageBucketInstance, err := client.GetAwsObjectStorageBucketInstanceByName(apiClient, apiEndpoint, o.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to find AWS object storage bucket instance by name %s: %w", o.Name, err)
	}

	// delete AWS object storage bucket instance
	deletedAwsObjectStorageBucketInstance, err := client.DeleteAwsObjectStorageBucketInstance(apiClient, apiEndpoint, *awsObjectStorageBucketInstance.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to delete AWS object storage bucket instance from threeport API: %w", err)
	}

	return deletedAwsObjectStorageBucketInstance, nil
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
					"failed to create AWS EKS kubernetes runtime definitiona with name %s: %w",
					awsEksKubernetesRuntimeDefinitionValues.Name,
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
					"failed to delete AWS EKS kubernetes runtime definitiona with name %s: %w",
					awsEksKubernetesRuntimeDefinitionValues.Name,
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
		AwsEksKubernetesRuntimeDefinition: AwsEksKubernetesRuntimeDefinitionValues{
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
					awsEksKubernetesRuntimeInstanceValues.Name,
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
					"failed to delete AWS EKS kubernetes runtime instance: with name %s: %w",
					awsEksKubernetesRuntimeInstanceValues.Name,
					err,
				)
			}
			return nil
		},
	})

	return &operations, &createdAwsEksKubernetesRuntimeDefinition, &createdAwsEksKubernetesRuntimeInstance
}
