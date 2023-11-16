package v0

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/threeport/threeport/internal/workload/status"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// WorkloadConfig contains the config for a workload which is an abstraction of
// a workload definition and workload instance.
type WorkloadConfig struct {
	Workload WorkloadValues `yaml:"Workload"`
}

// WorkloadValues contains the attributes needed to manage a workload
// definition and workload instance.
type WorkloadValues struct {
	Name                      string                           `yaml:"Name"`
	YAMLDocument              string                           `yaml:"YAMLDocument"`
	WorkloadConfigPath        string                           `yaml:"WorkloadConfigPath"`
	KubernetesRuntimeInstance *KubernetesRuntimeInstanceValues `yaml:"KubernetesRuntimeInstance"`
	DomainName                *DomainNameDefinitionValues      `yaml:"DomainName"`
	Gateway                   *GatewayDefinitionValues         `yaml:"Gateway"`
	AwsRelationalDatabase     *AwsRelationalDatabaseValues     `yaml:"AwsRelationalDatabase"`
	AwsObjectStorageBucket    *AwsObjectStorageBucketValues    `yaml:"AwsObjectStorageBucket"`
}

// WorkloadDefinitionConfig contains the config for a workload definition.
type WorkloadDefinitionConfig struct {
	WorkloadDefinition WorkloadDefinitionValues `yaml:"WorkloadDefinition"`
}

// WorkloadDefinitionValues contains the attributes needed to manage a workload
// definition.
type WorkloadDefinitionValues struct {
	Name               string `yaml:"Name"`
	YAMLDocument       string `yaml:"YAMLDocument"`
	WorkloadConfigPath string `yaml:"WorkloadConfigPath"`
}

// WorkloadInstanceConfig contains the config for a workload instance.
type WorkloadInstanceConfig struct {
	WorkloadInstance WorkloadInstanceValues `yaml:"WorkloadInstance"`
}

// WorkloadInstanceValues contains the attributes needed to manage a workload
// instance.
type WorkloadInstanceValues struct {
	Name                      string                           `yaml:"Name"`
	KubernetesRuntimeInstance *KubernetesRuntimeInstanceValues `yaml:"KubernetesRuntimeInstance"`
	WorkloadDefinition        WorkloadDefinitionValues         `yaml:"WorkloadDefinition"`
}

// operation defines the interface for operations that
// have been made to the Threeport API and may need to
// be undone.
type operation interface {
	Delete(apiClient *http.Client, apiEndpoint string) error
}

// OperationStack contains a list of operations that have been
// performed on the Threeport API.
type OperationStack struct {
	Operations []operation
}

// Push adds an operation to the operation stack.
func (r *OperationStack) Push(operation operation) {
	r.Operations = append(r.Operations, operation)
}

// Pop removes an operation from the operation stack.
func (r *OperationStack) Pop() operation {
	if len(r.Operations) == 0 {
		return nil
	}
	lastIndex := len(r.Operations) - 1
	operation := r.Operations[lastIndex]
	r.Operations = r.Operations[:lastIndex]
	return operation
}

// cleanOnCreateError cleans up resources created during a create operation
func (r *OperationStack) cleanOnCreateError(apiClient *http.Client, apiEndpoint string, createErr error) error {

	multiError := MultiError{}
	multiError.AppendError(createErr)

	for {
		var operation operation
		if operation = r.Pop(); operation == nil {
			break
		}
		if err := operation.Delete(apiClient, apiEndpoint); err != nil {
			multiError.AppendError(err)
		}
	}
	return multiError.Error()
}

// MultiError is an error type that contains multiple errors.
type MultiError struct {
	Errors []error
}

// AppendError adds an error to the MultiError.
func (me *MultiError) AppendError(err error) {
	me.Errors = append(me.Errors, err)
}

// Error returns a string representation of the MultiError.
func (me MultiError) Error() error {
	errorMessages := make([]string, len(me.Errors))
	for i, err := range me.Errors {
		errorMessages[i] = err.Error()
	}
	return errors.New(strings.Join(errorMessages, "\n"))
}

