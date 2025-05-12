package v0

import (
	"fmt"
	"net/http"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// OciAccountConfig contains the config for an OCI account.
type OciAccountConfig struct {
	OciAccount OciAccountValues `yaml:"OciAccount"`
}

// OciAccountValues contains the attributes needed to manage an OCI account.
type OciAccountValues struct {
	Name           *string `yaml:"Name"`
	TenancyID      *string `yaml:"TenancyID"`
	DefaultAccount *bool   `yaml:"DefaultAccount"`
	DefaultRegion  *string `yaml:"DefaultRegion"`
	UserID         *string `yaml:"UserID"`
	Fingerprint    *string `yaml:"Fingerprint"`
	PrivateKey     *string `yaml:"PrivateKey"`
	LocalConfig    *string `yaml:"LocalConfig"`
	LocalProfile   *string `yaml:"LocalProfile"`
}

// OciOkeKubernetesRuntimeConfig contains the config for an OCI OKE
// kubernetes runtime which is an abstraction of an OCI OKE kubernetes runtime
// definition and instance.
type OciOkeKubernetesRuntimeConfig struct {
	OciOkeKubernetesRuntime OciOkeKubernetesRuntimeValues `yaml:"OciOkeKubernetesRuntime"`
}

// OciOkeKubernetesRuntimeValues contains the attributes needed to
// manage an OCI OKE kubernetes runtime definition and instance.
type OciOkeKubernetesRuntimeValues struct {
	Name                    *string `yaml:"Name"`
	OciAccountName          *string `yaml:"OciAccountName"`
	AvailabilityDomainCount *int    `yaml:"AvailabilityDomainCount"`
	WorkerNodeShape         *string `yaml:"WorkerNodeShape"`
	WorkerNodeInitialCount  *int    `yaml:"WorkerNodeInitialCount"`
	WorkerNodeMinCount      *int    `yaml:"WorkerNodeMinCount"`
	WorkerNodeMaxCount      *int    `yaml:"WorkerNodeMaxCount"`
	Region                  *string `yaml:"Region"`
}

// OciOkeKubernetesRuntimeDefinitionConfig contains the config for an OCI OKE
// kubernetes runtime definition.
type OciOkeKubernetesRuntimeDefinitionConfig struct {
	OciOkeKubernetesRuntimeDefinition OciOkeKubernetesRuntimeDefinitionValues `yaml:"OciOkeKubernetesRuntimeDefinition"`
}

// OciOkeKubernetesRuntimeDefinitionValues contains the attributes needed to
// manage an OCI OKE kubernetes runtime definition.
type OciOkeKubernetesRuntimeDefinitionValues struct {
	Name                    *string `yaml:"Name"`
	OciAccountName          *string `yaml:"OciAccountName"`
	AvailabilityDomainCount *int    `yaml:"AvailabilityDomainCount"`
	WorkerNodeShape         *string `yaml:"WorkerNodeShape"`
	WorkerNodeInitialCount  *int    `yaml:"WorkerNodeInitialCount"`
	WorkerNodeMinCount      *int    `yaml:"WorkerNodeMinCount"`
	WorkerNodeMaxCount      *int    `yaml:"WorkerNodeMaxCount"`
}

// OciOkeKubernetesRuntimeInstanceConfig contains the config for an OCI OKE
// kubernetes runtime instance.
type OciOkeKubernetesRuntimeInstanceConfig struct {
	OciOkeKubernetesRuntimeInstance OciOkeKubernetesRuntimeInstanceValues `yaml:"OciOkeKubernetesRuntimeInstance"`
}

// OciOkeKubernetesRuntimeInstanceValues contains the attributes needed to
// manage an OCI OKE kubernetes runtime instance.
type OciOkeKubernetesRuntimeInstanceValues struct {
	Name                              *string                                  `yaml:"Name"`
	Region                            *string                                  `yaml:"Region"`
	OciOkeKubernetesRuntimeDefinition *OciOkeKubernetesRuntimeDefinitionValues `yaml:"OciOkeKubernetesRuntimeDefinition"`
}

// Methods for OciAccountValues

// Create creates a new OCI account in the Threeport API.
func (o *OciAccountValues) Create(apiClient *http.Client, apiEndpoint string) (*v0.OciAccount, error) {
	ociAccount := v0.OciAccount{
		Name:           o.Name,
		TenancyID:      o.TenancyID,
		DefaultAccount: o.DefaultAccount,
		DefaultRegion:  o.DefaultRegion,
		PrivateKey:     o.PrivateKey,
	}

	createdOciAccount, err := client.CreateOciAccount(apiClient, apiEndpoint, &ociAccount)
	if err != nil {
		return nil, fmt.Errorf("failed to create OCI account: %w", err)
	}

	return createdOciAccount, nil
}

// Delete deletes an OCI account from the Threeport API.
func (o *OciAccountValues) Delete(apiClient *http.Client, apiEndpoint string) (v0.OciAccount, error) {
	// First get the account by name to get its ID
	account, err := client.GetOciAccountByName(apiClient, apiEndpoint, *o.Name)
	if err != nil {
		return v0.OciAccount{}, fmt.Errorf("failed to get OCI account: %w", err)
	}

	// Delete the account using its ID
	deletedAccount, err := client.DeleteOciAccount(apiClient, apiEndpoint, *account.ID)
	if err != nil {
		return v0.OciAccount{}, fmt.Errorf("failed to delete OCI account: %w", err)
	}

	return *deletedAccount, nil
}

// Methods for OciOkeKubernetesRuntimeValues

// Create creates a new OCI OKE kubernetes runtime in the Threeport API.
func (o *OciOkeKubernetesRuntimeValues) Create(apiClient *http.Client, apiEndpoint string) (*v0.OciOkeKubernetesRuntimeDefinition, *v0.OciOkeKubernetesRuntimeInstance, error) {
	// Create the definition first
	definition := v0.OciOkeKubernetesRuntimeDefinition{
		Definition: v0.Definition{
			Name: o.Name,
		},
		OciAccountID:            nil, // TODO: Get this from OciAccountName
		AvailabilityDomainCount: util.Ptr(int32(*o.AvailabilityDomainCount)),
		WorkerNodeShape:         o.WorkerNodeShape,
		WorkerNodeInitialCount:  util.Ptr(int32(*o.WorkerNodeInitialCount)),
		WorkerNodeMinCount:      util.Ptr(int32(*o.WorkerNodeMinCount)),
		WorkerNodeMaxCount:      util.Ptr(int32(*o.WorkerNodeMaxCount)),
	}

	createdDefinition, err := client.CreateOciOkeKubernetesRuntimeDefinition(apiClient, apiEndpoint, &definition)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create OCI OKE kubernetes runtime definition: %w", err)
	}

	// Then create the instance
	instance := v0.OciOkeKubernetesRuntimeInstance{
		Instance: v0.Instance{
			Name: o.Name,
		},
		Region:                              o.Region,
		OciOkeKubernetesRuntimeDefinitionID: createdDefinition.ID,
	}

	createdInstance, err := client.CreateOciOkeKubernetesRuntimeInstance(apiClient, apiEndpoint, &instance)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create OCI OKE kubernetes runtime instance: %w", err)
	}

	return createdDefinition, createdInstance, nil
}

