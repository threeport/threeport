package v0

import (
	"errors"
	"fmt"
	"net/http"

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