// Create creates a workload definition and instance in the Threeport API.
func (w *WorkloadValues) Create(apiClient *http.Client, apiEndpoint string) (*v0.WorkloadDefinition, *v0.WorkloadInstance, error) {

	opStack := OperationStack{}

	// create the workload definition
	workloadDefinition := WorkloadDefinitionValues{
		Name:               w.Name,
		YAMLDocument:       w.YAMLDocument,
		WorkloadConfigPath: w.WorkloadConfigPath,
	}
	createdWorkloadDefinition, err := workloadDefinition.Create(apiClient, apiEndpoint)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create workload definition: %w", err)
	}
	opStack.Push(&workloadDefinition)

	// create the workload instance
	workloadInstance := WorkloadInstanceValues{
		Name:                      defaultInstanceName(w.Name),
		KubernetesRuntimeInstance: w.KubernetesRuntimeInstance,
		WorkloadDefinition: WorkloadDefinitionValues{
			Name: w.Name,
		},
	}
	createdWorkloadInstance, err := workloadInstance.Create(apiClient, apiEndpoint)
	if err != nil {
		return nil, nil, opStack.cleanOnCreateError(apiClient, apiEndpoint, err)
	}
	opStack.Push(&workloadInstance)

	if w.DomainName != nil && w.Gateway != nil {
		// create the domain name definition
		domainNameDefinition := DomainNameDefinitionValues{
			Name:       w.DomainName.Name,
			Zone:       w.DomainName.Zone,
			AdminEmail: w.DomainName.AdminEmail,
		}
		_, err = domainNameDefinition.CreateIfNotExist(apiClient, apiEndpoint)
		if err != nil {
			return nil, nil, opStack.cleanOnCreateError(apiClient, apiEndpoint, err)
		}
		opStack.Push(&domainNameDefinition)

		// create the domain name instance
		domainNameInstance := DomainNameInstanceValues{
			DomainNameDefinition:      domainNameDefinition,
			KubernetesRuntimeInstance: *w.KubernetesRuntimeInstance,
			WorkloadInstance:          workloadInstance,
		}
		_, err = domainNameInstance.Create(apiClient, apiEndpoint)
		if err != nil {
			return nil, nil, opStack.cleanOnCreateError(apiClient, apiEndpoint, err)
		}
		opStack.Push(&domainNameInstance)

		// create the gateway definition
		gatewayDefinition := GatewayDefinitionValues{
			Name:                 w.Gateway.Name,
			TCPPort:              w.Gateway.TCPPort,
			TLSEnabled:           w.Gateway.TLSEnabled,
			Path:                 w.Gateway.Path,
			ServiceName:          w.Gateway.ServiceName,
			DomainNameDefinition: domainNameDefinition,
		}
		_, err = gatewayDefinition.Create(apiClient, apiEndpoint)
		if err != nil {
			return nil, nil, opStack.cleanOnCreateError(apiClient, apiEndpoint, err)
		}
		opStack.Push(&gatewayDefinition)

		// create the gateway instance
		gatewayInstance := GatewayInstanceValues{
			GatewayDefinition:         gatewayDefinition,
			KubernetesRuntimeInstance: *w.KubernetesRuntimeInstance,
			WorkloadInstance:          workloadInstance,
		}
		_, err = gatewayInstance.Create(apiClient, apiEndpoint)
		if err != nil {
			return nil, nil, opStack.cleanOnCreateError(apiClient, apiEndpoint, err)
		}
		opStack.Push(&gatewayInstance)
	}

	// create AWS relational database
	// var awsRelationalDatabase AwsRelationalDatabaseValues
	if w.AwsRelationalDatabase != nil {
		awsRelationalDatabase := AwsRelationalDatabaseValues{
			Name:               w.AwsRelationalDatabase.Name,
			AwsAccountName:     w.AwsRelationalDatabase.AwsAccountName,
			Engine:             w.AwsRelationalDatabase.Engine,
			EngineVersion:      w.AwsRelationalDatabase.EngineVersion,
			DatabaseName:       w.AwsRelationalDatabase.DatabaseName,
			DatabasePort:       w.AwsRelationalDatabase.DatabasePort,
			BackupDays:         w.AwsRelationalDatabase.BackupDays,
			MachineSize:        w.AwsRelationalDatabase.MachineSize,
			StorageGb:          w.AwsRelationalDatabase.StorageGb,
			WorkloadSecretName: w.AwsRelationalDatabase.WorkloadSecretName,
			WorkloadInstance: &WorkloadInstanceValues{
				Name: defaultInstanceName(w.Name),
			},
		}
		_, _, err := awsRelationalDatabase.Create(apiClient, apiEndpoint)
		if err != nil {
			return nil, nil, opStack.cleanOnCreateError(apiClient, apiEndpoint, err)
		}
		opStack.Push(&awsRelationalDatabase)
	}

	// create AWS object storage bucket
	if w.AwsObjectStorageBucket != nil {
		awsObjectStorageBucket := AwsObjectStorageBucketValues{
			Name:                       w.AwsObjectStorageBucket.Name,
			AwsAccountName:             w.AwsObjectStorageBucket.AwsAccountName,
			PublicReadAccess:           w.AwsObjectStorageBucket.PublicReadAccess,
			WorkloadServiceAccountName: w.AwsObjectStorageBucket.WorkloadServiceAccountName,
			WorkloadBucketEnvVar:       w.AwsObjectStorageBucket.WorkloadBucketEnvVar,
			WorkloadInstance: &WorkloadInstanceValues{
				Name: defaultInstanceName(w.Name),
			},
		}
		_, _, err := awsObjectStorageBucket.Create(apiClient, apiEndpoint)
		if err != nil {
			return nil, nil, opStack.cleanOnCreateError(apiClient, apiEndpoint, err)
		}
		opStack.Push(&awsObjectStorageBucket)
	}

	return createdWorkloadDefinition, createdWorkloadInstance, nil
}

