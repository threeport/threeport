package v0

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"

	"github.com/threeport/threeport/internal/agent"
	"github.com/threeport/threeport/internal/workload/status"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// HelmWorkloadConfig contains the config for a helm workload which is an abstraction of
// a helm workload definition and helm workload instance.
type HelmWorkloadConfig struct {
	HelmWorkload HelmWorkloadValues `yaml:"HelmWorkload"`
}

// HelmWorkloadValues contains the attributes needed to manage a helm workload
// definition and helm workload instance.
type HelmWorkloadValues struct {
	Name                      string                           `yaml:"Name"`
	Repo                      string                           `yaml:"Repo"`
	Chart                     string                           `yaml:"Chart"`
	ChartVersion              string                           `yaml:"ChartVersion"`
	DefinitionValues          string                           `yaml:"DefinitionValues"`
	DefinitionValuesDocument  string                           `yaml:"DefinitionValuesDocument"`
	InstanceValues            string                           `yaml:"InstanceValues"`
	InstanceValuesDocument    string                           `yaml:"InstanceValuesDocument"`
	HelmWorkloadConfigPath    string                           `yaml:"HelmWorkloadConfigPath"`
	KubernetesRuntimeInstance *KubernetesRuntimeInstanceValues `yaml:"KubernetesRuntimeInstance"`
	DomainName                *DomainNameDefinitionValues      `yaml:"DomainName"`
	Gateway                   *GatewayDefinitionValues         `yaml:"Gateway"`
	AwsRelationalDatabase     *AwsRelationalDatabaseValues     `yaml:"AwsRelationalDatabase"`
	AwsObjectStorageBucket    *AwsObjectStorageBucketValues    `yaml:"AwsObjectStorageBucket"`
}

// HelmWorkloadDefinitionConfig contains the config for a helm workload definition.
type HelmWorkloadDefinitionConfig struct {
	HelmWorkloadDefinition HelmWorkloadDefinitionValues `yaml:"HelmWorkloadDefinition"`
}

// HelmWorkloadDefinitionValues contains the attributes needed to manage a helm workload
// definition.
type HelmWorkloadDefinitionValues struct {
	Name                   string `yaml:"Name"`
	Repo                   string `yaml:"Repo"`
	Chart                  string `yaml:"Chart"`
	ChartVersion           string `yaml:"ChartVersion"`
	Values                 string `yaml:"Values"`
	ValuesDocument         string `yaml:"ValuesDocument"`
	HelmWorkloadConfigPath string `yaml:"HelmWorkloadConfigPath"`
}

// HelmWorkloadInstanceConfig contains the config for a helm workload instance.
type HelmWorkloadInstanceConfig struct {
	HelmWorkloadInstance HelmWorkloadInstanceValues `yaml:"HelmWorkloadInstance"`
}

// HelmWorkloadInstanceValues contains the attributes needed to manage a helm workload
// instance.
type HelmWorkloadInstanceValues struct {
	Name                      string                           `yaml:"Name"`
	Values                    string                           `yaml:"Values"`
	ValuesDocument            string                           `yaml:"ValuesDocument"`
	KubernetesRuntimeInstance *KubernetesRuntimeInstanceValues `yaml:"KubernetesRuntimeInstance"`
	HelmWorkloadDefinition    HelmWorkloadDefinitionValues     `yaml:"HelmWorkloadDefinition"`
	HelmWorkloadConfigPath    string                           `yaml:"HelmWorkloadConfigPath"`
}

// Create creates a helm workload definition and instance in the Threeport API.
func (h *HelmWorkloadValues) Create(
	apiClient *http.Client,
	apiEndpoint string,
) (*v0.HelmWorkloadDefinition, *v0.HelmWorkloadInstance, error) {

	// get operations
	operations, createdHelmWorkloadDefinition, createdHelmWorkloadInstance, err := h.GetOperations(apiClient, apiEndpoint)
	if err != nil {
		return nil, nil, err
	}

	// execute create operations
	if err := operations.Create(); err != nil {
		return nil, nil, err
	}

	return createdHelmWorkloadDefinition, createdHelmWorkloadInstance, nil
}

