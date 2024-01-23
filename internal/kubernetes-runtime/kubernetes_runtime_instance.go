package kubernetesruntime

import (
	"errors"
	"fmt"

	"github.com/go-logr/logr"

	"github.com/threeport/threeport/internal/kubernetes-runtime/mapping"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

type KubernetesRuntimeInstanceConfig struct {
	r                         *controller.Reconciler
	kubernetesRuntimeInstance *v0.KubernetesRuntimeInstance
	log                       *logr.Logger
}

// kubernetesRuntimeInstanceCreated reconciles state for a new kubernetes
// runtime instance.
func kubernetesRuntimeInstanceCreated(
	r *controller.Reconciler,
	kubernetesRuntimeInstance *v0.KubernetesRuntimeInstance,
	log *logr.Logger,
) (int64, error) {
	// if a cluster instance is created by another mechanism and being
	// registered in the system with Reconciled=true, there's no need to do
	// anything - return immediately without error
	if *kubernetesRuntimeInstance.Reconciled == true {
		return 0, nil
	}

	// get runtime definition
	kubernetesRuntimeDefinition, err := client.GetKubernetesRuntimeDefinitionByID(
		r.APIClient,
		r.APIServer,
		*kubernetesRuntimeInstance.KubernetesRuntimeDefinitionID,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to get kubernetes runtime definition by ID: %w", err)
	}

	// create the provider-specific instance
	switch *kubernetesRuntimeDefinition.InfraProvider {
	case v0.KubernetesRuntimeInfraProviderKind:
		// kind clusters not managed by k8s runtime controller
		return 0, nil
	case v0.KubernetesRuntimeInfraProviderEKS:
		// get AWS EKS runtime definition
		awsEksKubernetesRuntimeDefinition, err := client.GetAwsEksKubernetesRuntimeDefinitionByK8sRuntimeDef(
			r.APIClient,
			r.APIServer,
			*kubernetesRuntimeDefinition.ID,
		)
		if err != nil {
			return 0, fmt.Errorf("failed to get aws eks runtime definition by kubernetes runtime definition ID: %w", err)
		}

		// add AWS EKS runtime instance
		region, err := mapping.GetProviderRegionForLocation(util.AwsProvider, *kubernetesRuntimeInstance.Location)
		if err != nil {
			return 0, fmt.Errorf("failed to map threeport location to AWS region: %w", err)
		}
		awsEksKubernetesRuntimeInstance := v0.AwsEksKubernetesRuntimeInstance{
			Instance: v0.Instance{
				Name: kubernetesRuntimeInstance.Name,
			},
			Region:                              &region,
			KubernetesRuntimeInstanceID:         kubernetesRuntimeInstance.ID,
			AwsEksKubernetesRuntimeDefinitionID: awsEksKubernetesRuntimeDefinition.ID,
		}
		_, err = client.CreateAwsEksKubernetesRuntimeInstance(
			r.APIClient,
			r.APIServer,
			&awsEksKubernetesRuntimeInstance,
		)
		if err != nil {
			return 0, fmt.Errorf("failed to create EKS kubernetes runtime instance: %w", err)
		}
	}

	// configure kubernetes runtime instance config
	c := &KubernetesRuntimeInstanceConfig{
		r:                         r,
		kubernetesRuntimeInstance: kubernetesRuntimeInstance,
		log:                       log,
	}

	// configure observability
	if err := c.configureObservability(); err != nil {
		return 0, fmt.Errorf("failed to configure observability: %w", err)
	}

	// update kubernetes runtime instance with observability info
	kubernetesRuntimeInstance.Reconciled = util.BoolPtr(true)
	if _, err = client.UpdateKubernetesRuntimeInstance(
		r.APIClient,
		r.APIServer,
		kubernetesRuntimeInstance,
	); err != nil {
		return 0, fmt.Errorf("failed to update kubernetes runtime instance: %w", err)
	}

	return 0, nil
}

// kubernetesRuntimeInstanceUpdated reconciles state for a kubernetes
// runtime instance whenever it is changed.
func kubernetesRuntimeInstanceUpdated(
	r *controller.Reconciler,
	kubernetesRuntimeInstance *v0.KubernetesRuntimeInstance,
	log *logr.Logger,
) (int64, error) {
	// // check to see if we have API endpoint - no further reconciliation can
	// // occur until we have that
	// if kubernetesRuntimeInstance.APIEndpoint == nil {
	// 	return 0, nil
	// }

	// // check to see if kubernetes runtime is being deleted - if so no updates
	// // required
	// if kubernetesRuntimeInstance.DeletionScheduled != nil {
	// 	return 0, nil
	// }

	// // get runtime definition
	// kubernetesRuntimeDefinition, err := client.GetKubernetesRuntimeDefinitionByID(
	// 	r.APIClient,
	// 	r.APIServer,
	// 	*kubernetesRuntimeInstance.KubernetesRuntimeDefinitionID,
	// )
	// if err != nil {
	// 	return 0, fmt.Errorf("failed to retrieve kubernetes runtime definition by ID: %w", err)
	// }

	// // get kube client to install compute space control plane components
	// dynamicKubeClient, mapper, err := kube.GetClient(
	// 	kubernetesRuntimeInstance,
	// 	false,
	// 	r.APIClient,
	// 	r.APIServer,
	// 	r.EncryptionKey,
	// )
	// if err != nil {
	// 	return 0, fmt.Errorf("failed to get a Kubernetes client and mapper: %w", err)
	// }

	// // TODO: sort out an elegant way to pass the custom image info for
	// // install compute space control plane components
	// var agentImage string
	// if kubernetesRuntimeInstance.ThreeportAgentImage != nil {
	// 	agentImage = *kubernetesRuntimeInstance.ThreeportAgentImage
	// }

	// cpi := threeport.NewInstaller()

	// if agentImage != "" {
	// 	agentRegistry, _, agentTag, err := util.ParseImage(agentImage)
	// 	if err != nil {
	// 		return 0, fmt.Errorf("failed to parse custom threeport agent image: %w", err)
	// 	}

	// 	cpi.Opts.AgentInfo.ImageRepo = agentRegistry
	// 	cpi.Opts.AgentInfo.ImageTag = agentTag
	// }

	// // threeport control plane components
	// if err := cpi.InstallComputeSpaceControlPlaneComponents(
	// 	dynamicKubeClient,
	// 	mapper,
	// 	*kubernetesRuntimeInstance.Name,
	// ); err != nil {
	// 	return 0, fmt.Errorf("failed to insall compute space control plane components: %w", err)
	// }

	// // wait for kube API to persist the change and refresh the client and mapper
	// // this is necessary to have the updated REST mapping for the CRDs as the
	// // support services operator install includes one of those custom resources
	// time.Sleep(time.Second * 10)
	// dynamicKubeClient, mapper, err = kube.GetClient(
	// 	kubernetesRuntimeInstance,
	// 	false,
	// 	r.APIClient,
	// 	r.APIServer,
	// 	r.EncryptionKey,
	// )
	// if err != nil {
	// 	return 0, fmt.Errorf("failed to refresh dynamic kube API client: %w", err)
	// }

	// // support services operator
	// if err := threeport.InstallThreeportSupportServicesOperator(dynamicKubeClient, mapper); err != nil {
	// 	return 0, fmt.Errorf("failed to install support services operator: %w", err)
	// }

	// if *kubernetesRuntimeDefinition.InfraProvider == v0.KubernetesRuntimeInfraProviderEKS {
	// 	// get aws account
	// 	awsAccount, err := client.GetAwsAccountByName(
	// 		r.APIClient,
	// 		r.APIServer,
	// 		*kubernetesRuntimeDefinition.InfraProviderAccountName,
	// 	)
	// 	if err != nil {
	// 		return 0, fmt.Errorf("failed to get AWS account by name: %w", err)
	// 	}

	// 	// system components e.g. cluster-autoscaler
	// 	if err := threeport.InstallThreeportSystemServices(
	// 		dynamicKubeClient,
	// 		mapper,
	// 		*kubernetesRuntimeDefinition.InfraProvider,
	// 		*kubernetesRuntimeInstance.Name,
	// 		*awsAccount.AccountID,
	// 	); err != nil {
	// 		return 0, fmt.Errorf("failed to install system services: %w", err)
	// 	}
	// }

	// configure kubernetes runtime instance config
	c := &KubernetesRuntimeInstanceConfig{
		r:                         r,
		kubernetesRuntimeInstance: kubernetesRuntimeInstance,
		log:                       log,
	}

	// configure observability
	if err := c.configureObservability(); err != nil {
		return 0, fmt.Errorf("failed to configure observability: %w", err)
	}

	// update kubernetes runtime instance with observability info
	kubernetesRuntimeInstance.Reconciled = util.BoolPtr(true)
	if _, err := client.UpdateKubernetesRuntimeInstance(
		r.APIClient,
		r.APIServer,
		kubernetesRuntimeInstance,
	); err != nil {
		return 0, fmt.Errorf("failed to update kubernetes runtime instance: %w", err)
	}

	return 0, nil
}

// kubernetesRuntimeInstanceDeleted reconciles state for a kubernetes
// runtime instance whenever it is removed.
func kubernetesRuntimeInstanceDeleted(
	r *controller.Reconciler,
	kubernetesRuntimeInstance *v0.KubernetesRuntimeInstance,
	log *logr.Logger,
) (int64, error) {
	// check that deletion is scheduled - if not there's a problem
	if kubernetesRuntimeInstance.DeletionScheduled == nil {
		return 0, errors.New("deletion notification receieved but not scheduled")
	}

	// check to see if deletion confirmed - it should not be, but if so we should do no
	// more
	if kubernetesRuntimeInstance.DeletionConfirmed != nil {
		return 0, nil
	}

	// get runtime definition
	kubernetesRuntimeDefinition, err := client.GetKubernetesRuntimeDefinitionByID(
		r.APIClient,
		r.APIServer,
		*kubernetesRuntimeInstance.KubernetesRuntimeDefinitionID,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to retrieve kubernetes runtime definition by ID: %w", err)
	}

	// TODO: delete support services svc to elliminate ELB

	// delete kubernetes runtime instance
	switch *kubernetesRuntimeDefinition.InfraProvider {
	case v0.KubernetesRuntimeInfraProviderEKS:
		// get AWS EKS runtime instance
		awsEksKubernetesRuntimeInstance, err := client.GetAwsEksKubernetesRuntimeInstanceByK8sRuntimeInst(
			r.APIClient,
			r.APIServer,
			*kubernetesRuntimeInstance.ID,
		)
		if err != nil {
			return 0, fmt.Errorf("failed to get aws eks runtime instance by kubernetes runtime instance ID: %w", err)
		}

		// delete AWS EKS runtime instance
		_, err = client.DeleteAwsEksKubernetesRuntimeInstance(
			r.APIClient,
			r.APIServer,
			*awsEksKubernetesRuntimeInstance.ID,
		)
		if err != nil {
			return 0, fmt.Errorf("failed to delete aws eks runtime instance by ID: %w", err)
		}
	}

	return 0, nil
}

// configureObservability configures observability for a kubernetes runtime
func (c *KubernetesRuntimeInstanceConfig) configureObservability() error {
	// configure metrics
	switch {
	case *c.kubernetesRuntimeInstance.MetricsEnabled &&
		c.kubernetesRuntimeInstance.MetricsInstanceID == nil:
		c.log.Info("enabling metrics")

		// get metrics operations
		operations := c.getMetricsOperations(nil)

		// execute create metrics operations
		if err := operations.Create(); err != nil {
			return fmt.Errorf("failed to execute create metrics operations: %w", err)
		}

	case !*c.kubernetesRuntimeInstance.MetricsEnabled &&
		c.kubernetesRuntimeInstance.MetricsInstanceID != nil:
		c.log.Info("disabling metrics")

		// get metrics instance
		metricsInstance, err := client.GetMetricsInstanceByID(
			c.r.APIClient,
			c.r.APIServer,
			uint(c.kubernetesRuntimeInstance.MetricsInstanceID.Int64),
		)
		if err != nil {
			return fmt.Errorf("failed to get metrics definition by ID: %w", err)
		}

		// get metrics operations
		operations := c.getMetricsOperations(metricsInstance.MetricsDefinitionID)

		// execute delete metrics operations
		if err := operations.Delete(); err != nil {
			return fmt.Errorf("failed to execute delete metrics operations: %w", err)
		}
	}

	// configure logging
	switch {
	case *c.kubernetesRuntimeInstance.LoggingEnabled &&
		c.kubernetesRuntimeInstance.LoggingInstanceID == nil:
		c.log.Info("enabling logging")

		// get metrics operations
		operations := c.getLoggingOperations(nil)

		// execute create metrics operations
		if err := operations.Create(); err != nil {
			return fmt.Errorf("failed to execute create logging operations: %w", err)
		}

	case !*c.kubernetesRuntimeInstance.LoggingEnabled &&
		c.kubernetesRuntimeInstance.LoggingInstanceID != nil:
		c.log.Info("disabling logging")

		// get logging instance
		loggingInstance, err := client.GetLoggingInstanceByID(
			c.r.APIClient,
			c.r.APIServer,
			uint(c.kubernetesRuntimeInstance.LoggingInstanceID.Int64),
		)
		if err != nil {
			return fmt.Errorf("failed to get logging definition by ID: %w", err)
		}

		// get metrics operations
		operations := c.getLoggingOperations(loggingInstance.LoggingDefinitionID)

		// execute delete metrics operations
		if err := operations.Delete(); err != nil {
			return fmt.Errorf("failed to execute delete logging operations: %w", err)
		}

	}
	return nil
}
