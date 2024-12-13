package v0

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/threeport/threeport/internal/agent"
	"github.com/threeport/threeport/internal/workload/status"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
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
	Name                      *string                          `yaml:"Name"`
	YAMLDocument              *string                          `yaml:"YAMLDocument"`
	WorkloadConfigPath        *string                          `yaml:"WorkloadConfigPath"`
	KubernetesRuntimeInstance *KubernetesRuntimeInstanceValues `yaml:"KubernetesRuntimeInstance"`
	DomainName                *DomainNameDefinitionValues      `yaml:"DomainName"`
	Gateway                   *GatewayDefinitionValues         `yaml:"Gateway"`
	AwsRelationalDatabase     *AwsRelationalDatabaseValues     `yaml:"AwsRelationalDatabase"`
	AwsObjectStorageBucket    *AwsObjectStorageBucketValues    `yaml:"AwsObjectStorageBucket"`
	Secret                    *SecretValues                    `yaml:"Secret"`
}

// WorkloadDefinitionConfig contains the config for a workload definition.
type WorkloadDefinitionConfig struct {
	WorkloadDefinition WorkloadDefinitionValues `yaml:"WorkloadDefinition"`
}

// WorkloadDefinitionValues contains the attributes needed to manage a workload
// definition.
type WorkloadDefinitionValues struct {
	Name               *string `yaml:"Name"`
	YAMLDocument       *string `yaml:"YAMLDocument"`
	WorkloadConfigPath *string `yaml:"WorkloadConfigPath"`
}

// WorkloadInstanceConfig contains the config for a workload instance.
type WorkloadInstanceConfig struct {
	WorkloadInstance WorkloadInstanceValues `yaml:"WorkloadInstance"`
}

// WorkloadInstanceValues contains the attributes needed to manage a workload
// instance.
type WorkloadInstanceValues struct {
	Name                      *string                          `yaml:"Name"`
	KubernetesRuntimeInstance *KubernetesRuntimeInstanceValues `yaml:"KubernetesRuntimeInstance"`
	WorkloadDefinition        *WorkloadDefinitionValues        `yaml:"WorkloadDefinition"`
}

// Create creates a workload definition and instance in the Threeport API.
func (w *WorkloadValues) Create(apiClient *http.Client, apiEndpoint string) (*v0.WorkloadDefinition, *v0.WorkloadInstance, error) {

	// get operations
	operations, createdWorkloadDefinition, createdWorkloadInstance := w.GetOperations(apiClient, apiEndpoint)

	// execute create operations
	if err := operations.Create(); err != nil {
		return nil, nil, fmt.Errorf(
			"failed to execute create operations for workload defined instance with name %s: %w",
			*w.Name,
			err,
		)
	}

	return createdWorkloadDefinition, createdWorkloadInstance, nil
}

// Delete deletes a workload definition, workload instance, domain name definition,
// domain name instance, gateway definition, and gateway instance from the Threeport API.
func (w *WorkloadValues) Delete(apiClient *http.Client, apiEndpoint string) (*v0.WorkloadDefinition, *v0.WorkloadInstance, error) {

	// get operation
	operations, _, _ := w.GetOperations(apiClient, apiEndpoint)

	// execute delete operations
	if err := operations.Delete(); err != nil {
		return nil, nil, fmt.Errorf(
			"failed to execute delete operations for workload defined instance with name %s: %w",
			*w.Name,
			err,
		)
	}

	return nil, nil, nil
}

