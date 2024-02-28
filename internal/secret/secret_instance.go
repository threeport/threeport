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
	r                         *controller.Reconciler
	secretInstance            *v0.SecretInstance
	secretDefinition          *v0.SecretDefinition
	log                       *logr.Logger
	workloadInstance          *v0.WorkloadInstance
	helmWorkloadInstance      *v0.HelmWorkloadInstance
	kubernetesRuntimeInstance *v0.KubernetesRuntimeInstance
	workloadInstanceType      string
	workloadInstanceId        *uint
}

// secretInstanceCreated reconciles state for a new secret
// instance.
func secretInstanceCreated(
	r *controller.Reconciler,
	secretInstance *v0.SecretInstance,
	log *logr.Logger,
) (int64, error) {
	// configure secret instance config
	c := &SecretInstanceConfig{
		r:              r,
		secretInstance: secretInstance,
		log:            log,
	}

	// get threeport objects
	if err := c.getThreeportObjects(); err != nil {
		return 0, fmt.Errorf("failed to get threeport objects: %w", err)
	}

	// ensure attached object reference exists
	if err := client.EnsureAttachedObjectReferenceExists(
		r.APIClient,
		r.APIServer,
		c.workloadInstanceType,
		c.workloadInstanceId,
		util.TypeName(*secretInstance),
		secretInstance.ID,
	); err != nil {
		return 0, fmt.Errorf("failed to ensure attached object reference exists: %w", err)
	}

	// validate threeport state
	if err := c.validateThreeportState(); err != nil {
		return 0, fmt.Errorf("failed to validate threeport state: %w", err)
	}

	// configure secret json manifests
	jsonManifests, err := c.configureSecretControllerJsonManifests()
	if err != nil {
		return 0, fmt.Errorf("failed to configure secret controller json manifests: %w", err)
	}

	// update workload with secret manifests
	switch c.workloadInstanceType {
	case util.TypeName(v0.WorkloadInstance{}):
		// get workload instance
		workloadInstance, err := client.GetWorkloadInstanceByID(c.r.APIClient, c.r.APIServer, *c.secretInstance.WorkloadInstanceID)
		if err != nil {
			return 0, fmt.Errorf("failed to get workload instance: %w", err)
		}

		// create workload resource instances
		for _, jsonManifest := range jsonManifests {
			workloadResourceInstance := v0.WorkloadResourceInstance{
				WorkloadInstanceID: c.workloadInstanceId,
				JSONDefinition:     &jsonManifest,
			}
			_, err = client.CreateWorkloadResourceInstance(c.r.APIClient, c.r.APIServer, &workloadResourceInstance)
			if err != nil {
				return 0, fmt.Errorf("failed to create workload resource instance: %w", err)
			}
		}

		// trigger workload instance reconciliation
		workloadInstance.Reconciled = util.BoolPtr(false)
		_, err = client.UpdateWorkloadInstance(c.r.APIClient, c.r.APIServer, workloadInstance)
		if err != nil {
			return 0, fmt.Errorf("failed to update workload instance: %w", err)
		}
	case util.TypeName(v0.HelmWorkloadInstance{}):
		helmWorkloadInstance, err := client.GetHelmWorkloadInstanceByID(c.r.APIClient, c.r.APIServer, *c.secretInstance.HelmWorkloadInstanceID)
		if err != nil {
			return 0, fmt.Errorf("failed to get helm workload instance: %w", err)
		}
		var appendedResources datatypes.JSONSlice[datatypes.JSON]
		if helmWorkloadInstance.AdditionalResources == nil {
			appendedResources = jsonManifests
		} else {
			appendedResources = append(*helmWorkloadInstance.AdditionalResources, jsonManifests...)
		}

		// update namespaces
		for index, jsonManifest := range appendedResources {
			// appendedResources[index] =
			updatedJsonManifest, err := util.UpdateNamespace(jsonManifest, *helmWorkloadInstance.ReleaseNamespace)
			if err != nil {
				return 0, fmt.Errorf("failed to update namespace: %w", err)
			}
			appendedResources[index] = updatedJsonManifest
		}

		helmWorkloadInstance.AdditionalResources = &appendedResources
		helmWorkloadInstance.Reconciled = util.BoolPtr(false)
		_, err = client.UpdateHelmWorkloadInstance(c.r.APIClient, c.r.APIServer, helmWorkloadInstance)
		if err != nil {
			return 0, fmt.Errorf("failed to update helm workload instance: %w", err)
		}
	default:
		return 0, errors.New("secret instance must be attached to a workload instance or a helm workload instance")
	}

	return 0, nil
}

