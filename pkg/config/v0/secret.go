package v0

import (
	"net/http"

	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// SecretConfig contains the configuration for a Secret
// object
type SecretConfig struct {
	Secret SecretValues `yaml:"Secret"`
}

// SecretValues contains the values for a Secret object
// configuration
type SecretValues struct {
	Name string `yaml:"Name"`
}

// SecretDefinitionConfig contains the configuration for a
// SecretDefinition object
type SecretDefinitionConfig struct {
	SecretDefinition SecretDefinitionValues `yaml:"SecretDefinition"`
}

// SecretDefinitionValues contains the values for a
// SecretDefinition object
type SecretDefinitionValues struct {
	Name string `yaml:"Name"`
}

// SecretInstanceConfig contains the configuration for a
// SecretInstance object
type SecretInstanceConfig struct {
	SecretInstance SecretInstanceValues `yaml:"SecretInstance"`
}

// SecretInstanceValues contains the values for a
// SecretInstance object
type SecretInstanceValues struct {
	Name string `yaml:"Name"`
}

// Create creates a Secret object
func (s *SecretValues) Create(
	apiClient *http.Client,
	apiEndpoint string,
) (*v0.SecretDefinition, *v0.SecretInstance, error) {
	return nil, nil, nil
}

// Delete deletes a Secret object
func (s *SecretValues) Delete(
	apiClient *http.Client,
	apiEndpoint string,
) (*v0.SecretDefinition, *v0.SecretInstance, error) {
	return nil, nil, nil
}

// Create creates a SecretDefinition object
func (s *SecretDefinitionValues) Create(
	apiClient *http.Client,
	apiEndpoint string,
) (*v0.SecretDefinition, error) {
	return nil, nil
}

// Delete deletes a SecretDefinition object
func (s *SecretDefinitionValues) Delete(
	apiClient *http.Client,
	apiEndpoint string,
) (*v0.SecretDefinition, error) {
	return nil, nil
}

// Create creates a SecretInstance object
func (s *SecretInstanceValues) Create(
	apiClient *http.Client,
	apiEndpoint string,
) (*v0.SecretInstance, error) {
	return nil, nil
}

// Delete deletes a SecretInstance object
func (s *SecretInstanceValues) Delete(
	apiClient *http.Client,
	apiEndpoint string,
) (*v0.SecretInstance, error) {
	return nil, nil
}