// Create creates a workload definition in the Threeport API.
func (wd *WorkloadDefinitionValues) Create(apiClient *http.Client, apiEndpoint string) (*v0.WorkloadDefinition, error) {
	// validate required fields
	if wd.Name == nil || wd.YAMLDocument == nil {
		return nil, errors.New("missing required field/s in config - required fields: Name, YAMLDocument")
	}

	// build the path to the YAML document relative to the user's working directory
	configPath, _ := filepath.Split(*wd.WorkloadConfigPath)
	relativeYamlPath := path.Join(configPath, *wd.YAMLDocument)

	// load YAML document
	definitionContent, err := os.ReadFile(relativeYamlPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read definition YAMLDocument file with name %s: %w", *wd.YAMLDocument, err)
	}
	stringContent := string(definitionContent)

	// construct workload definition object
	workloadDefinition := v0.WorkloadDefinition{
		Definition: v0.Definition{
			Name: wd.Name,
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

// Describe returns details related to a workload definition.
func (wd *WorkloadDefinitionValues) Describe(apiClient *http.Client, apiEndpoint string) (*status.WorkloadDefinitionStatusDetail, error) {
	// validate
	if wd.Name == nil {
		return nil, errors.New("missing required field: Name")
	}

	// get workload definition by name
	workloadDefinition, err := client.GetWorkloadDefinitionByName(apiClient, apiEndpoint, *wd.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to find workload definition with name %s: %w", *wd.Name, err)
	}

	// get workload definition status
	statusDetail, err := status.GetWorkloadDefinitionStatus(
		apiClient,
		apiEndpoint,
		*workloadDefinition.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get status for workload definition with name %s: %w", *wd.Name, err)
	}

	return statusDetail, nil
}

// Delete deletes a workload definition from the Threeport API.
func (wd *WorkloadDefinitionValues) Delete(apiClient *http.Client, apiEndpoint string) (*v0.WorkloadDefinition, error) {
	// validate
	if wd.Name == nil {
		return nil, errors.New("missing required field: Name")
	}

	// get workload definition by name
	workloadDefinition, err := client.GetWorkloadDefinitionByName(apiClient, apiEndpoint, *wd.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to find workload definition with name %s: %w", *wd.Name, err)
	}

	// delete workload definition
	deletedWorkloadDefinition, err := client.DeleteWorkloadDefinition(apiClient, apiEndpoint, *workloadDefinition.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to delete workload definition from threeport API: %w", err)
	}

	return deletedWorkloadDefinition, nil
}

// Create creates a workload instance in the Threeport API.
func (wi *WorkloadInstanceValues) Create(apiClient *http.Client, apiEndpoint string) (*v0.WorkloadInstance, error) {
	// validate required fields
	if wi.Name == nil || wi.WorkloadDefinition == nil || wi.WorkloadDefinition.Name == nil {
		return nil, errors.New("missing required field/s in config - required fields: Name, WorkloadDefinition.Name")
	}

	// get kubernetes runtime instance API object
	kubernetesRuntimeInstance, err := SetKubernetesRuntimeInstanceForConfig(
		wi.KubernetesRuntimeInstance,
		apiClient,
		apiEndpoint,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to set kubernetes runtime instance: %w", err)
	}

	// get workload definition by name
	workloadDefinition, err := client.GetWorkloadDefinitionByName(
		apiClient,
		apiEndpoint,
		*wi.WorkloadDefinition.Name,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get workload definition by name %s: %w", *wi.WorkloadDefinition.Name, err)
	}

	// check to see if threeport is managing namespace
	jsonObjects, err := kube.GetJsonResourcesFromYamlDoc(*workloadDefinition.YAMLDocument)
	threeportManagedNs := true
	for _, jsonContent := range jsonObjects {
		kubeObject := &unstructured.Unstructured{Object: map[string]interface{}{}}
		if err := kubeObject.UnmarshalJSON(jsonContent); err != nil {
			return nil, fmt.Errorf("failed to unmarshal json to kubernetes unstructured object: %w", err)
		}
		if kubeObject.GetKind() == "Namespace" {
			threeportManagedNs = false
		}
	}

	if !threeportManagedNs {
		// if client managed namespaces, get instances for definition
		instances, err := client.GetWorkloadInstancesByWorkloadDefinitionID(
			apiClient,
			apiEndpoint,
			*workloadDefinition.ID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to get workload instances by workload definition ID %d: %w", *workloadDefinition.ID, err)
		}

		// if there's already an instance for a workload with client managed
		// namespace, check clusters
		var runtimeNames []string
		for _, inst := range *instances {
			runtime, err := client.GetKubernetesRuntimeInstanceByID(
				apiClient,
				apiEndpoint,
				*inst.KubernetesRuntimeInstanceID,
			)
			if err != nil {
				return nil, fmt.Errorf("failed to get kubernetes runtime instance ID: %w", err)
			}
			runtimeNames = append(runtimeNames, *runtime.Name)
		}
		for _, rName := range runtimeNames {
			// if the workload instance is using a cluster that already has an
			// instance for this definition, return error
			if rName == *kubernetesRuntimeInstance.Name {
				return nil, errors.New("only one workload instance per Kubernetes runtime may be deployed when a Kubernetes namespace is included in the workload definition YAMLDocument\nif you would like to deploy this workload to the same Kubernetes runtime (and continue to manage namespaces), you will need a new workload definition that uses a different namespace")
			}
		}
	}

	// construct workload instance object
	workloadInstance := v0.WorkloadInstance{
		Instance: v0.Instance{
			Name: wi.Name,
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
	// validate
	if wi.Name == nil {
		return nil, errors.New("missing required field: Name")
	}

	// get workload instance by name
	workloadInstance, err := client.GetWorkloadInstanceByName(apiClient, apiEndpoint, *wi.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get workload instance with name %s: %w", *wi.Name, err)
	}

	// get workload instance status
	statusDetail := status.GetWorkloadInstanceStatus(
		apiClient,
		apiEndpoint,
		agent.WorkloadInstanceType,
		*workloadInstance.ID,
		*workloadInstance.Reconciled,
	)
	if statusDetail.Error != nil {
		return nil, fmt.Errorf("failed to get status for workload instance by name %s: %w", *wi.Name, statusDetail.Error)
	}

	return statusDetail, nil
}

// Delete deletes a workload instance from the Threeport API.
func (wi *WorkloadInstanceValues) Delete(apiClient *http.Client, apiEndpoint string) (*v0.WorkloadInstance, error) {
	// validate
	if wi.Name == nil {
		return nil, errors.New("missing required field: Name")
	}

	// get workload instance by name
	workloadInstance, err := client.GetWorkloadInstanceByName(apiClient, apiEndpoint, *wi.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get workload instance by name %s: %w", *wi.Name, err)
	}

	// delete workload instance
	deletedWorkloadInstance, err := client.DeleteWorkloadInstance(apiClient, apiEndpoint, *workloadInstance.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to delete workload instance from threeport API: %w", err)
	}

	// wait for workload instance to be deleted
	util.Retry(60, 1, func() error {
		if _, err := client.GetWorkloadInstanceByName(apiClient, apiEndpoint, *wi.Name); err == nil {
			return errors.New("workload instance not deleted")
		}
		return nil
	})

	return deletedWorkloadInstance, nil
}

// GetOperations returns a slice of operations used to create or delete a
// workload.
func (w *WorkloadValues) GetOperations(apiClient *http.Client, apiEndpoint string) (*util.Operations, *v0.WorkloadDefinition, *v0.WorkloadInstance) {

	var err error
	var createdWorkloadInstance v0.WorkloadInstance
	var createdWorkloadDefinition v0.WorkloadDefinition

	operations := util.Operations{}

	// add workload definition operation
	workloadDefinitionValues := WorkloadDefinitionValues{
		Name:               w.Name,
		YAMLDocument:       w.YAMLDocument,
		WorkloadConfigPath: w.WorkloadConfigPath,
	}
	operations.AppendOperation(util.Operation{
		Name: "workload definition",
		Create: func() error {
			workloadDefinition, err := workloadDefinitionValues.Create(apiClient, apiEndpoint)
			if err != nil {
				return fmt.Errorf("failed to create workload definition with name %s: %w", *w.Name, err)
			}
			createdWorkloadDefinition = *workloadDefinition
			return nil
		},
		Delete: func() error {
			_, err = workloadDefinitionValues.Delete(apiClient, apiEndpoint)
			if err != nil {
				return fmt.Errorf("failed to delete workload definition with name %s: %w", *w.Name, err)
			}
			return nil
		},
	})

	// add workload instance operation
	workloadInstanceValues := WorkloadInstanceValues{
		Name:                      w.Name,
		KubernetesRuntimeInstance: w.KubernetesRuntimeInstance,
		WorkloadDefinition: &WorkloadDefinitionValues{
			Name: w.Name,
		},
	}
	operations.AppendOperation(util.Operation{
		Name: "workload instance",
		Create: func() error {
			workloadInstance, err := workloadInstanceValues.Create(apiClient, apiEndpoint)
			if err != nil {
				return fmt.Errorf("failed to create workload instance with name %s: %w", *w.Name, err)
			}
			createdWorkloadInstance = *workloadInstance
			return nil
		},
		Delete: func() error {
			_, err = workloadInstanceValues.Delete(apiClient, apiEndpoint)
			if err != nil {
				return fmt.Errorf("failed to delete workload instance with name %s: %w", *w.Name, err)
			}
			return nil
		},
	})

	// add domain name if provided
	var domainNameDefinitionValues DomainNameDefinitionValues
	if w.DomainName != nil {
		// add domain name definition operation
		domainNameDefinitionValues = DomainNameDefinitionValues{
			Name:       w.DomainName.Name,
			Domain:     w.DomainName.Domain,
			Zone:       w.DomainName.Zone,
			AdminEmail: w.DomainName.AdminEmail,
		}
		operations.AppendOperation(util.Operation{
			Name: "domain name definition",
			Create: func() error {
				_, err = domainNameDefinitionValues.Create(apiClient, apiEndpoint)
				if err != nil {
					return fmt.Errorf("failed to create domain name definition with name %s: %w", w.DomainName.Name, err)
				}
				return nil
			},
			Delete: func() error {
				_, err = domainNameDefinitionValues.Delete(apiClient, apiEndpoint)
				if err != nil {
					return fmt.Errorf("failed to delete domain name definition with name %s: %w", w.DomainName.Name, err)
				}
				return nil
			},
		})

		// add domain name instance operation
		domainNameInstanceValues := DomainNameInstanceValues{
			Name:                      w.DomainName.Name,
			DomainNameDefinition:      &domainNameDefinitionValues,
			KubernetesRuntimeInstance: w.KubernetesRuntimeInstance,
			WorkloadInstance:          &workloadInstanceValues,
		}
		operations.AppendOperation(util.Operation{
			Name: "domain name instance",
			Create: func() error {
				_, err = domainNameInstanceValues.Create(apiClient, apiEndpoint)
				if err != nil {
					return fmt.Errorf("failed to create domain name instance with name %s: %w", w.DomainName.Name, err)
				}
				return nil
			},
			Delete: func() error {
				_, err = domainNameInstanceValues.Delete(apiClient, apiEndpoint)
				if err != nil {
					return fmt.Errorf("failed to delete domain name instance with name %s: %w", w.DomainName.Name, err)
				}
				return nil
			},
		})
	}

	// add gateway if provided
	if w.Gateway != nil {
		// add gateway definition operation
		gatewayDefinitionValues := GatewayDefinitionValues{
			Name:                 w.Gateway.Name,
			HttpPorts:            w.Gateway.HttpPorts,
			TcpPorts:             w.Gateway.TcpPorts,
			ServiceName:          w.Gateway.ServiceName,
			SubDomain:            w.Gateway.SubDomain,
			DomainNameDefinition: &domainNameDefinitionValues,
		}

		operations.AppendOperation(util.Operation{
			Name: "gateway definition",
			Create: func() error {
				_, err = gatewayDefinitionValues.Create(apiClient, apiEndpoint)
				if err != nil {
					return fmt.Errorf("failed to create gateway definition with name %s: %w", w.Gateway.Name, err)
				}
				return nil
			},
			Delete: func() error {
				_, err = gatewayDefinitionValues.Delete(apiClient, apiEndpoint)
				if err != nil {
					return fmt.Errorf("failed to delete gateway definition with name %s: %w", w.Gateway.Name, err)
				}
				return nil
			},
		})

		// add gateway instance operation
		gatewayInstanceValues := GatewayInstanceValues{
			Name:                      w.Gateway.Name,
			GatewayDefinition:         &gatewayDefinitionValues,
			KubernetesRuntimeInstance: w.KubernetesRuntimeInstance,
			WorkloadInstance:          &workloadInstanceValues,
		}
		operations.AppendOperation(util.Operation{
			Name: "gateway instance",
			Create: func() error {
				_, err = gatewayInstanceValues.Create(apiClient, apiEndpoint)
				if err != nil {
					return fmt.Errorf("failed to create gateway instance with name %s: %w", w.Gateway.Name, err)
				}
				return nil
			},
			Delete: func() error {
				_, err = gatewayInstanceValues.Delete(apiClient, apiEndpoint)
				if err != nil {
					return fmt.Errorf("failed to delete gateway instance with name %s: %w", w.Gateway.Name, err)
				}
				return nil
			},
		})
	}

	// add AWS relational database operation
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
				Name: w.Name,
			},
		}
		operations.AppendOperation(util.Operation{
			Name: "aws relational database",
			Create: func() error {
				_, _, err := awsRelationalDatabase.Create(apiClient, apiEndpoint)
				if err != nil {
					return fmt.Errorf("failed to create aws relational database with name %s: %w", w.AwsRelationalDatabase.Name, err)
				}
				return nil
			},
			Delete: func() error {
				_, _, err = awsRelationalDatabase.Delete(apiClient, apiEndpoint)
				if err != nil {
					return fmt.Errorf("failed to delete aws relational database with name %s: %w", w.AwsRelationalDatabase.Name, err)
				}
				return nil
			},
		})
	}

	// add AWS object storage bucket operation
	if w.AwsObjectStorageBucket != nil {
		awsObjectStorageBucket := AwsObjectStorageBucketValues{
			Name:                       w.AwsObjectStorageBucket.Name,
			AwsAccountName:             w.AwsObjectStorageBucket.AwsAccountName,
			PublicReadAccess:           w.AwsObjectStorageBucket.PublicReadAccess,
			WorkloadServiceAccountName: w.AwsObjectStorageBucket.WorkloadServiceAccountName,
			WorkloadBucketEnvVar:       w.AwsObjectStorageBucket.WorkloadBucketEnvVar,
			WorkloadInstance: &WorkloadInstanceValues{
				Name: w.Name,
			},
		}
		operations.AppendOperation(util.Operation{
			Name: "aws object storage bucket",
			Create: func() error {
				_, _, err := awsObjectStorageBucket.Create(apiClient, apiEndpoint)
				if err != nil {
					return fmt.Errorf("failed to create aws object storage bucket with name %s: %w", w.AwsObjectStorageBucket.Name, err)
				}
				return nil
			},
			Delete: func() error {
				_, _, err := awsObjectStorageBucket.Delete(apiClient, apiEndpoint)
				if err != nil {
					return fmt.Errorf("failed to delete aws object storage bucket with name %s: %w", w.AwsObjectStorageBucket.Name, err)
				}
				return nil
			},
		})
	}

	// add secret operation
	if w.Secret != nil {
		secret := SecretValues{
			Name:                      w.Secret.Name,
			AwsAccountName:            w.Secret.AwsAccountName,
			Data:                      w.Secret.Data,
			KubernetesRuntimeInstance: w.KubernetesRuntimeInstance,
			WorkloadInstance:          &workloadInstanceValues,
		}
		operations.AppendOperation(util.Operation{
			Name: "secret",
			Create: func() error {
				_, _, err := secret.Create(apiClient, apiEndpoint)
				if err != nil {
					return fmt.Errorf("failed to create secret defined instance with name %s: %w", w.Secret.Name, err)
				}
				return nil
			},
			Delete: func() error {
				_, _, err := secret.Delete(apiClient, apiEndpoint)
				if err != nil {
					return fmt.Errorf("failed to delete secret defined instance with name %s: %w", w.Secret.Name, err)
				}
				return nil
			},
		})
	}

	return &operations, &createdWorkloadDefinition, &createdWorkloadInstance
}