// secretInstanceCreated reconciles state for a secret instance
// instance when it is changed.
func secretInstanceUpdated(
	r *controller.Reconciler,
	secretInstance *v0.SecretInstance,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}

// secretInstanceCreated reconciles state for a secret instance
// instance when it is removed.
func secretInstanceDeleted(
	r *controller.Reconciler,
	secretInstance *v0.SecretInstance,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}

// getSecretInstanceWorkloadTypeAndID returns the workload type and ID
// for a secret instance.
func getSecretInstanceWorkloadTypeAndId(secretInstance *v0.SecretInstance) (string, *uint, error) {
	var workloadInstanceType string
	var workloadInstanceID *uint

	switch {
	case secretInstance.WorkloadInstanceID != nil:
		workloadInstanceType = util.TypeName(v0.WorkloadInstance{})
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
	c.kubernetesRuntimeInstance, err = client.GetKubernetesRuntimeInstanceByID(c.r.APIClient, c.r.APIServer, *c.secretInstance.KubernetesRuntimeInstanceID)
	if err != nil {
		return fmt.Errorf("failed to get kubernetes runtime instance: %w", err)
	}

	// get secret definition
	c.secretDefinition, err = client.GetSecretDefinitionByID(c.r.APIClient, c.r.APIServer, *c.secretInstance.SecretDefinitionID)
	if err != nil {
		return fmt.Errorf("failed to get gateway controller workload definition: %w", err)
	}

	// determine correct workload type and instance ID
	c.workloadInstanceType, c.workloadInstanceId, err = getSecretInstanceWorkloadTypeAndId(c.secretInstance)
	if err != nil {
		return fmt.Errorf("failed to determine workload type and instance ID: %w", err)
	}

	// update secret config with correct workload instance
	switch c.workloadInstanceType {
	case util.TypeName(v0.WorkloadInstance{}):
		// get workload instance
		c.workloadInstance, err = client.GetWorkloadInstanceByID(c.r.APIClient, c.r.APIServer, *c.secretInstance.WorkloadInstanceID)
		if err != nil {
			return fmt.Errorf("failed to get workload instance: %w", err)
		}
	case util.TypeName(v0.HelmWorkloadInstance{}):
		// get helm workload instance
		c.helmWorkloadInstance, err = client.GetHelmWorkloadInstanceByID(c.r.APIClient, c.r.APIServer, *c.secretInstance.HelmWorkloadInstanceID)
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
	case util.TypeName(v0.WorkloadInstance{}):
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
		workloadInstance, err := client.GetWorkloadInstanceByID(c.r.APIClient, c.r.APIServer, *c.kubernetesRuntimeInstance.SecretsControllerInstanceID)
		if err != nil {
			return fmt.Errorf("failed to get secret controller workload instance: %w", err)
		}
		if !*workloadInstance.Reconciled {
			return errors.New("secret controller not reconciled")
		}

		// secret controller is reconciled
		return nil
	}

	externalSecrets, err := util.MapStringInterfaceToString(getExternalSecretsSupportServiceManifest())
	if err != nil {
		return fmt.Errorf("failed to create external secrets: %w", err)
	}

	// create secret controller workload definition
	workloadDefName := fmt.Sprintf("%s-%s", "external-secrets", *c.kubernetesRuntimeInstance.Name)
	externalSecretsWorkloadDefinition := v0.WorkloadDefinition{
		Definition:   v0.Definition{Name: &workloadDefName},
		YAMLDocument: util.StringPtr(externalSecrets),
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
	createdSecretControllerWorkloadInstance, err := client.CreateWorkloadInstance(
		c.r.APIClient,
		c.r.APIServer,
		&v0.WorkloadInstance{
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

// configureSecretControllerJsonManifests configures the secret controller
// json manifests
func (c *SecretInstanceConfig) configureSecretControllerJsonManifests() ([]datatypes.JSON, error) {
	var manifests []datatypes.JSON

	awsSecretMarshaled, err := util.MarshalJSON(getAwssmSecret())
	if err != nil {
		return nil, fmt.Errorf("failed to marshal aws secret: %w", err)
	}
	manifests = append(manifests, awsSecretMarshaled)

	secretStoreMarshaled, err := util.MarshalJSON(getSecretStore())
	if err != nil {
		return nil, fmt.Errorf("failed to marshal secret store: %w", err)
	}
	manifests = append(manifests, secretStoreMarshaled)

	externalSecretMarshaled, err := util.MarshalJSON(getExternalSecret(*c.secretDefinition.Name))
	if err != nil {
		return nil, fmt.Errorf("failed to marshal external secret: %w", err)
	}
	manifests = append(manifests, externalSecretMarshaled)

	return manifests, nil
}
