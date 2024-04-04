package v0

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
	"gorm.io/datatypes"
)

// SecretConfig contains the configuration for a Secret
// object
type SecretConfig struct {
	Secret SecretValues `yaml:"Secret"`
}

// SecretValues contains the values for a Secret object
// configuration
type SecretValues struct {
	Name                      string                           `yaml:"Name"`
	Data                      map[string]string                `yaml:"Data"`
	AwsAccountName            string                           `yaml:"AwsAccountName"`
	SecretConfigPath          string                           `yaml:"SecretConfigPath"`
	WorkloadInstance          *WorkloadInstanceValues          `yaml:"WorkloadInstance"`
	HelmWorkloadInstance      *HelmWorkloadInstanceValues      `yaml:"HelmWorkloadInstance"`
	KubernetesRuntimeInstance *KubernetesRuntimeInstanceValues `yaml:"KubernetesRuntimeInstance"`
}

// SecretDefinitionConfig contains the configuration for a
// SecretDefinition object
type SecretDefinitionConfig struct {
	SecretDefinition SecretDefinitionValues `yaml:"SecretDefinition"`
}

// SecretDefinitionValues contains the values for a
// SecretDefinition object
type SecretDefinitionValues struct {
	Name             string            `yaml:"Name"`
	AwsAccountName   string            `yaml:"AwsAccountName"`
	Data             map[string]string `yaml:"Data"`
	SecretConfigPath string            `yaml:"SecretConfigPath"`
}

// SecretInstanceConfig contains the configuration for a
// SecretInstance object
type SecretInstanceConfig struct {
	SecretInstance SecretInstanceValues `yaml:"SecretInstance"`
}

// SecretInstanceValues contains the values for a
// SecretInstance object
type SecretInstanceValues struct {
	Name                      string                           `yaml:"Name"`
	SecretDefinition          *SecretDefinitionValues          `yaml:"SecretDefinition"`
	WorkloadInstance          *WorkloadInstanceValues          `yaml:"WorkloadInstance"`
	HelmWorkloadInstance      *HelmWorkloadInstanceValues      `yaml:"HelmWorkloadInstance"`
	KubernetesRuntimeInstance *KubernetesRuntimeInstanceValues `yaml:"KubernetesRuntimeInstance"`
	SecretConfigPath          string                           `yaml:"SecretConfigPath"`
}

// Create creates a Secret object
func (s *SecretValues) Create(
	apiClient *http.Client,
	apiEndpoint string,
) (*v0.SecretDefinition, *v0.SecretInstance, error) {
	// validate required fields
	if err := s.ValidateCreate(); err != nil {
		return nil, nil, fmt.Errorf("failed to validate secret values: %w", err)
	}

	// get operations
	operations, createdSecretDefinition, createdSecretInstance := s.GetOperations(
		apiClient,
		apiEndpoint,
	)

	// execute create operations
	if err := operations.Create(); err != nil {
		return nil, nil, fmt.Errorf("failed to create secret: %w", err)
	}

	return createdSecretDefinition, createdSecretInstance, nil
}

// GetOperations returns the operations to create and delete a Secret object
func (s *SecretValues) GetOperations(
	apiClient *http.Client,
	apiEndpoint string,
) (*util.Operations, *v0.SecretDefinition, *v0.SecretInstance) {
	var createdSecretDefinition v0.SecretDefinition
	var createdSecretInstance v0.SecretInstance

	operations := util.Operations{}

	secretDefinitionValues := SecretDefinitionValues{
		Name:           s.Name,
		AwsAccountName: s.AwsAccountName,
		Data:           s.Data,
	}
	operations.AppendOperation(util.Operation{
		Name: "secret definition",
		Create: func() error {
			secretDefinition, err := secretDefinitionValues.Create(apiClient, apiEndpoint)
			if err != nil {
				return fmt.Errorf("failed to create secret definition with name %s: %w", s.Name, err)
			}
			createdSecretDefinition = *secretDefinition
			return nil
		},
		Delete: func() error {
			_, err := secretDefinitionValues.Delete(apiClient, apiEndpoint)
			if err != nil {
				return fmt.Errorf("failed to delete secret definition with name %s: %w", s.Name, err)
			}
			return nil
		},
	})

	secretInstanceValues := SecretInstanceValues{
		Name:                      s.Name,
		SecretDefinition:          &secretDefinitionValues,
		KubernetesRuntimeInstance: s.KubernetesRuntimeInstance,
		WorkloadInstance:          s.WorkloadInstance,
		HelmWorkloadInstance:      s.HelmWorkloadInstance,
	}
	operations.AppendOperation(util.Operation{
		Name: "secret instance",
		Create: func() error {
			secretInstance, err := secretInstanceValues.Create(apiClient, apiEndpoint)
			if err != nil {
				return fmt.Errorf("failed to create secret instance with name %s: %w", s.Name, err)
			}
			createdSecretInstance = *secretInstance
			return nil
		},
		Delete: func() error {
			_, err := secretInstanceValues.Delete(apiClient, apiEndpoint)
			if err != nil {
				return fmt.Errorf("failed to delete secret instance with name %s: %w", s.Name, err)
			}
			return nil
		},
	})

	return &operations, &createdSecretDefinition, &createdSecretInstance
}