// Delete deletes a helm workload definition, helm workload instance,
// domain name definition, domain name instance,
// gateway definition, and gateway instance from the Threeport API.
func (h *HelmWorkloadValues) Delete(
	apiClient *http.Client,
	apiEndpoint string,
) (*v0.HelmWorkloadDefinition, *v0.HelmWorkloadInstance, error) {

	// get operation
	operations, _, _, err := h.GetOperations(apiClient, apiEndpoint)
	if err != nil {
		return nil, nil, err
	}

	// execute delete operations
	if err := operations.Delete(); err != nil {
		return nil, nil, err
	}

	return nil, nil, nil
}

// Create creates a helm workload definition in the Threeport API.
func (h *HelmWorkloadDefinitionValues) Create(
	apiClient *http.Client,
	apiEndpoint string,
) (*v0.HelmWorkloadDefinition, error) {
	// validate required fields
	if h.Name == "" || h.Repo == "" || h.Chart == "" {
		return nil, errors.New("missing required field/s in config - required fields: Name, Repo, Chart")
	}

	// construct helm workload definition object
	helmWorkloadDefinition := v0.HelmWorkloadDefinition{
		Definition: v0.Definition{
			Name: &h.Name,
		},
		Repo:  &h.Repo,
		Chart: &h.Chart,
	}

	// set helm values if present
	values, err := GetValuesFromDocumentOrInline(h.Values, h.ValuesDocument, h.HelmWorkloadConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get values document from path: %w", err)
	}
	helmWorkloadDefinition.ValuesDocument = values

	// create helm workload definition
	createdHelmWorkloadDefinition, err := client.CreateHelmWorkloadDefinition(
		apiClient,
		apiEndpoint,
		&helmWorkloadDefinition,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create helm workload definition in threeport API: %w", err)
	}

	return createdHelmWorkloadDefinition, nil
}

// Delete deletes a helm workload definition from the Threeport API.
func (h *HelmWorkloadDefinitionValues) Delete(
	apiClient *http.Client,
	apiEndpoint string,
) (*v0.HelmWorkloadDefinition, error) {
	// get helm workload definition by name
	helmWorkloadDefinition, err := client.GetHelmWorkloadDefinitionByName(apiClient, apiEndpoint, h.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to find helm workload definition with name %s: %w", h.Name, err)
	}

	// delete helm workload definition
	deletedHelmWorkloadDefinition, err := client.DeleteHelmWorkloadDefinition(
		apiClient,
		apiEndpoint,
		*helmWorkloadDefinition.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to delete helm workload definition from threeport API: %w", err)
	}

	return deletedHelmWorkloadDefinition, nil
}

// Create creates a helm workload instance in the Threeport API.
func (h *HelmWorkloadInstanceValues) Create(
	apiClient *http.Client,
	apiEndpoint string,
) (*v0.HelmWorkloadInstance, error) {
	// validate required fields
	if h.Name == "" || h.HelmWorkloadDefinition.Name == "" {
		return nil, errors.New("missing required field/s in config - required fields: Name, HelmWorkloadDefinition.Name")
	}

	// get kubernetes runtime instance API object
	kubernetesRuntimeInstance, err := setKubernetesRuntimeInstanceForConfig(
		h.KubernetesRuntimeInstance,
		apiClient,
		apiEndpoint,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to set kubernetes runtime instance: %w", err)
	}

	// get helm workload definition by name
	helmWorkloadDefinition, err := client.GetHelmWorkloadDefinitionByName(
		apiClient,
		apiEndpoint,
		h.HelmWorkloadDefinition.Name,
	)
	if err != nil {
		return nil, err
	}

	// construct helm workload instance object
	helmWorkloadInstance := v0.HelmWorkloadInstance{
		Instance: v0.Instance{
			Name: &h.Name,
		},
		KubernetesRuntimeInstanceID: kubernetesRuntimeInstance.ID,
		HelmWorkloadDefinitionID:    helmWorkloadDefinition.ID,
	}

	// get helm instance values
	values, err := GetValuesFromDocumentOrInline(h.Values, h.ValuesDocument, h.HelmWorkloadConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get helm instance values document from path: %w", err)
	}
	helmWorkloadInstance.ValuesDocument = values

	// create helm workload instance
	createdHelmWorkloadInstance, err := client.CreateHelmWorkloadInstance(
		apiClient,
		apiEndpoint,
		&helmWorkloadInstance,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create helm workload instance in threeport API: %w", err)
	}

	return createdHelmWorkloadInstance, nil
}