// Delete deletes a workload definition, workload instance,
// domain name definition, domain name instance,
// gateway definition, and gateway instance from the Threeport API.
func (w *WorkloadValues) Delete(apiClient *http.Client, apiEndpoint string) (*v0.WorkloadDefinition, *v0.WorkloadInstance, error) {
	if w.DomainName != nil {
		// get domain name instance by name
		domainNameInstance, err := client.GetDomainNameInstanceByName(apiClient, apiEndpoint, w.DomainName.Name)
		if err == nil {
			client.DeleteDomainNameInstance(apiClient, apiEndpoint, *domainNameInstance.ID)
		}

		// get domain name definition by name
		domainNameDefinition, err := client.GetDomainNameDefinitionByName(apiClient, apiEndpoint, w.DomainName.Name)
		if err == nil {
			client.DeleteDomainNameDefinition(apiClient, apiEndpoint, *domainNameDefinition.ID)
		}
	}

	if w.Gateway != nil {
		// get gateway instance by name
		gatewayInstance, err := client.GetGatewayInstanceByName(apiClient, apiEndpoint, w.Gateway.Name)
		if err != nil {
			client.DeleteGatewayInstance(apiClient, apiEndpoint, *gatewayInstance.ID)
		}

		// get gateway definition by name
		gatewayDefinition, err := client.GetGatewayDefinitionByName(apiClient, apiEndpoint, w.Gateway.Name)
		if err != nil {
			client.DeleteGatewayDefinition(apiClient, apiEndpoint, *gatewayDefinition.ID)
		}

	}

	// get workload instance by name
	workloadInstName := defaultInstanceName(w.Name)
	workloadInstance, err := client.GetWorkloadInstanceByName(apiClient, apiEndpoint, workloadInstName)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to find workload instance with name %s: %w", workloadInstName, err)
	}

	// get workload definition by name
	workloadDefinition, err := client.GetWorkloadDefinitionByName(apiClient, apiEndpoint, w.Name)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to find workload definition with name %s: %w", w.Name, err)
	}

	// ensure the workload definition has no more than one associated instance
	workloadDefInsts, err := client.GetWorkloadInstancesByWorkloadDefinitionID(apiClient, apiEndpoint, *workloadDefinition.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get workload instances by workload definition with ID: %d: %w", workloadDefinition.ID, err)
	}
	if len(*workloadDefInsts) > 1 {
		err = errors.New("deletion using the workload abstraction is only permitted when there is a one-to-one workload definition and workload instance relationship")
		return nil, nil, fmt.Errorf("the workload definition has more than one workload instance associated: %w", err)
	}

	// delete workload instance
	deletedWorkloadInstance, err := client.DeleteWorkloadInstance(apiClient, apiEndpoint, *workloadInstance.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to delete workload instance from threeport API: %w", err)
	}

	// wait for workload instance to be reconciled
	deletedCheckAttempts := 0
	deletedCheckAttemptsMax := 30
	deletedCheckDurationSeconds := 1
	workloadInstanceDeleted := false
	for deletedCheckAttempts < deletedCheckAttemptsMax {
		_, err := client.GetWorkloadInstanceByID(apiClient, apiEndpoint, *workloadInstance.ID)
		if err != nil {
			if errors.Is(err, client.ErrorObjectNotFound) {
				workloadInstanceDeleted = true
				break
			} else {
				return nil, nil, fmt.Errorf("failed to get workload instance from API when checking deletion: %w", err)
			}
		}
		// no error means workload instance was found - hasn't yet been deleted
		deletedCheckAttempts += 1
		time.Sleep(time.Second * time.Duration(deletedCheckDurationSeconds))
	}
	if !workloadInstanceDeleted {
		return nil, nil, errors.New(fmt.Sprintf(
			"workload instance not deleted after %d seconds",
			deletedCheckAttemptsMax*deletedCheckDurationSeconds,
		))
	}

	// delete workload definition
	deletedWorkloadDefinition, err := client.DeleteWorkloadDefinition(apiClient, apiEndpoint, *workloadDefinition.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to delete workload definition from threeport API: %w", err)
	}

	return deletedWorkloadDefinition, deletedWorkloadInstance, nil
}