// Delete deletes a Secret object
func (s *SecretValues) Delete(
	apiClient *http.Client,
	apiEndpoint string,
) (*v0.SecretDefinition, *v0.SecretInstance, error) {
	// get operations
	operations, _, _ := s.GetOperations(
		apiClient,
		apiEndpoint,
	)

	// execute create operations
	if err := operations.Delete(); err != nil {
		return nil, nil, fmt.Errorf("failed to delete secret: %w", err)
	}

	return nil, nil, nil
}

// Create creates a SecretDefinition object
func (s *SecretDefinitionValues) Create(
	apiClient *http.Client,
	apiEndpoint string,
) (*v0.SecretDefinition, error) {
	// validate required fields
	if err := s.ValidateCreate(); err != nil {
		return nil, fmt.Errorf("failed to validate secret values: %w", err)
	}

	// get aws account
	awsAccount, err := client.GetAwsAccountByName(
		apiClient,
		apiEndpoint,
		s.AwsAccountName,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get aws account by id: %w", err)
	}

	// marshal json data
	jsonData, err := json.Marshal(s.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal data: %w", err)
	}

	// create secret definition
	createdSecretDefinition, err := client.CreateSecretDefinition(
		apiClient,
		apiEndpoint,
		&v0.SecretDefinition{
			Definition: v0.Definition{
				Name: util.Ptr(s.Name),
			},
			AwsAccountID: awsAccount.ID,
			Data:         util.Ptr(datatypes.JSON(jsonData)),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create secret definition: %w", err)
	}

	return createdSecretDefinition, nil
}

// Delete deletes a SecretDefinition object
func (s *SecretDefinitionValues) Delete(
	apiClient *http.Client,
	apiEndpoint string,
) (*v0.SecretDefinition, error) {
	// get secret definition
	secretDefinition, err := client.GetSecretDefinitionByName(
		apiClient,
		apiEndpoint,
		s.Name,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to get secret definition by name %s: %w",
			s.Name,
			err,
		)
	}

	// delete secret definition
	deletedSecretDefinition, err := client.DeleteSecretDefinition(
		apiClient,
		apiEndpoint,
		*secretDefinition.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to delete secret definition: %w", err)
	}

	return deletedSecretDefinition, nil
}

// Create creates a SecretInstance object
func (s *SecretInstanceValues) Create(
	apiClient *http.Client,
	apiEndpoint string,
) (*v0.SecretInstance, error) {
	// validate required fields
	if err := s.ValidateCreate(); err != nil {
		return nil, fmt.Errorf("failed to validate secret values: %w", err)
	}

	// get kubernetes runtime instance
	kubernetesRuntimeInstance, err := client.GetKubernetesRuntimeInstanceByName(
		apiClient,
		apiEndpoint,
		s.KubernetesRuntimeInstance.Name,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to get kubernetes runtime instance by name %s: %w",
			s.KubernetesRuntimeInstance.Name,
			err,
		)
	}

	// get secret definition
	secretDefinition, err := client.GetSecretDefinitionByName(
		apiClient,
		apiEndpoint,
		s.Name,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get secret definition by name %s: %w", s.Name, err)
	}

	// init secret instance object
	secretInstance := &v0.SecretInstance{
		Instance: v0.Instance{
			Name: util.Ptr(s.Name),
		},
		KubernetesRuntimeInstanceID: kubernetesRuntimeInstance.ID,
		SecretDefinitionID:          secretDefinition.ID,
	}

	// get workload instance
	switch {
	case s.WorkloadInstance != nil:
		workloadInstance, err := client.GetWorkloadInstanceByName(
			apiClient,
			apiEndpoint,
			s.WorkloadInstance.Name,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to get workload instance by name: %w", err)
		}
		secretInstance.WorkloadInstanceID = workloadInstance.ID
	case s.HelmWorkloadInstance != nil:
		helmWorkloadInstance, err := client.GetHelmWorkloadInstanceByName(
			apiClient,
			apiEndpoint,
			s.HelmWorkloadInstance.Name,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to get helm workload instance by name: %w", err)
		}
		secretInstance.HelmWorkloadInstanceID = helmWorkloadInstance.ID
	}

	// create secret instance
	createdSecretInstance, err := client.CreateSecretInstance(
		apiClient,
		apiEndpoint,
		secretInstance,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create secret instance: %w", err)
	}

	return createdSecretInstance, nil
}