// Describe returns important failure events related to a helm workload instance.
func (h *HelmWorkloadInstanceValues) Describe(apiClient *http.Client, apiEndpoint string) (*status.WorkloadInstanceStatusDetail, error) {
	// get helm workload instance by name
	helmWorkloadInstance, err := client.GetHelmWorkloadInstanceByName(apiClient, apiEndpoint, h.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to find helm workload instance with name %s: %w", h.Name, err)
	}

	// get helm workload instance status
	statusDetail := status.GetWorkloadInstanceStatus(
		apiClient,
		apiEndpoint,
		agent.HelmWorkloadInstanceType,
		*helmWorkloadInstance.ID,
		*helmWorkloadInstance.Reconciled,
	)
	if statusDetail.Error != nil {
		return nil, fmt.Errorf("failed to get status for helm workload instance with name %s: %w", h.Name, statusDetail.Error)
	}

	return statusDetail, nil
}

// Delete deletes a helm workload instance from the Threeport API.
func (h *HelmWorkloadInstanceValues) Delete(
	apiClient *http.Client,
	apiEndpoint string,
) (*v0.HelmWorkloadInstance, error) {
	// get helm workload instance by name
	helmWorkloadInstance, err := client.GetHelmWorkloadInstanceByName(apiClient, apiEndpoint, h.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to find helm workload instance with name %s: %w", h.Name, err)
	}

	// delete helm workload instance
	deletedHelmWorkloadInstance, err := client.DeleteHelmWorkloadInstance(
		apiClient,
		apiEndpoint,
		*helmWorkloadInstance.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to delete helm workload instance from threeport API: %w", err)
	}

	// wait for helm workload instance to be deleted
	util.Retry(60, 1, func() error {
		if _, err := client.GetHelmWorkloadInstanceByName(apiClient, apiEndpoint, h.Name); err == nil {
			return errors.New("helm workload instance not deleted")
		}
		return nil
	})

	return deletedHelmWorkloadInstance, nil
}

