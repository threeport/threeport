package v0

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

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
	AwsAccountID              string                           `yaml:"AwsAccountID"`
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
	Name         string            `yaml:"Name"`
	AwsAccountID string            `yaml:"AwsAccountID"`
	Data         map[string]string `yaml:"Data"`
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
		Name:         s.Name,
		AwsAccountID: s.AwsAccountID,
		Data:         s.Data,
	}
	operations.AppendOperation(util.Operation{
		Name: "secret definition",
		Create: func() error {
			secretDefinition, err := secretDefinitionValues.Create(apiClient, apiEndpoint)
			if err != nil {
				return fmt.Errorf("failed to create secret definition: %w", err)
			}
			createdSecretDefinition = *secretDefinition
			return nil
		},
		Delete: func() error {
			_, err := secretDefinitionValues.Delete(apiClient, apiEndpoint)
			if err != nil {
				return fmt.Errorf("failed to delete secret definition: %w", err)
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
				return fmt.Errorf("failed to create secret instance: %w", err)
			}
			createdSecretInstance = *secretInstance
			return nil
		},
		Delete: func() error {
			_, err := secretInstanceValues.Delete(apiClient, apiEndpoint)
			if err != nil {
				return fmt.Errorf("failed to delete secret instance: %w", err)
			}
			return nil
		},
	})

	return &operations, &createdSecretDefinition, &createdSecretInstance
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

	if s.AwsAccountID == "" {
		multiError.AppendError(errors.New("missing required field in config: AwsAccountID"))
	}

	// ensure definition values or definition values document is set
	if s.WorkloadInstance == nil && s.HelmWorkloadInstance == nil {
		multiError.AppendError(errors.New("missing required field in config: WorkloadInstance or HelmWorkloadInstance"))
	}

	return multiError.Error()
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

	awsAccountID, err := strconv.Atoi(s.AwsAccountID)
	if err != nil {
		return nil, fmt.Errorf("failed to convert aws account id to int: %w", err)
	}

	jsonData, err := json.Marshal(s.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal data: %w", err)
	}

	// Initialize a datatypes.JSON value
	createdSecretDefinition, err := client.CreateSecretDefinition(
		apiClient,
		apiEndpoint,
		&v0.SecretDefinition{
			Definition: v0.Definition{
				Name: util.Ptr(s.Name),
			},
			AwsAccountID: util.Ptr(uint(awsAccountID)),
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
	secretDefinition, err := client.GetSecretDefinitionByName(
		apiClient,
		apiEndpoint,
		s.Name,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get secret definition by name: %w", err)
	}

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
	kubernetesRuntimeInstance, err := client.GetKubernetesRuntimeInstanceByName(
		apiClient,
		apiEndpoint,
		s.KubernetesRuntimeInstance.Name,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get kubernetes runtime instance by name: %w", err)
	}
	secretDefinition, err := client.GetSecretDefinitionByName(
		apiClient,
		apiEndpoint,
		s.Name,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get secret definition by name: %w", err)
	}

	secretInstance := &v0.SecretInstance{
		Instance: v0.Instance{
			Name: util.Ptr(s.Name),
		},
		KubernetesRuntimeInstanceID: util.Ptr(*kubernetesRuntimeInstance.ID),
		SecretDefinitionID:          util.Ptr(*secretDefinition.ID),
	}

	if s.WorkloadInstance != nil {
		workloadInstance, err := client.GetWorkloadInstanceByName(
			apiClient,
			apiEndpoint,
			s.WorkloadInstance.Name,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to get workload instance by name: %w", err)
		}
		secretInstance.WorkloadInstanceID = util.Ptr(*workloadInstance.ID)
	}

	if s.HelmWorkloadInstance != nil {
		helmWorkloadInstance, err := client.GetHelmWorkloadInstanceByName(
			apiClient,
			apiEndpoint,
			s.HelmWorkloadInstance.Name,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to get helm workload instance by name: %w", err)
		}
		secretInstance.HelmWorkloadInstanceID = util.Ptr(*helmWorkloadInstance.ID)
	}

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

	secretInstance, err := client.GetSecretInstanceByName(
		apiClient,
		apiEndpoint,
		s.Name,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get secret instance by name: %w", err)
	}

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