// Create creates a workload definition in the Threeport API.
func (wd *WorkloadDefinitionValues) Create(apiClient *http.Client, apiEndpoint string) (*v0.WorkloadDefinition, error) {
	// validate required fields
	if wd.Name == "" || wd.YAMLDocument == "" {
		return nil, errors.New("missing required field/s in config - required fields: Name, YAMLDocument")
	}

	// build the path to the YAML document relative to the user's working
	// directory
	configPath, _ := filepath.Split(wd.WorkloadConfigPath)
	relativeYamlPath := path.Join(configPath, wd.YAMLDocument)

	// load YAML document
	//definitionContent, err := os.ReadFile(wd.YAMLDocument)
	definitionContent, err := os.ReadFile(relativeYamlPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read definition YAMLDocument file %s: %w", wd.YAMLDocument, err)
	}
	stringContent := string(definitionContent)

	// construct workload definition object
	workloadDefinition := v0.WorkloadDefinition{
		Definition: v0.Definition{
			Name: &wd.Name,
		},
		YAMLDocument: &stringContent,
	}

	// create workload definition
	createdWorkloadDefinition, err := client.CreateWorkloadDefinition(apiClient, apiEndpoint, &workloadDefinition)
	if err != nil {
		return nil, fmt.Errorf("failed to create workload definition in threeport API: %w", err)
	}

	return createdWorkloadDefinition, nil
}

// Delete deletes a workload definition from the Threeport API.
func (wd *WorkloadDefinitionValues) Delete(apiClient *http.Client, apiEndpoint string) error {
	// get workload definition by name
	workloadDefinition, err := client.GetWorkloadDefinitionByName(apiClient, apiEndpoint, wd.Name)
	if err != nil {
		return fmt.Errorf("failed to find workload definition with name %s: %w", wd.Name, err)
	}

	// delete workload definition
	_, err = client.DeleteWorkloadDefinition(apiClient, apiEndpoint, *workloadDefinition.ID)
	if err != nil {
		return fmt.Errorf("failed to delete workload definition from threeport API: %w", err)
	}

	return nil
}

