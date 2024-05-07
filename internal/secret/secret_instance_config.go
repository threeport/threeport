package secret

import (
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	workloadutil "github.com/threeport/threeport/internal/workload/util"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	v1 "github.com/threeport/threeport/pkg/api/v1"
	client "github.com/threeport/threeport/pkg/client/v0"
	client_v1 "github.com/threeport/threeport/pkg/client/v1"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// SecretInstanceConfig contains the configuration for a secret instance
// reconcile function.
type SecretInstanceConfig struct {
	r                           *controller.Reconciler
	secretInstance              *v0.SecretInstance
	secretDefinition            *v0.SecretDefinition
	log                         *logr.Logger
	workloadInstance            *v1.WorkloadInstance
	helmWorkloadInstance        *v0.HelmWorkloadInstance
	kubernetesRuntimeInstance   *v0.KubernetesRuntimeInstance
	kubernetesRuntimeDefinition *v0.KubernetesRuntimeDefinition
	workloadInstanceType        string
	workloadInstanceId          *uint
}

// getSecretInstanceOperations returns the operations
// for a secret instance
func (c *SecretInstanceConfig) getSecretInstanceOperations() *util.Operations {
	operations := util.Operations{}

	// append attached object operations
	operations.AppendOperation(util.Operation{
		Name:   "attached object reference",
		Create: c.createAttachedObjectReference,
		Delete: c.deleteAttachedObjectReference,
	})

	// append secret instance operations
	operations.AppendOperation(util.Operation{
		Name:   "secret objects",
		Create: c.createSecretObjects,
		Delete: func() error {
			if err := c.deleteSecretObjects(); err != nil && !errors.Is(err, client.ErrObjectNotFound) {
				return fmt.Errorf("failed to delete secret objects: %w", err)
			}
			return nil
		},
	})

	return &operations
}

// createAttachedObjectReference creates an attached object reference
// for the secret instance.
func (c *SecretInstanceConfig) createAttachedObjectReference() error {
	if err := client_v1.EnsureAttachedObjectReferenceExists(
		c.r.APIClient,
		c.r.APIServer,
		c.workloadInstanceType,
		c.workloadInstanceId,
		util.TypeName(*c.secretInstance),
		c.secretInstance.ID,
	); err != nil {
		return fmt.Errorf("failed to ensure attached object reference exists: %w", err)
	}

	return nil
}

// deleteAttachedObjectReference deletes an attached object reference
// for the secret instance.
func (c *SecretInstanceConfig) deleteAttachedObjectReference() error {
	attachedObjectReference, err := client_v1.GetAttachedObjectReferenceByAttachedObjectID(
		c.r.APIClient,
		c.r.APIServer,
		*c.secretInstance.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to get attached object references by attached object id: %w", err)
	}
	if _, err := client.DeleteAttachedObjectReference(
		c.r.APIClient,
		c.r.APIServer,
		*attachedObjectReference.ID,
	); err != nil && !errors.Is(err, client.ErrObjectNotFound) {
		return fmt.Errorf("failed to delete attached object reference: %w", err)
	}

	return nil
}

// createSecretObjects creates a secret instance
func (c *SecretInstanceConfig) createSecretObjects() error {

	// configure secret json manifests
	secretObjects, err := c.getSecretObjects()
	if err != nil {
		return fmt.Errorf("failed to configure secret controller json manifests: %w", err)
	}

	// update workload with secret manifests
	switch c.workloadInstanceType {
	case util.TypeName(v1.WorkloadInstance{}):
		// get workload instance
		workloadInstance, err := client.GetWorkloadInstanceByID(
			c.r.APIClient,
			c.r.APIServer,
			*c.secretInstance.WorkloadInstanceID,
		)
		if err != nil {
			return fmt.Errorf("failed to get workload instance: %w", err)
		}

		// create workload resource instances
		for _, secretObject := range secretObjects {
			jsonDefinition, err := util.UnstructuredToDatatypesJson(secretObject)
			if err != nil {
				return fmt.Errorf("failed to convert unstructured object to datatypes.JSON: %w", err)
			}
			workloadResourceInstance := v0.WorkloadResourceInstance{
				WorkloadInstanceID: c.workloadInstanceId,
				JSONDefinition:     &jsonDefinition,
			}
			_, err = client.CreateWorkloadResourceInstance(
				c.r.APIClient,
				c.r.APIServer,
				&workloadResourceInstance,
			)
			if err != nil {
				return fmt.Errorf("failed to create workload resource instance: %w", err)
			}
		}

		// trigger workload instance reconciliation
		workloadInstance.Reconciled = util.Ptr(false)
		_, err = client.UpdateWorkloadInstance(c.r.APIClient, c.r.APIServer, workloadInstance)
		if err != nil {
			return fmt.Errorf("failed to update workload instance: %w", err)
		}
	case util.TypeName(v0.HelmWorkloadInstance{}):
		helmWorkloadInstance, err := client.GetHelmWorkloadInstanceByID(
			c.r.APIClient,
			c.r.APIServer,
			*c.secretInstance.HelmWorkloadInstanceID,
		)
		if err != nil {
			return fmt.Errorf("failed to get helm workload instance: %w", err)
		}

		// update namespace of secret objects
		for _, object := range secretObjects {
			object.SetNamespace(*helmWorkloadInstance.ReleaseNamespace)
		}

		// convert unstructured objects to datatypes.JSON slice
		appendedResources, err := util.UnstructuredListToDatatypesJsonSlice(secretObjects)
		if err != nil {
			return fmt.Errorf("failed to convert unstructured list to datatypes.JSON slice: %w", err)
		}

		// append to existing resources
		if helmWorkloadInstance.AdditionalResources != nil {
			appendedResources = append(*helmWorkloadInstance.AdditionalResources, appendedResources...)
		}

		helmWorkloadInstance.AdditionalResources = &appendedResources
		helmWorkloadInstance.Reconciled = util.Ptr(false)
		_, err = client.UpdateHelmWorkloadInstance(c.r.APIClient, c.r.APIServer, helmWorkloadInstance)
		if err != nil {
			return fmt.Errorf("failed to update helm workload instance: %w", err)
		}
	default:
		return errors.New("secret instance must be attached to a workload instance or a helm workload instance")
	}

	return nil
}

// deleteSecretObjects deletes a secret instance
func (c *SecretInstanceConfig) deleteSecretObjects() error {

	// configure secret json manifests
	secretObjects, err := c.getSecretObjects()
	if err != nil {
		return fmt.Errorf("failed to configure secret controller json manifests: %w", err)
	}

	// update workload with secret manifests
	switch c.workloadInstanceType {
	case util.TypeName(v1.WorkloadInstance{}):
		// get workload instance
		workloadInstance, err := client.GetWorkloadInstanceByID(
			c.r.APIClient,
			c.r.APIServer,
			*c.workloadInstanceId,
		)
		if err != nil {
			if errors.Is(err, client.ErrObjectNotFound) {
				return nil
			}
			return fmt.Errorf("failed to get workload instance: %w", err)
		}

		// get workload resource instances
		workloadResourceInstances, err := client.GetWorkloadResourceInstancesByWorkloadInstanceID(
			c.r.APIClient,
			c.r.APIServer,
			*c.workloadInstanceId,
		)
		if err != nil {
			if errors.Is(err, client.ErrObjectNotFound) {
				return nil
			}
			return fmt.Errorf("failed to get workload resource instances by workload instance ID: %w", err)
		}

		// remove workload resource instances
		for _, secretObject := range secretObjects {

			// get workload resource instance for secret object
			workloadResourceInstance, err := workloadutil.GetUniqueWorkloadResourceInstanceByName(
				workloadResourceInstances,
				secretObject.GetKind(),
				secretObject.GetName(),
			)
			if err != nil {
				return fmt.Errorf("failed to get workload resource instance: %w", err)
			}

			// schedule workload resource instance for deletion
			workloadResourceInstance = &v0.WorkloadResourceInstance{
				Common:               v0.Common{ID: workloadResourceInstance.ID},
				ScheduledForDeletion: util.Ptr(time.Now().UTC()),
				Reconciled:           util.Ptr(false),
			}
			_, err = client.UpdateWorkloadResourceInstance(
				c.r.APIClient,
				c.r.APIServer,
				workloadResourceInstance,
			)
			if err != nil {
				if errors.Is(err, client.ErrObjectNotFound) {
					// workload resource instance has already been deleted
					return nil
				}
				return fmt.Errorf("failed to update workload resource instance: %w", err)
			}
		}

		// trigger workload instance reconciliation
		workloadInstance.Reconciled = util.Ptr(false)
		_, err = client.UpdateWorkloadInstance(
			c.r.APIClient,
			c.r.APIServer,
			workloadInstance,
		)
		if err != nil && !errors.Is(err, client.ErrObjectNotFound) {
			return fmt.Errorf("failed to update workload instance: %w", err)
		}
	case util.TypeName(v0.HelmWorkloadInstance{}):
		helmWorkloadInstance, err := client.GetHelmWorkloadInstanceByID(
			c.r.APIClient,
			c.r.APIServer,
			*c.secretInstance.HelmWorkloadInstanceID,
		)
		if err != nil {
			return fmt.Errorf("failed to get helm workload instance: %w", err)
		}

		// remove secret objects
		for _, object := range secretObjects {
			if err = util.RemoveDataTypesJsonFromDataTypesJsonSlice(
				object.GetName(),
				object.GetKind(),
				helmWorkloadInstance.AdditionalResources,
			); err != nil {
				return fmt.Errorf("failed to remove secret object from helm workload instance: %w", err)
			}
		}

		helmWorkloadInstance.Reconciled = util.Ptr(false)
		_, err = client.UpdateHelmWorkloadInstance(c.r.APIClient, c.r.APIServer, helmWorkloadInstance)
		if err != nil && !errors.Is(err, client.ErrObjectNotFound) {
			return fmt.Errorf("failed to update helm workload instance: %w", err)
		}
	default:
		return errors.New("secret instance must be attached to a workload instance or a helm workload instance")
	}

	return nil
}

// getSecretInstanceWorkloadTypeAndID returns the workload type and ID
// for a secret instance.
func getSecretInstanceWorkloadTypeAndId(secretInstance *v0.SecretInstance) (string, *uint, error) {
	var workloadInstanceType string
	var workloadInstanceID *uint

	switch {
	case secretInstance.WorkloadInstanceID != nil:
		workloadInstanceType = util.TypeName(v1.WorkloadInstance{})
		workloadInstanceID = secretInstance.WorkloadInstanceID
	case secretInstance.HelmWorkloadInstanceID != nil:
		workloadInstanceType = util.TypeName(v0.HelmWorkloadInstance{})
		workloadInstanceID = secretInstance.HelmWorkloadInstanceID
	default:
		return "", nil, errors.New("secret instance must be attached to a workload instance or a helm workload instance")
	}

	return workloadInstanceType, workloadInstanceID, nil
}

// getThreeportobjects returns all threeport objects required for
// gateway instance reconciliation
func (c *SecretInstanceConfig) getThreeportObjects() error {
	var err error

	// get kubernetes runtime instance
	c.kubernetesRuntimeInstance, err = client.GetKubernetesRuntimeInstanceByID(
		c.r.APIClient,
		c.r.APIServer,
		*c.secretInstance.KubernetesRuntimeInstanceID,
	)
	if err != nil {
		return fmt.Errorf("failed to get kubernetes runtime instance: %w", err)
	}

	// get kubernetes runtime definition
	c.kubernetesRuntimeDefinition, err = client.GetKubernetesRuntimeDefinitionByID(
		c.r.APIClient,
		c.r.APIServer,
		*c.kubernetesRuntimeInstance.KubernetesRuntimeDefinitionID,
	)
	if err != nil {
		return fmt.Errorf("failed to get kubernetes runtime definition: %w", err)
	}

	// get secret definition
	c.secretDefinition, err = client.GetSecretDefinitionByID(
		c.r.APIClient,
		c.r.APIServer,
		*c.secretInstance.SecretDefinitionID,
	)
	if err != nil {
		return fmt.Errorf("failed to get secret definition: %w", err)
	}

	// determine correct workload type and instance ID
	c.workloadInstanceType, c.workloadInstanceId, err = getSecretInstanceWorkloadTypeAndId(c.secretInstance)
	if err != nil {
		return fmt.Errorf("failed to determine workload type and instance ID: %w", err)
	}

	// update secret config with correct workload instance
	switch c.workloadInstanceType {
	case util.TypeName(v1.WorkloadInstance{}):
		// get workload instance
		c.workloadInstance, err = client_v1.GetWorkloadInstanceByID(
			c.r.APIClient,
			c.r.APIServer,
			*c.secretInstance.WorkloadInstanceID,
		)
		if err != nil {
			return fmt.Errorf("failed to get workload instance: %w", err)
		}
	case util.TypeName(v0.HelmWorkloadInstance{}):
		// get helm workload instance
		c.helmWorkloadInstance, err = client.GetHelmWorkloadInstanceByID(
			c.r.APIClient,
			c.r.APIServer,
			*c.secretInstance.HelmWorkloadInstanceID,
		)
		if err != nil {
			return fmt.Errorf("failed to get helm workload instance: %w", err)
		}
	default:
		return errors.New("secret instance must be attached to a workload instance or a helm workload instance")
	}

	return nil
}

// validateThreeportState validates the state of the threeport objects
// required for secret instance reconciliation
func (c *SecretInstanceConfig) validateThreeportState() error {
	// validate secret definition is reconciled
	if !*c.secretDefinition.Reconciled {
		return errors.New("secret definition not reconciled")
	}

	// validate workload is reconciled
	switch c.workloadInstanceType {
	case util.TypeName(v1.WorkloadInstance{}):
		if !*c.workloadInstance.Reconciled {
			return errors.New("workload instance not reconciled")
		}
	case util.TypeName(v0.HelmWorkloadInstance{}):
		if !*c.helmWorkloadInstance.Reconciled {
			return errors.New("helm workload instance not reconciled")
		}
	default:
		return errors.New("secret instance must be attached to a workload instance or a helm workload instance")
	}

	// confirm secret controller is deployed
	if err := c.confirmSecretControllerDeployed(); err != nil {
		return fmt.Errorf("failed to confirm secret controller deployed: %w", err)
	}

	return nil
}

// confirmSecretControllerDeployed confirms the secret controller
// is deployed
func (c *SecretInstanceConfig) confirmSecretControllerDeployed() error {
	if c.kubernetesRuntimeInstance.SecretsControllerInstanceID != nil {
		workloadInstance, err := client.GetWorkloadInstanceByID(
			c.r.APIClient,
			c.r.APIServer,
			*c.kubernetesRuntimeInstance.SecretsControllerInstanceID,
		)
		if err != nil {
			return fmt.Errorf("failed to get secret controller workload instance: %w", err)
		}
		if !*workloadInstance.Reconciled {
			return errors.New("secret controller not reconciled")
		}

		// secret controller is reconciled
		return nil
	}

	var externalSecretsYaml string
	var err error
	switch *c.kubernetesRuntimeDefinition.InfraProvider {
	case v0.KubernetesRuntimeInfraProviderEKS:
		resourceInventory, err := client.GetResourceInventoryByK8sRuntimeInst(
			c.r.APIClient,
			c.r.APIServer,
			c.kubernetesRuntimeInstance.ID,
		)
		if err != nil {
			return fmt.Errorf("failed to get dns management iam role arn: %w", err)
		}

		externalSecretsYaml, err = c.getExternalSecretsSupportServiceYaml(resourceInventory.SecretsManagerRole.RoleArn)
		if err != nil {
			return fmt.Errorf("failed to create external secrets: %w", err)
		}

	case v0.KubernetesRuntimeInfraProviderKind:
		externalSecretsYaml, err = c.getExternalSecretsSupportServiceYaml("testIamRoleArn")
		if err != nil {
			return fmt.Errorf("failed to create external secrets: %w", err)
		}
	}

	// create secret controller workload definition
	workloadDefName := fmt.Sprintf("%s-%s", "external-secrets", *c.kubernetesRuntimeInstance.Name)
	externalSecretsWorkloadDefinition := v0.WorkloadDefinition{
		Definition:   v0.Definition{Name: &workloadDefName},
		YAMLDocument: util.Ptr(externalSecretsYaml),
	}

	// create secret controller workload definition
	createdSecretControllerWorkloadDefinition, err := client.CreateWorkloadDefinition(
		c.r.APIClient,
		c.r.APIServer,
		&externalSecretsWorkloadDefinition,
	)
	if err != nil {
		return fmt.Errorf("failed to create secret controller workload definition: %w", err)
	}

	// create secret controller workload instance
	createdSecretControllerWorkloadInstance, err := client_v1.CreateWorkloadInstance(
		c.r.APIClient,
		c.r.APIServer,
		&v1.WorkloadInstance{
			Instance:                    v0.Instance{Name: &workloadDefName},
			WorkloadDefinitionID:        createdSecretControllerWorkloadDefinition.ID,
			KubernetesRuntimeInstanceID: c.kubernetesRuntimeInstance.ID,
		},
	)

	// update kubernetes runtime instance with secret controller instance ID
	c.kubernetesRuntimeInstance.SecretsControllerInstanceID = createdSecretControllerWorkloadInstance.ID
	if _, err = client.UpdateKubernetesRuntimeInstance(
		c.r.APIClient,
		c.r.APIServer,
		c.kubernetesRuntimeInstance,
	); err != nil {
		return fmt.Errorf("failed to update kubernetes runtime instance with secret controller instance ID: %w", err)
	}

	return nil
}

// getSecretObjects configures the secret controller
// json manifests
func (c *SecretInstanceConfig) getSecretObjects() ([]*unstructured.Unstructured, error) {
	var manifests []*unstructured.Unstructured

	// append secret store
	secretStoreJson, err := c.getSecretStore()
	if err != nil {
		return nil, fmt.Errorf("failed to get secret store: %w", err)
	}
	manifests = append(manifests, secretStoreJson)

	// append external secret
	manifests = append(manifests, c.getExternalSecret())

	return manifests, nil
}