// Delete deletes an OCI OKE kubernetes runtime from the Threeport API.
func (o *OciOkeKubernetesRuntimeValues) Delete(apiClient *http.Client, apiEndpoint string) (*v0.OciOkeKubernetesRuntimeDefinition, *v0.OciOkeKubernetesRuntimeInstance, error) {
	// First get the instance by name to get its ID
	instance, err := client.GetOciOkeKubernetesRuntimeInstanceByName(apiClient, apiEndpoint, *o.Name)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get OCI OKE kubernetes runtime instance: %w", err)
	}

	// Delete the instance using its ID
	deletedInstance, err := client.DeleteOciOkeKubernetesRuntimeInstance(apiClient, apiEndpoint, *instance.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to delete OCI OKE kubernetes runtime instance: %w", err)
	}

	// Then get the definition by name to get its ID
	definition, err := client.GetOciOkeKubernetesRuntimeDefinitionByName(apiClient, apiEndpoint, *o.Name)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get OCI OKE kubernetes runtime definition: %w", err)
	}

	// Delete the definition using its ID
	deletedDefinition, err := client.DeleteOciOkeKubernetesRuntimeDefinition(apiClient, apiEndpoint, *definition.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to delete OCI OKE kubernetes runtime definition: %w", err)
	}

	return deletedDefinition, deletedInstance, nil
}

