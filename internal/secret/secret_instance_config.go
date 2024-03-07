package secret

import (
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	"gorm.io/datatypes"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
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
	workloadInstance            *v0.WorkloadInstance
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
		Name: "secret object",
		Create: c.createSecretObject,
		Delete: c.deleteSecretObject,
	})

	return &operations
}

// createAttachedObjectReference creates an attached object reference
// for the secret instance.
func (c *SecretInstanceConfig) createAttachedObjectReference() error {
	if err := client.EnsureAttachedObjectReferenceExists(
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
	attachedObjectReference, err := client.GetAttachedObjectReferenceByObjectID(
		c.r.APIClient,
		c.r.APIServer,
		*c.workloadInstance.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to get attached object references by workload instance ID: %w", err)
	}
	if _, err := client.DeleteAttachedObjectReference(
		c.r.APIClient,
		c.r.APIServer,
		*attachedObjectReference.ObjectID,
	); err != nil && !errors.Is(err, client.ErrObjectNotFound) {
		return fmt.Errorf("failed to delete attached object reference: %w", err)
	}

	return nil
}

// createSecretObject creates a secret instance
func (c *SecretInstanceConfig) createSecretObject() error {

	// configure secret json manifests
	jsonManifests, err := c.configureSecretControllerJsonManifests()
	if err != nil {
		return fmt.Errorf("failed to configure secret controller json manifests: %w", err)
	}

	// update workload with secret manifests
	switch c.workloadInstanceType {
	case util.TypeName(v0.WorkloadInstance{}):
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
		for _, jsonManifest := range jsonManifests {
			workloadResourceInstance := v0.WorkloadResourceInstance{
				WorkloadInstanceID: c.workloadInstanceId,
				JSONDefinition:     &jsonManifest,
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
		workloadInstance.Reconciled = util.BoolPtr(false)
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
		var appendedResources datatypes.JSONSlice[datatypes.JSON]
		if helmWorkloadInstance.AdditionalResources == nil {
			appendedResources = jsonManifests
		} else {
			appendedResources = append(*helmWorkloadInstance.AdditionalResources, jsonManifests...)
		}

		// update namespaces
		for index, jsonManifest := range appendedResources {
			updatedJsonManifest, err := util.UpdateNamespace(jsonManifest, *helmWorkloadInstance.ReleaseNamespace)
			if err != nil {
				return fmt.Errorf("failed to update namespace: %w", err)
			}
			appendedResources[index] = updatedJsonManifest
		}

		helmWorkloadInstance.AdditionalResources = &appendedResources
		helmWorkloadInstance.Reconciled = util.BoolPtr(false)
		_, err = client.UpdateHelmWorkloadInstance(c.r.APIClient, c.r.APIServer, helmWorkloadInstance)
		if err != nil {
			return fmt.Errorf("failed to update helm workload instance: %w", err)
		}
	default:
		return errors.New("secret instance must be attached to a workload instance or a helm workload instance")
	}

	return nil
}

// deleteSecretObject deletes a secret instance
func (c *SecretInstanceConfig) deleteSecretObject() error {

	return nil
}