// Delete deletes a SecretInstance object
func (s *SecretInstanceValues) Delete(
	apiClient *http.Client,
	apiEndpoint string,
) (*v0.SecretInstance, error) {
	// get secret instance
	secretInstance, err := client.GetSecretInstanceByName(
		apiClient,
		apiEndpoint,
		s.Name,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get secret instance by name %s: %w", s.Name, err)
	}

	// delete secret instance
	deletedSecretInstance, err := client.DeleteSecretInstance(
		apiClient,
		apiEndpoint,
		*secretInstance.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to delete secret instance: %w", err)
	}

	return deletedSecretInstance, nil
}

// ValidateCreate validates the secret values before creating a secret
func (s *SecretValues) ValidateCreate() error {
	multiError := util.MultiError{}

	if s.Name == "" {
		multiError.AppendError(errors.New("missing required field in config: Name"))
	}

	if s.Data == nil {
		multiError.AppendError(errors.New("missing required field in config: Data"))
	}

	if s.AwsAccountName == "" {
		multiError.AppendError(errors.New("missing required field in config: AwsAccountID"))
	}

	// ensure definition values or definition values document is set
	if s.WorkloadInstance == nil && s.HelmWorkloadInstance == nil {
		multiError.AppendError(errors.New("missing required field in config: WorkloadInstance or HelmWorkloadInstance"))
	}

	if s.KubernetesRuntimeInstance == nil {
		multiError.AppendError(errors.New("missing required field in config: KubernetesRuntimeInstance"))
	}

	if s.KubernetesRuntimeInstance != nil && s.KubernetesRuntimeInstance.Name == "" {
		multiError.AppendError(errors.New("missing required field in config: KubernetesRuntimeInstance.Name"))
	}

	return multiError.Error()
}

// ValidateCreate validates the secret values before creating a secret
func (s *SecretDefinitionValues) ValidateCreate() error {
	multiError := util.MultiError{}

	if s.Name == "" {
		multiError.AppendError(errors.New("missing required field in config: Name"))
	}

	if s.Data == nil {
		multiError.AppendError(errors.New("missing required field in config: Data"))
	}

	if s.AwsAccountName == "" {
		multiError.AppendError(errors.New("missing required field in config: AwsAccountID"))
	}

	return multiError.Error()
}

// ValidateCreate validates the secret values before creating a secret
func (s *SecretInstanceValues) ValidateCreate() error {
	multiError := util.MultiError{}

	if s.Name == "" {
		multiError.AppendError(errors.New("missing required field in config: Name"))
	}

	if s.SecretDefinition == nil {
		multiError.AppendError(errors.New("missing required field in config: SecretDefinition"))
	}

	// ensure definition values or definition values document is set
	if s.WorkloadInstance == nil && s.HelmWorkloadInstance == nil {
		multiError.AppendError(errors.New("missing required field in config: WorkloadInstance or HelmWorkloadInstance"))
	}

	if s.KubernetesRuntimeInstance == nil {
		multiError.AppendError(errors.New("missing required field in config: KubernetesRuntimeInstance"))
	}

	if s.KubernetesRuntimeInstance != nil && s.KubernetesRuntimeInstance.Name == "" {
		multiError.AppendError(errors.New("missing required field in config: KubernetesRuntimeInstance.Name"))
	}


	return multiError.Error()
}