// Methods for OciOkeKubernetesRuntimeDefinitionValues

// Create creates a new OCI OKE kubernetes runtime definition in the Threeport API.
func (o *OciOkeKubernetesRuntimeDefinitionValues) Create(apiClient *http.Client, apiEndpoint string) (*v0.OciOkeKubernetesRuntimeDefinition, error) {
	ociOkeKubernetesRuntimeDefinition := v0.OciOkeKubernetesRuntimeDefinition{
		// Name:                    o.Name,
		// OciAccountName:          o.OciAccountName,
		// AvailabilityDomainCount: o.AvailabilityDomainCount,
		WorkerNodeShape: o.WorkerNodeShape,
		// WorkerNodeInitialCount:  o.WorkerNodeInitialCount,
		// WorkerNodeMinCount:      o.WorkerNodeMinCount,
		// WorkerNodeMaxCount:      o.WorkerNodeMaxCount,
	}

	createdDefinition, err := client.CreateOciOkeKubernetesRuntimeDefinition(apiClient, apiEndpoint, &ociOkeKubernetesRuntimeDefinition)
	if err != nil {
		return nil, fmt.Errorf("failed to create OCI OKE kubernetes runtime definition: %w", err)
	}

	return createdDefinition, nil
}

// Delete deletes an OCI OKE kubernetes runtime definition from the Threeport API.
func (o *OciOkeKubernetesRuntimeDefinitionValues) Delete(apiClient *http.Client, apiEndpoint string) (v0.OciOkeKubernetesRuntimeDefinition, error) {
	// First get the definition by name to get its ID
	definition, err := client.GetOciOkeKubernetesRuntimeDefinitionByName(apiClient, apiEndpoint, *o.Name)
	if err != nil {
		return v0.OciOkeKubernetesRuntimeDefinition{}, fmt.Errorf("failed to get OCI OKE kubernetes runtime definition: %w", err)
	}

	// Delete the definition using its ID
	deletedDefinition, err := client.DeleteOciOkeKubernetesRuntimeDefinition(apiClient, apiEndpoint, *definition.ID)
	if err != nil {
		return v0.OciOkeKubernetesRuntimeDefinition{}, fmt.Errorf("failed to delete OCI OKE kubernetes runtime definition: %w", err)
	}

	return *deletedDefinition, nil
}

// Methods for OciOkeKubernetesRuntimeInstanceValues

// Create creates a new OCI OKE kubernetes runtime instance in the Threeport API.
func (o *OciOkeKubernetesRuntimeInstanceValues) Create(apiClient *http.Client, apiEndpoint string) (*v0.OciOkeKubernetesRuntimeInstance, error) {
	ociOkeKubernetesRuntimeInstance := v0.OciOkeKubernetesRuntimeInstance{
		// Name:                              o.Name,
		Region: o.Region,
		// OciOkeKubernetesRuntimeDefinition: o.OciOkeKubernetesRuntimeDefinition,
	}

	createdInstance, err := client.CreateOciOkeKubernetesRuntimeInstance(apiClient, apiEndpoint, &ociOkeKubernetesRuntimeInstance)
	if err != nil {
		return nil, fmt.Errorf("failed to create OCI OKE kubernetes runtime instance: %w", err)
	}

	return createdInstance, nil
}

// Delete deletes an OCI OKE kubernetes runtime instance from the Threeport API.
func (o *OciOkeKubernetesRuntimeInstanceValues) Delete(apiClient *http.Client, apiEndpoint string) (v0.OciOkeKubernetesRuntimeInstance, error) {
	// First get the instance by name to get its ID
	instance, err := client.GetOciOkeKubernetesRuntimeInstanceByName(apiClient, apiEndpoint, *o.Name)
	if err != nil {
		return v0.OciOkeKubernetesRuntimeInstance{}, fmt.Errorf("failed to get OCI OKE kubernetes runtime instance: %w", err)
	}

	// Delete the instance using its ID
	deletedInstance, err := client.DeleteOciOkeKubernetesRuntimeInstance(apiClient, apiEndpoint, *instance.ID)
	if err != nil {
		return v0.OciOkeKubernetesRuntimeInstance{}, fmt.Errorf("failed to delete OCI OKE kubernetes runtime instance: %w", err)
	}

	return *deletedInstance, nil
}
