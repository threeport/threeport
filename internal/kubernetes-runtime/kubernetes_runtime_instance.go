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
	if err := configureObservability(c); err != nil {
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
	if err := configureObservability(c); err != nil {
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
func configureObservability(c *KubernetesRuntimeInstanceConfig) error {
	// configure metrics
	switch {
	case *c.kubernetesRuntimeInstance.MetricsEnabled &&
		c.kubernetesRuntimeInstance.MetricsInstanceID == nil:
		c.log.Info("enabling metrics")

		// get metrics operations
		operations := getMetricsOperations(c, nil)

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
		operations := getMetricsOperations(c, metricsInstance.MetricsDefinitionID)

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
		operations := getLoggingOperations(c, nil)

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
		operations := getLoggingOperations(c, loggingInstance.LoggingDefinitionID)

		// execute delete metrics operations
		if err := operations.Delete(); err != nil {
			return fmt.Errorf("failed to execute delete logging operations: %w", err)
		}

	}
	return nil
}

// createLoggingDefinition configures a logging definition for a kubernetes runtime
// instance
func (c *KubernetesRuntimeInstanceConfig) createLoggingDefinition() (*uint, error) {
	// create logging definition
	createdLoggingDefinition, err := client.CreateLoggingDefinition(
		c.r.APIClient,
		c.r.APIServer,
		&v0.LoggingDefinition{
			Definition: v0.Definition{
				Name: c.kubernetesRuntimeInstance.Name,
			},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create logging definition: %w", err)
	}

	// wait for logging definition to be reconciled
	if err = util.Retry(120, 1, func() error {
		loggingDefinition, err := client.GetLoggingDefinitionByID(
			c.r.APIClient,
			c.r.APIServer,
			*createdLoggingDefinition.ID,
		)
		if err != nil {
			return fmt.Errorf("failed to get logging definition by ID: %w", err)
		}
		if !*loggingDefinition.Reconciled {
			return fmt.Errorf("logging definition not reconciled")
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("failed to wait for logging definition to be created: %w", err)
	}

	return createdLoggingDefinition.ID, nil
}

// createLoggingInstance configures a logging instance for a kubernetes runtime
// instance
func (c *KubernetesRuntimeInstanceConfig) createLoggingInstance(loggingDefinitionID *uint) error {
	// create logging instance
	createdLoggingInstance, err := client.CreateLoggingInstance(
		c.r.APIClient,
		c.r.APIServer,
		&v0.LoggingInstance{
			Instance: v0.Instance{
				Name: c.kubernetesRuntimeInstance.Name,
			},
			LoggingDefinitionID:         loggingDefinitionID,
			KubernetesRuntimeInstanceID: c.kubernetesRuntimeInstance.ID,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to create logging instance: %w", err)
	}

	// wait for logging instance to be reconciled
	if err = util.Retry(120, 1, func() error {
		loggingInstance, err := client.GetLoggingInstanceByID(
			c.r.APIClient,
			c.r.APIServer,
			*createdLoggingInstance.ID,
		)
		if err != nil {
			return fmt.Errorf("failed to get logging instance by ID: %w", err)
		}
		if !*loggingInstance.Reconciled {
			return fmt.Errorf("logging instance not reconciled")
		}
		return nil
	}); err != nil {
		return fmt.Errorf("failed to wait for logging instance to be created: %w", err)
	}

	// update kubernetes runtime instance with logging instance ID
	c.kubernetesRuntimeInstance.LoggingInstanceID = util.SqlNullInt64(createdLoggingInstance.ID)

	return nil
}

// deleteLoggingInstance disables logging instance for a kubernetes runtime
// instance
func (c *KubernetesRuntimeInstanceConfig) deleteLoggingInstance() error {
	// delete logging instance
	_, err := client.DeleteLoggingInstance(
		c.r.APIClient,
		c.r.APIServer,
		uint(c.kubernetesRuntimeInstance.LoggingInstanceID.Int64),
	)
	if err != nil && !errors.Is(err, client.ErrObjectNotFound) {
		return fmt.Errorf("failed to delete logging instance: %w", err)
	}

	// wait for logging instance to be deleted
	if err = util.Retry(120, 1, func() error {
		_, err = client.GetLoggingInstanceByID(
			c.r.APIClient,
			c.r.APIServer,
			uint(c.kubernetesRuntimeInstance.LoggingInstanceID.Int64),
		)
		if err != nil {
			if errors.Is(err, client.ErrObjectNotFound) {
				return nil
			}
			return fmt.Errorf("failed to get logging instance by ID: %w", err)
		}
		return fmt.Errorf("logging instance still exists")
	}); err != nil {
		return fmt.Errorf("failed to wait for logging instance to be deleted: %w", err)
	}

	return nil
}

// deleteLoggingDefinition disables logging instance for a kubernetes runtime
// instance
func (c *KubernetesRuntimeInstanceConfig) deleteLoggingDefinition(loggingDefinitionID *uint) error {
	// delete logging definition
	_, err := client.DeleteLoggingDefinition(
		c.r.APIClient,
		c.r.APIServer,
		*loggingDefinitionID,
	)
	if err != nil && !errors.Is(err, client.ErrObjectNotFound) {
		return fmt.Errorf("failed to delete logging definition: %w", err)
	}

	// wait for logging definition to be deleted
	if err = util.Retry(120, 1, func() error {
		_, err = client.GetLoggingDefinitionByID(
			c.r.APIClient,
			c.r.APIServer,
			*loggingDefinitionID,
		)
		if err != nil {
			if errors.Is(err, client.ErrObjectNotFound) {
				return nil
			}
			return fmt.Errorf("failed to get logging definition by ID: %w", err)
		}
		return fmt.Errorf("logging definition still exists")
	}); err != nil {
		return fmt.Errorf("failed to wait for logging definition to be deleted: %w", err)
	}

	c.kubernetesRuntimeInstance.LoggingInstanceID = util.SqlNullInt64(nil)
	return nil
}

// createMetricsDefinition configures a metrics definition for a kubernetes runtime
// instance
func (c *KubernetesRuntimeInstanceConfig) createMetricsDefinition() (*uint, error) {
	// create metrics definition
	createdMetricsDefinition, err := client.CreateMetricsDefinition(
		c.r.APIClient,
		c.r.APIServer,
		&v0.MetricsDefinition{
			Definition: v0.Definition{
				Name: c.kubernetesRuntimeInstance.Name,
			},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create metrics definition: %w", err)
	}

	// wait for metrics definition to be reconciled
	if err = util.Retry(120, 1, func() error {
		metricsDefinition, err := client.GetMetricsDefinitionByID(
			c.r.APIClient,
			c.r.APIServer,
			*createdMetricsDefinition.ID,
		)
		if err != nil {
			return fmt.Errorf("failed to get metrics definition by ID: %w", err)
		}
		if !*metricsDefinition.Reconciled {
			return fmt.Errorf("metrics definition not reconciled")
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("failed to wait for metrics definition to be created: %w", err)
	}

	return createdMetricsDefinition.ID, nil
}

// createMetricsInstance configures a metrics for a kubernetes runtime
// instance
func (c *KubernetesRuntimeInstanceConfig) createMetricsInstance(metricsDefinitionID *uint) error {
	// create metrics instance
	createdMetricsInstance, err := client.CreateMetricsInstance(
		c.r.APIClient,
		c.r.APIServer,
		&v0.MetricsInstance{
			Instance: v0.Instance{
				Name: c.kubernetesRuntimeInstance.Name,
			},
			MetricsDefinitionID:         metricsDefinitionID,
			KubernetesRuntimeInstanceID: c.kubernetesRuntimeInstance.ID,
		},
	)
	if err != nil && !errors.Is(err, client.ErrConflict) {
		return fmt.Errorf("failed to create metrics instance: %w", err)
	}

	// wait for metrics instance to be reconciled
	if err = util.Retry(120, 1, func() error {
		metricsInstance, err := client.GetMetricsInstanceByID(
			c.r.APIClient,
			c.r.APIServer,
			*createdMetricsInstance.ID,
		)
		if err != nil {
			return fmt.Errorf("failed to get metrics instance by ID: %w", err)
		}
		if !*metricsInstance.Reconciled {
			return fmt.Errorf("metrics instance not reconciled")
		}
		return nil
	}); err != nil {
		return fmt.Errorf("failed to wait for metrics instance to be created: %w", err)
	}

	// update kubernetes runtime instance with metrics instance ID
	c.kubernetesRuntimeInstance.MetricsInstanceID = util.SqlNullInt64(createdMetricsInstance.ID)

	return nil
}

// deleteMetricsInstance disables a metrics definition for a kubernetes runtime
// instance
func (c *KubernetesRuntimeInstanceConfig) deleteMetricsInstance() error {
	// delete metrics instance
	_, err := client.DeleteMetricsInstance(
		c.r.APIClient,
		c.r.APIServer,
		uint(c.kubernetesRuntimeInstance.MetricsInstanceID.Int64),
	)
	if err != nil && !errors.Is(err, client.ErrObjectNotFound) {
		return fmt.Errorf("failed to delete metrics instance: %w", err)
	}

	// wait for metrics instance to be deleted
	if err = util.Retry(120, 1, func() error {
		_, err = client.GetMetricsInstanceByID(
			c.r.APIClient,
			c.r.APIServer,
			uint(c.kubernetesRuntimeInstance.MetricsInstanceID.Int64),
		)
		if err != nil {
			if errors.Is(err, client.ErrObjectNotFound) {
				return nil
			}
			return fmt.Errorf("failed to get metrics instance by ID: %w", err)
		}
		return fmt.Errorf("metrics instance still exists")
	}); err != nil {
		return fmt.Errorf("failed to wait for metrics instance to be deleted: %w", err)
	}

	return nil
}

// deleteMetricsDefinition disables a metrics definition for a kubernetes runtime
// instance
func (c *KubernetesRuntimeInstanceConfig) deleteMetricsDefinition(metricsDefinitionID *uint) error {

	// delete metrics definition
	_, err := client.DeleteMetricsDefinition(
		c.r.APIClient,
		c.r.APIServer,
		*metricsDefinitionID,
	)
	if err != nil && !errors.Is(err, client.ErrObjectNotFound) {
		return fmt.Errorf("failed to delete metrics definition: %w", err)
	}
	c.kubernetesRuntimeInstance.MetricsInstanceID = util.SqlNullInt64(nil)

	// wait for metrics definition to be deleted
	if err = util.Retry(120, 1, func() error {
		_, err = client.GetMetricsDefinitionByID(
			c.r.APIClient,
			c.r.APIServer,
			*metricsDefinitionID,
		)
		if err != nil {
			if errors.Is(err, client.ErrObjectNotFound) {
				return nil
			}
			return fmt.Errorf("failed to get metrics definition by ID: %w", err)
		}
		return fmt.Errorf("metrics definition still exists")
	}); err != nil {
		return fmt.Errorf("failed to wait for metrics definition to be deleted: %w", err)
	}

	return nil
}

// getMetricsOperations returns a set of operations for configuring metrics
func getMetricsOperations(c *KubernetesRuntimeInstanceConfig, metricsDefinitionID *uint) *util.Operations {

	operations := util.Operations{}

	var createdMetricsDefinitionID *uint
	var err error

	// append metrics definition operations
	operations.AppendOperation(util.Operation{
		Name: "metrics definition",
		Create: func() error {
			createdMetricsDefinitionID, err = c.createMetricsDefinition()
			if err != nil {
				return fmt.Errorf("failed to create grafana helm workload instance: %w", err)
			}
			return nil
		},
		Delete: func() error {
			if err = c.deleteMetricsDefinition(metricsDefinitionID); err != nil {
				return fmt.Errorf("failed to delete grafana helm workload instance: %w", err)
			}
			return nil
		},
	})

	// append metrics instance operations
	operations.AppendOperation(util.Operation{
		Name: "metrics instance",
		Create: func() error {
			if err := c.createMetricsInstance(createdMetricsDefinitionID); err != nil {
				return fmt.Errorf("failed to create loki helm workload instance: %w", err)
			}
			return nil
		},
		Delete: func() error {
			if err := c.deleteMetricsInstance(); err != nil {
				return fmt.Errorf("failed to delete loki helm workload instance: %w", err)
			}
			return nil
		},
	})

	return &operations
}

// getMetricsOperations returns a set of operations for configuring metrics
func getLoggingOperations(c *KubernetesRuntimeInstanceConfig, loggingDefinitionID *uint) *util.Operations {

	operations := util.Operations{}

	var loggingMetricsDefinitionID *uint
	var err error

	// append metrics definition operations
	operations.AppendOperation(util.Operation{
		Name: "logging definition",
		Create: func() error {
			loggingMetricsDefinitionID, err = c.createLoggingDefinition()
			if err != nil {
				return fmt.Errorf("failed to create grafana helm workload instance: %w", err)
			}
			return nil
		},
		Delete: func() error {
			if err = c.deleteLoggingDefinition(loggingDefinitionID); err != nil {
				return fmt.Errorf("failed to delete grafana helm workload instance: %w", err)
			}
			return nil
		},
	})

	// append metrics instance operations
	operations.AppendOperation(util.Operation{
		Name: "logging instance",
		Create: func() error {
			if err := c.createLoggingInstance(loggingMetricsDefinitionID); err != nil {
				return fmt.Errorf("failed to create loki helm workload instance: %w", err)
			}
			return nil
		},
		Delete: func() error {
			if err := c.deleteLoggingInstance(); err != nil {
				return fmt.Errorf("failed to delete loki helm workload instance: %w", err)
			}
			return nil
		},
	})

	return &operations
}