// GetOperations returns a slice of operations used to
// create, update, or delete a helm workload.
func (h *HelmWorkloadValues) GetOperations(
	apiClient *http.Client,
	apiEndpoint string,
) (*util.Operations, *v0.HelmWorkloadDefinition, *v0.HelmWorkloadInstance, error) {

	var err error
	var createdHelmWorkloadInstance v0.HelmWorkloadInstance
	var createdHelmWorkloadDefinition v0.HelmWorkloadDefinition

	operations := util.Operations{}

	// get helm definition values
	helmDefinitionValues, err := GetValuesFromDocumentOrInline(h.DefinitionValues, h.DefinitionValuesDocument, h.HelmWorkloadConfigPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get helm instance values document from path: %w", err)
	}

	// add helm workload definition operation
	helmWorkloadDefinitionValues := HelmWorkloadDefinitionValues{
		Name:                   h.Name,
		Repo:                   h.Repo,
		Chart:                  h.Chart,
		ChartVersion:           h.ChartVersion,
		Values:                 *helmDefinitionValues,
		HelmWorkloadConfigPath: h.HelmWorkloadConfigPath,
	}
	operations.AppendOperation(util.Operation{
		Name: "helm workload definition",
		Create: func() error {
			helmWorkloadDefinition, err := helmWorkloadDefinitionValues.Create(apiClient, apiEndpoint)
			if err != nil {
				return err
			}
			createdHelmWorkloadDefinition = *helmWorkloadDefinition
			return nil
		},
		Delete: func() error {
			_, err = helmWorkloadDefinitionValues.Delete(apiClient, apiEndpoint)
			return err
		},
	})

	// get helm instance values
	helmInstanceValues, err := GetValuesFromDocumentOrInline(h.InstanceValues, h.InstanceValuesDocument, h.HelmWorkloadConfigPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get helm instance values document from path: %w", err)
	}

	// add helm workload instance operation
	helmWorkloadInstanceValues := HelmWorkloadInstanceValues{
		Name:                      h.Name,
		Values:                    *helmInstanceValues,
		HelmWorkloadConfigPath:    h.HelmWorkloadConfigPath,
		KubernetesRuntimeInstance: h.KubernetesRuntimeInstance,
		HelmWorkloadDefinition: HelmWorkloadDefinitionValues{
			Name: h.Name,
		},
	}
	operations.AppendOperation(util.Operation{
		Name: "helm workload instance",
		Create: func() error {
			helmWorkloadInstance, err := helmWorkloadInstanceValues.Create(apiClient, apiEndpoint)
			if err != nil {
				return err
			}
			createdHelmWorkloadInstance = *helmWorkloadInstance
			return nil
		},
		Delete: func() error {
			_, err = helmWorkloadInstanceValues.Delete(apiClient, apiEndpoint)
			return err
		},
	})

	//// add domain name and gateway if provided
	//if h.DomainName != nil && h.Gateway != nil {

	//	// add domain name definition operation
	//	domainNameDefinitionValues := DomainNameDefinitionValues{
	//		Domain:     h.DomainName.Domain,
	//		Zone:       h.DomainName.Zone,
	//		AdminEmail: h.DomainName.AdminEmail,
	//	}
	//	operations.AppendOperation(util.Operation{
	//		Name: "domain name definition",
	//		Create: func() error {
	//			_, err = domainNameDefinitionValues.CreateIfNotExist(apiClient, apiEndpoint)
	//			return err
	//		},
	//		Delete: func() error {
	//			_, err = domainNameDefinitionValues.Delete(apiClient, apiEndpoint)
	//			return err
	//		},
	//	})

	//	// add domain name instance operation
	//	domainNameInstanceValues := DomainNameInstanceValues{
	//		DomainNameDefinition:      domainNameDefinitionValues,
	//		KubernetesRuntimeInstance: h.KubernetesRuntimeInstance,
	//		HelmWorkloadInstance:      helmWorkloadInstanceValues,
	//	}
	//	operations.AppendOperation(util.Operation{
	//		Name: "domain name instance",
	//		Create: func() error {
	//			_, err = domainNameInstanceValues.Create(apiClient, apiEndpoint)
	//			return err
	//		},
	//		Delete: func() error {
	//			_, err = domainNameInstanceValues.Delete(apiClient, apiEndpoint)
	//			return err
	//		},
	//	})

	//	// add gateway definition operation
	//	gatewayDefinitionValues := GatewayDefinitionValues{
	//		Name:                 h.Name,
	//		HttpPorts:            h.Gateway.HttpPorts,
	//		TcpPorts:             h.Gateway.TcpPorts,
	//		ServiceName:          h.Gateway.ServiceName,
	//		SubDomain:            h.Gateway.SubDomain,
	//		DomainNameDefinition: domainNameDefinitionValues,
	//	}
	//	operations.AppendOperation(util.Operation{
	//		Name: "gateway definition",
	//		Create: func() error {
	//			_, err = gatewayDefinitionValues.Create(apiClient, apiEndpoint)
	//			return err
	//		},
	//		Delete: func() error {
	//			_, err = gatewayDefinitionValues.Delete(apiClient, apiEndpoint)
	//			return err
	//		},
	//	})

	//	// add gateway instance operation
	//	gatewayInstanceValues := GatewayInstanceValues{
	//		GatewayDefinition:         gatewayDefinitionValues,
	//		KubernetesRuntimeInstance: h.KubernetesRuntimeInstance,
	//		HelmWorkloadInstance:      helmWorkloadInstanceValues,
	//	}
	//	operations.AppendOperation(util.Operation{
	//		Name: "gateway instance",
	//		Create: func() error {
	//			_, err = gatewayInstanceValues.Create(apiClient, apiEndpoint)
	//			return err
	//		},
	//		Delete: func() error {
	//			_, err = gatewayInstanceValues.Delete(apiClient, apiEndpoint)
	//			return err
	//		},
	//	})
	//}

	//// add AWS relational database operation
	//if h.AwsRelationalDatabase != nil {
	//	awsRelationalDatabase := AwsRelationalDatabaseValues{
	//		Name:                   h.AwsRelationalDatabase.Name,
	//		AwsAccountName:         h.AwsRelationalDatabase.AwsAccountName,
	//		Engine:                 h.AwsRelationalDatabase.Engine,
	//		EngineVersion:          h.AwsRelationalDatabase.EngineVersion,
	//		DatabaseName:           h.AwsRelationalDatabase.DatabaseName,
	//		DatabasePort:           h.AwsRelationalDatabase.DatabasePort,
	//		BackupDays:             h.AwsRelationalDatabase.BackupDays,
	//		MachineSize:            h.AwsRelationalDatabase.MachineSize,
	//		StorageGb:              h.AwsRelationalDatabase.StorageGb,
	//		HelmWorkloadSecretName: h.AwsRelationalDatabase.HelmWorkloadSecretName,
	//		HelmWorkloadInstance: &HelmWorkloadInstanceValues{
	//			Name: h.Name,
	//		},
	//	}
	//	operations.AppendOperation(util.Operation{
	//		Name: "aws relational database",
	//		Create: func() error {
	//			_, _, err := awsRelationalDatabase.Create(apiClient, apiEndpoint)
	//			return err
	//		},
	//		Delete: func() error {
	//			_, _, err = awsRelationalDatabase.Delete(apiClient, apiEndpoint)
	//			return err
	//		},
	//	})
	//}

	//// add AWS object storage bucket operation
	//if h.AwsObjectStorageBucket != nil {
	//	awsObjectStorageBucket := AwsObjectStorageBucketValues{
	//		Name:                           h.AwsObjectStorageBucket.Name,
	//		AwsAccountName:                 h.AwsObjectStorageBucket.AwsAccountName,
	//		PublicReadAccess:               h.AwsObjectStorageBucket.PublicReadAccess,
	//		HelmWorkloadServiceAccountName: h.AwsObjectStorageBucket.HelmWorkloadServiceAccountName,
	//		HelmWorkloadBucketEnvVar:       h.AwsObjectStorageBucket.HelmWorkloadBucketEnvVar,
	//		HelmWorkloadInstance: &HelmWorkloadInstanceValues{
	//			Name: h.Name,
	//		},
	//	}
	//	operations.AppendOperation(util.Operation{
	//		Name: "aws object storage bucket",
	//		Create: func() error {
	//			_, _, err := awsObjectStorageBucket.Create(apiClient, apiEndpoint)
	//			return err
	//		},
	//		Delete: func() error {
	//			_, _, err := awsObjectStorageBucket.Delete(apiClient, apiEndpoint)
	//			return err
	//		},
	//	})
	//}

	return &operations, &createdHelmWorkloadDefinition, &createdHelmWorkloadInstance
}

// GetValuesDocumentFromPath returns the values document content from the path
// provided.
func GetValuesDocumentFromPath(valuesDocument, helmWorkloadConfigPath string) (*string, error) {
	// set helm values if present
	if valuesDocument != "" {
		// build the path to the values document relative to the user's working
		// directory
		configPath, _ := filepath.Split(helmWorkloadConfigPath)
		relativeValuesPath := path.Join(configPath, valuesDocument)

		// load vaules document
		valuesContent, err := os.ReadFile(relativeValuesPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read definition ValuesDocument file %s: %w", valuesDocument, err)
		}
		stringContent := string(valuesContent)
		return &stringContent, nil
	}
	return nil, nil
}

// GetValuesFromDocumentOrInline returns the values document content from the
// inline value provided first, then from the document provided
func GetValuesFromDocumentOrInline(inline, valuesDocument, helmWorkloadConfigPath string) (*string, error) {
	if inline != "" {
		return &inline, nil
	}
	return GetValuesDocumentFromPath(valuesDocument, helmWorkloadConfigPath)
}
