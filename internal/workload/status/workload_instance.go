package status

import (
	"fmt"
	"net/http"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/yaml"

	"github.com/threeport/threeport/internal/util"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
)

// WorkloadInstanceStatus is a standardized status for a workload instance.
type WorkloadInstanceStatus string

const (
	// WorkloadInstanceStatusReconciling indicates a workload instance is in the
	// process of being reconciled - either currently being created or updated
	WorkloadInstanceStatusReconciling WorkloadInstanceStatus = "Reconciling"

	// WorkloadInstanceStatusHealthy indicates a workload instance is in an
	// expected, healthy state
	WorkloadInstanceStatusHealthy WorkloadInstanceStatus = "Healthy"

	// WorkloadInstanceStatusUnhealthy indicates there is something wrong with a
	// workload instance and should be inspected
	WorkloadInstanceStatusUnhealthy WorkloadInstanceStatus = "Unhealthy"

	// WorkloadInstanceStatusDown indicates a workload instance is not running
	// and has a critical problem that should be remedied
	WorkloadInstanceStatusDown WorkloadInstanceStatus = "Down"

	// WorkloadInstanceStatusError indicates there was a system error that
	// prevented retrieving workload instance status
	WorkloadInstanceStatusError WorkloadInstanceStatus = "Error"
)

// WorkloadInstanceStatusDetail contains all the data for workload instance
// status info.
type WorkloadInstanceStatusDetail struct {
	Status WorkloadInstanceStatus
	Reason string
	Error  error
	Events []string
}

// GetWorkloadInstanceStatus inspects a workload instance and returns the status
// detials for it.
func GetWorkloadInstanceStatus(
	apiClient *http.Client,
	apiEndpoint string,
	workloadInstance *v0.WorkloadInstance,
) *WorkloadInstanceStatusDetail {
	var workloadInstanceStatusDetail WorkloadInstanceStatusDetail

	// check workload instance is reconciled
	if !*workloadInstance.Reconciled {
		workloadInstanceStatusDetail.Status = WorkloadInstanceStatusReconciling
		return &workloadInstanceStatusDetail
	}

	// collect any failed events
	workloadEvents, err := client.GetWorkloadEventsByWorkloadInstanceID(
		apiClient,
		apiEndpoint,
		*workloadInstance.ID,
	)
	if err != nil {
		workloadInstanceStatusDetail.Status = WorkloadInstanceStatusError
		workloadInstanceStatusDetail.Error = fmt.Errorf("failed to get workload events from API: %w", err)
		return &workloadInstanceStatusDetail
	}
	var failedEvents []string
	for _, event := range *workloadEvents {
		if *event.Reason == "Failed" {
			failedEvents = util.AddStringToSliceIfNotExists(failedEvents, *event.Message)
		}
	}
	workloadInstanceStatusDetail.Events = failedEvents

	// find Deployment or StatefulSet resources and check they are healthy
	workloadResourceInstances, err := client.GetWorkloadResourceInstancesByWorkloadInstanceID(
		apiClient,
		apiEndpoint,
		*workloadInstance.ID,
	)
	if err != nil {
		workloadInstanceStatusDetail.Status = WorkloadInstanceStatusError
		workloadInstanceStatusDetail.Error = fmt.Errorf("failed to get workload resource instances from API: %w", err)
		return &workloadInstanceStatusDetail
	}
	for _, wri := range *workloadResourceInstances {
		var runtimeDefinition unstructured.Unstructured
		if err := yaml.Unmarshal([]byte(*wri.RuntimeDefinition), &runtimeDefinition); err != nil {
			workloadInstanceStatusDetail.Status = WorkloadInstanceStatusError
			workloadInstanceStatusDetail.Error = fmt.Errorf("failed to get workload resource instances from API: %w", err)
			return &workloadInstanceStatusDetail
		}
		if runtimeDefinition.GetKind() == "Deployment" {
			status, reason, err := inspectDeployment(&runtimeDefinition)
			if err != nil {
				workloadInstanceStatusDetail.Status = status
				workloadInstanceStatusDetail.Error = err
				return &workloadInstanceStatusDetail
			}
			if status != WorkloadInstanceStatusHealthy {
				workloadInstanceStatusDetail.Status = status
				workloadInstanceStatusDetail.Reason = reason
				return &workloadInstanceStatusDetail
			}
		}
	}

	workloadInstanceStatusDetail.Status = WorkloadInstanceStatusHealthy
	return &workloadInstanceStatusDetail
}

// inspectDeployment inspects a Deployment resource for status.
func inspectDeployment(runtimeDefinition *unstructured.Unstructured) (WorkloadInstanceStatus, string, error) {
	var deployment appsv1.Deployment
	if err := scheme.Scheme.Convert(runtimeDefinition, &deployment, nil); err != nil {
		return WorkloadInstanceStatusError, "", fmt.Errorf("failed to convert runtime definition into typed Deployment object: %w", err)
	}

	// check deployment replicas
	desiredReplicas := deployment.Spec.Replicas
	readyReplicas := deployment.Status.ReadyReplicas
	if readyReplicas == int32(0) {
		reason := fmt.Sprintf(
			"Deployment %s/%s has 0 replicas ready",
			deployment.ObjectMeta.Namespace, deployment.ObjectMeta.Name,
		)
		return WorkloadInstanceStatusDown, reason, nil
	}
	if readyReplicas < *desiredReplicas {
		reason := fmt.Sprintf(
			"Deployment %s/%s is configured to have %d replicas but has %d ready",
			deployment.ObjectMeta.Namespace, deployment.ObjectMeta.Name,
			desiredReplicas, readyReplicas,
		)
		return WorkloadInstanceStatusUnhealthy, reason, nil
	}

	return WorkloadInstanceStatusHealthy, "", nil
}
