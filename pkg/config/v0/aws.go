package v0

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"gopkg.in/ini.v1"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
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
	LocalConfig      string `yaml:"LocalConfig"`
	LocalCredentials string `yaml:"LocalCredentials"`
	LocalProfile     string `yaml:"LocalProfile"`
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
	Name                                  string `yaml:"Name"`
	Region                                string `yaml:"Region"`
	AwsEksKubernetesRuntimeDefinitionName string `yaml:"AwsEksKubernetesRuntimeDefinitionName"`
}

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

// Create creates an AWS account in the Threeport API.
func (aa *AwsAccountValues) Create(apiClient *http.Client, apiEndpoint string) (*v0.AwsAccount, error) {
	// validate required fields
	if aa.Name == "" || aa.AccountID == "" {
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
	if aa.LocalConfig != "" && aa.LocalCredentials != "" && aa.LocalProfile != "" {
		localConfig = true
	}
	if aa.DefaultRegion != "" && aa.AccessKeyID != "" && aa.SecretAccessKey != "" {
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
	if aa.DefaultAccount {
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
	if aa.DefaultRegion == "" {
		awsConfig, err := ini.Load(aa.LocalConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to load aws config: %w", err)
		}
		if awsConfig.Section(aa.LocalProfile).HasKey("region") {
			region = awsConfig.Section(aa.LocalProfile).Key("region").String()
		} else {
			return nil, errors.New(
				fmt.Sprintf("profile %s not found in aws config %s", aa.LocalProfile, aa.LocalConfig),
			)
		}
	} else {
		region = aa.DefaultRegion
	}

	// retrieve access key ID and secret access key if needed
	var accessKeyID string
	var secretAccessKey string
	if aa.AccessKeyID == "" && aa.SecretAccessKey == "" {
		awsCredentials, err := ini.Load(aa.LocalCredentials)
		if err != nil {
			return nil, fmt.Errorf("failed to load aws credentials: %w", err)
		}
		if awsCredentials.Section(aa.LocalProfile).HasKey("aws_access_key_id") &&
			awsCredentials.Section(aa.LocalProfile).HasKey("aws_secret_access_key") {
			accessKeyID = awsCredentials.Section(aa.LocalProfile).Key("aws_access_key_id").String()
			secretAccessKey = awsCredentials.Section(aa.LocalProfile).Key("aws_secret_access_key").String()
		}
	} else {
		accessKeyID = aa.AccessKeyID
		secretAccessKey = aa.SecretAccessKey
	}

	// construct AWS account object
	awsAccount := v0.AwsAccount{
		Name:            &aa.Name,
		DefaultAccount:  &aa.DefaultAccount,
		DefaultRegion:   &region,
		AccountID:       &aa.AccountID,
		AccessKeyID:     &accessKeyID,
		SecretAccessKey: &secretAccessKey,
	}

	// create AWS account
	createdAwsAccount, err := client.CreateAwsAccount(apiClient, apiEndpoint, &awsAccount)
	if err != nil {
		return nil, fmt.Errorf("failed to create aws account in threeport API: %w", err)
	}

	return createdAwsAccount, nil
}

// Delete deletes a AWS account from the Threeport API.
func (aa *AwsAccountValues) Delete(apiClient *http.Client, apiEndpoint string) (*v0.AwsAccount, error) {
	// get AWS account by name
	awsAccount, err := client.GetAwsAccountByName(apiClient, apiEndpoint, aa.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to find AWS account with name %s: %w", aa.Name, err)
	}

	// delete AWS account
	deletedAwsAccount, err := client.DeleteAwsAccount(apiClient, apiEndpoint, *awsAccount.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to delete AWS account from threeport API: %w", err)
	}

	return deletedAwsAccount, nil
}

func (aekrd *AwsEksKubernetesRuntimeDefinitionValues) Create(apiClient *http.Client, apiEndpoint string) (*v0.AwsEksKubernetesRuntimeDefinition, error) {
	// validate required fields
	if aekrd.Name == "" || aekrd.AwsAccountName == "" || aekrd.ZoneCount == 0 ||
		aekrd.DefaultNodeGroupInstanceType == "" || aekrd.DefaultNodeGroupInitialSize == 0 ||
		aekrd.DefaultNodeGroupMinimumSize == 0 || aekrd.DefaultNodeGroupMaximumSize == 0 {
		return nil, errors.New("missing required field/s in config - required fields: Name, AwsAccountName, ZoneCount, DefaultNodeGroupInstanceType, DefaultNodeGroupInitialSize, DefaultNodeGroupMinimumSize, DefaultNodeGroupMaximumSize")
	}

	// look up AWS account by name
	awsAccount, err := client.GetAwsAccountByName(apiClient, apiEndpoint, aekrd.AwsAccountName)
	if err != nil {
		return nil, fmt.Errorf("failed to find AWS account with name %s: %w", aekrd.Name, err)
	}

	// construct kubernetes runtime definition
	infraProvider := v0.KubernetesRuntimeInfraProviderEKS
	kubernetesRuntimeDefinition := v0.KubernetesRuntimeDefinition{
		Definition: v0.Definition{
			Name: &aekrd.Name,
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
			Name: &aekrd.Name,
		},
		AwsAccountID:                  awsAccount.ID,
		ZoneCount:                     &aekrd.ZoneCount,
		DefaultNodeGroupInstanceType:  &aekrd.DefaultNodeGroupInstanceType,
		DefaultNodeGroupInitialSize:   &aekrd.DefaultNodeGroupInitialSize,
		DefaultNodeGroupMinimumSize:   &aekrd.DefaultNodeGroupMinimumSize,
		DefaultNodeGroupMaximumSize:   &aekrd.DefaultNodeGroupMaximumSize,
		KubernetesRuntimeDefinitionID: createdKubernetesRuntimeDefinition.ID,
	}

	// create AWS EKS kubernetes definition
	createdAwsEksKubernetesRuntimeDefinition, err := client.CreateAwsEksKubernetesRuntimeDefinition(apiClient, apiEndpoint, &awsEksKubernetesRuntimeDefinition)
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS EKS kubernetes runtime definition in threeport API: %w", err)
	}

	return createdAwsEksKubernetesRuntimeDefinition, nil
}

// Delete deletes a AWS EKS kubernetes definition from the Threeport API.
func (aekrd *AwsEksKubernetesRuntimeDefinitionValues) Delete(apiClient *http.Client, apiEndpoint string) (*v0.AwsEksKubernetesRuntimeDefinition, error) {
	// get AWS EKS kubernetes definition by name
	awsEksKubernetesRuntimeDefinition, err := client.GetAwsEksKubernetesRuntimeDefinitionByName(apiClient, apiEndpoint, aekrd.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to find AWS EKS kubernetes definition with name %s: %w", aekrd.Name, err)
	}

	// delete AWS EKS kubernetes definition
	deletedAwsEksKubernetesRuntimeDefinition, err := client.DeleteAwsEksKubernetesRuntimeDefinition(apiClient, apiEndpoint, *awsEksKubernetesRuntimeDefinition.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to delete AWS EKS kubernetes definition from threeport API: %w", err)
	}

	return deletedAwsEksKubernetesRuntimeDefinition, nil
}

func (aekri *AwsEksKubernetesRuntimeInstanceValues) Create(apiClient *http.Client, apiEndpoint string) (*v0.AwsEksKubernetesRuntimeInstance, error) {
	// validate required fields
	if aekri.Name == "" || aekri.AwsEksKubernetesRuntimeDefinitionName == "" {
		return nil, errors.New("missing required field/s in config - required fields: Name, AwsEksKubernetesRuntimeDefinitionName")
	}

	// look up AWS EKS kubernetes runtime definition by name
	awsEksKubernetesRuntimeDefinition, err := client.GetAwsEksKubernetesRuntimeDefinitionByName(apiClient, apiEndpoint, aekri.AwsEksKubernetesRuntimeDefinitionName)
	if err != nil {
		return nil, fmt.Errorf("failed to find AWS EKS kubernetes runtime definition with name %s: %w", aekri.AwsEksKubernetesRuntimeDefinitionName, err)
	}

	// construct kubernetes runtime instance object
	controlPlaneHost := false
	defaultRuntime := false
	kubernetesRuntimeInstance := v0.KubernetesRuntimeInstance{
		Instance: v0.Instance{
			Name: &aekri.Name,
		},
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
			Name: &aekri.Name,
		},
		Region:                              &aekri.Region,
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

// Delete deletes a AWS EKS kubernetes runtime instance from the Threeport API.
func (aekri *AwsEksKubernetesRuntimeInstanceValues) Delete(apiClient *http.Client, apiEndpoint string) (*v0.AwsEksKubernetesRuntimeInstance, error) {
	// get AWS EKS kubernetes runtime instance by name
	awsEksKubernetesRuntimeInstance, err := client.GetAwsEksKubernetesRuntimeInstanceByName(apiClient, apiEndpoint, aekri.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to find AWS EKS kubernetes runtime instance with name %s: %w", aekri.Name, err)
	}

	// delete AWS EKS kubernetes runtime instance
	deletedAwsEksKubernetesRuntimeInstance, err := client.DeleteAwsEksKubernetesRuntimeInstance(apiClient, apiEndpoint, *awsEksKubernetesRuntimeInstance.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to delete AWS EKS kubernetes runtime instance from threeport API: %w", err)
	}

	return deletedAwsEksKubernetesRuntimeInstance, nil
}

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
		Name: defaultInstanceName(r.Name),
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

func (r *AwsRelationalDatabaseValues) Delete(apiClient *http.Client, apiEndpoint string) (*v0.AwsRelationalDatabaseDefinition, *v0.AwsRelationalDatabaseInstance, error) {
	// get AWS relational database definition by name
	awsRelationalDatabaseDefinition, err := client.GetAwsRelationalDatabaseDefinitionByName(apiClient, apiEndpoint, r.Name)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to find AWS relational database definition by name %s: %w", r.Name, err)
	}

	// get AWS relational database instance by name
	awsRelationalDatabaseInstName := defaultInstanceName(r.Name)
	awsRelationalDatabaseInstance, err := client.GetAwsRelationalDatabaseInstanceByName(apiClient, apiEndpoint, awsRelationalDatabaseInstName)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to find AWS relational database instance by name %s: %w", r.Name, err)
	}

	// ensure the AWS relational database definition has no more than one
	// associated instance
	awsRelationalDatabaseInsts, err := client.GetAwsRelationalDatabaseInstancesByAwsRelationalDatabaseDefinitionID(apiClient, apiEndpoint, *awsRelationalDatabaseDefinition.ID)
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
	deletedCheckAttemptsMax := 30
	deletedCheckDurationSeconds := 10
	awsRelationalDatabaseInstanceDeleted := false
	for deletedCheckAttempts < deletedCheckAttemptsMax {
		_, err := client.GetAwsRelationalDatabaseInstanceByID(apiClient, apiEndpoint, *awsRelationalDatabaseInstance.ID)
		if err != nil {
			if errors.Is(err, client.ErrorObjectNotFound) {
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