// Create creates a workload instance in the Threeport API.
func (wi *WorkloadInstanceValues) Create(apiClient *http.Client, apiEndpoint string) (*v0.WorkloadInstance, error) {
	// validate required fields
	if wi.Name == "" || wi.WorkloadDefinition.Name == "" {
		return nil, errors.New("missing required field/s in config - required fields: Name, WorkloadDefinition.Name")
	}

	// get kubernetes runtime instance by name if provided, otherwise default kubernetes runtime
	var kubernetesRuntimeInstance v0.KubernetesRuntimeInstance
	if wi.KubernetesRuntimeInstance == nil {
		// get default kubernetes runtime instance
		kubernetesRuntimeInst, err := client.GetDefaultKubernetesRuntimeInstance(apiClient, apiEndpoint)
		if err != nil {
			return nil, fmt.Errorf("kubernetes runtime instance not provided and failed to find default kubernetes runtime instance: %w", err)
		}
		kubernetesRuntimeInstance = *kubernetesRuntimeInst
	} else {
		kubernetesRuntimeInst, err := client.GetKubernetesRuntimeInstanceByName(
			apiClient,
			apiEndpoint,
			wi.KubernetesRuntimeInstance.Name,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to find kubernetes runtime instance by name %s: %w", wi.KubernetesRuntimeInstance.Name, err)
		}
		kubernetesRuntimeInstance = *kubernetesRuntimeInst
	}

	// get workload definition by name
	workloadDefinition, err := client.GetWorkloadDefinitionByName(
		apiClient,
		apiEndpoint,
		wi.WorkloadDefinition.Name,
	)
	if err != nil {
		return nil, err
	}

	// construct workload instance object
	workloadInstance := v0.WorkloadInstance{
		Instance: v0.Instance{
			Name: &wi.Name,
		},
		KubernetesRuntimeInstanceID: kubernetesRuntimeInstance.ID,
		WorkloadDefinitionID:        workloadDefinition.ID,
	}

	// create workload instance
	createdWorkloadInstance, err := client.CreateWorkloadInstance(apiClient, apiEndpoint, &workloadInstance)
	if err != nil {
		return nil, fmt.Errorf("failed to create workload instance in threeport API: %w", err)
	}

	return createdWorkloadInstance, nil
}

// Describe returns important failure events related to a workload instance.
func (wi *WorkloadInstanceValues) Describe(apiClient *http.Client, apiEndpoint string) (*status.WorkloadInstanceStatusDetail, error) {
	// get workload instance by name
	workloadInstance, err := client.GetWorkloadInstanceByName(apiClient, apiEndpoint, wi.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to find workload instance with name %s: %w", wi.Name, err)
	}

	// get workload instance status
	statusDetail := status.GetWorkloadInstanceStatus(apiClient, apiEndpoint, workloadInstance)
	if statusDetail.Error != nil {
		return nil, fmt.Errorf("failed to get status for workload instance with name %s: %w", wi.Name, statusDetail.Error)
	}

	return statusDetail, nil
}

// Delete deletes a workload instance from the Threeport API.
func (wi *WorkloadInstanceValues) Delete(apiClient *http.Client, apiEndpoint string) error {
	// get workload instance by name
	workloadInstance, err := client.GetWorkloadInstanceByName(apiClient, apiEndpoint, wi.Name)
	if err != nil {
		return fmt.Errorf("failed to find workload instance with name %s: %w", wi.Name, err)
	}

	// delete workload instance
	_, err = client.DeleteWorkloadInstance(apiClient, apiEndpoint, *workloadInstance.ID)
	if err != nil {
		return fmt.Errorf("failed to delete workload instance from threeport API: %w", err)
	}

	// wait for workload instance to be deleted
	util.Retry(60, 1, func() error {
		if _, err := client.GetWorkloadInstanceByName(apiClient, apiEndpoint, wi.Name); err == nil {
			return errors.New("workload instance not deleted")
		}
		return nil
	})

	return nil
}
