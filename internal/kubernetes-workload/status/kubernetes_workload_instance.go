package status

import (
	"fmt"
	"net/http"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/yaml"

	"github.com/threeport/threeport/internal/agent"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
)

// WorkloadInstanceStatus is a standardized status for a kubernetes workload instance.
type WorkloadInstanceStatus string

const (
	// WorkloadInstanceStatusReconciling indicates a kubernetes workload instance is in the
	// process of being reconciled - either currently being created or updated
	WorkloadInstanceStatusReconciling WorkloadInstanceStatus = "Reconciling"

	// WorkloadInstanceStatusHealthy indicates a kubernetes workload instance is in an
	// expected, healthy state
	WorkloadInstanceStatusHealthy WorkloadInstanceStatus = "Healthy"

	// WorkloadInstanceStatusUnhealthy indicates there is something wrong with a
	// kubernetes workload instance and should be inspected
	WorkloadInstanceStatusUnhealthy WorkloadInstanceStatus = "Unhealthy"

	// WorkloadInstanceStatusDown indicates a kubernetes workload instance is not running
	// and has a critical problem that should be remedied
	WorkloadInstanceStatusDown WorkloadInstanceStatus = "Down"

	// WorkloadInstanceStatusError indicates there was a system error that
	// prevented retrieving kubernetes workload instance status
	WorkloadInstanceStatusError WorkloadInstanceStatus = "Error"
)

// WorkloadInstanceStatusDetail contains all the data for kubernetes workload instance
// status info.
type WorkloadInstanceStatusDetail struct {
	Status WorkloadInstanceStatus
	Reason string
	Error  error
	Events []v0.Event
}

// GetKubernetesWorkloadInstanceStatus inspects a kubernetes workload instance and returns
// the status details for it.
func GetKubernetesWorkloadInstanceStatus(
	apiClient *http.Client,
	apiEndpoint string,
	workloadInstanceType string,
	workloadInstanceId uint,
	workloadInstanceReconciled bool,
) *WorkloadInstanceStatusDetail {
	var workloadInstanceStatusDetail WorkloadInstanceStatusDetail

	// retrieve events for the kubernetes workload instance via the AOR join,
	// filtering on the subject (object type + id). Events are stored in
	// the Event table; the per-instance linkage lives on the
	// AttachedObjectReference.
	var subjectType string
	switch workloadInstanceType {
	case agent.KubernetesWorkloadInstanceType:
		subjectType = "KubernetesWorkloadInstance"
	case agent.HelmWorkloadInstanceType:
		subjectType = "HelmWorkloadInstance"
	default:
		workloadInstanceStatusDetail.Status = WorkloadInstanceStatusError
		workloadInstanceStatusDetail.Error = fmt.Errorf(
			"%s is an unrecognized workload type - recoginzed types: [%s,%s]",
			workloadInstanceType,
			agent.KubernetesWorkloadInstanceType,
			agent.HelmWorkloadInstanceType,
		)
		return &workloadInstanceStatusDetail
	}

	workloadEvents, err := client.GetEventsJoinAttachedObjectReferenceByQueryString(
		apiClient,
		apiEndpoint,
		fmt.Sprintf(
			"objectid=%d&objecttypename=%s&objectnamespace=threeport.io&objectversion=v0",
			workloadInstanceId, subjectType,
		),
	)
	if err != nil {
		workloadInstanceStatusDetail.Status = WorkloadInstanceStatusError
		workloadInstanceStatusDetail.Error = fmt.Errorf("failed to get events from API: %w", err)
		return &workloadInstanceStatusDetail
	}

	// return status "Reconciling" until we begin to get events that indicate
	// the Kubernetes resources are being created
	if len(*workloadEvents) == 0 {
		workloadInstanceStatusDetail.Status = WorkloadInstanceStatusReconciling
		return &workloadInstanceStatusDetail
	}

	// collect any events of type Warning (Event type only emits Normal
	// and Warning; the legacy "Failed" event type no longer
	// exists).
	var alertEvents []v0.Event
	for _, evt := range *workloadEvents {
		if *evt.Type == "Warning" {
			// capture event if we haven't already
			eventCaptured := false
			for _, ae := range alertEvents {
				if *ae.Note == *evt.Note {
					eventCaptured = true
					break
				}
			}
			if !eventCaptured {
				alertEvents = append(alertEvents, evt)
			}
		}
	}
	workloadInstanceStatusDetail.Events = alertEvents

	// check kubernetes workload instance is reconciled
	//if !*workloadInstance.Reconciled {
	if !workloadInstanceReconciled {
		workloadInstanceStatusDetail.Status = WorkloadInstanceStatusReconciling
		return &workloadInstanceStatusDetail
	}

	// find Deployment or StatefulSet resources and check they are healthy
	workloadResourceInstances, err := client.GetKubernetesWorkloadResourceInstancesByID(
		apiClient,
		apiEndpoint,
		workloadInstanceId,
	)
	if err != nil {
		workloadInstanceStatusDetail.Status = WorkloadInstanceStatusError
		workloadInstanceStatusDetail.Error = fmt.Errorf("failed to get workload resource instances from API: %w", err)
		return &workloadInstanceStatusDetail
	}
	for _, wri := range *workloadResourceInstances {
		if wri.RuntimeDefinition != nil {
			var runtimeDefinition unstructured.Unstructured
			if err := yaml.Unmarshal([]byte(*wri.RuntimeDefinition), &runtimeDefinition); err != nil {
				workloadInstanceStatusDetail.Status = WorkloadInstanceStatusError
				workloadInstanceStatusDetail.Error = fmt.Errorf("failed to get workload resource instances from API: %w", err)
				return &workloadInstanceStatusDetail
			}
			switch runtimeDefinition.GetKind() {
			case "Deployment":
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
			case "StatefulSet":
				status, reason, err := inspectStatefulSet(&runtimeDefinition)
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

// inspectStatefulSet inspects a StatefulSet resource for status.
func inspectStatefulSet(runtimeDefinition *unstructured.Unstructured) (WorkloadInstanceStatus, string, error) {
	var statefulSet appsv1.StatefulSet
	if err := scheme.Scheme.Convert(runtimeDefinition, &statefulSet, nil); err != nil {
		return WorkloadInstanceStatusError, "", fmt.Errorf("failed to convert runtime definition into typed StatefulSet object: %w", err)
	}

	// check statefulset replicas
	desiredReplicas := statefulSet.Spec.Replicas
	readyReplicas := statefulSet.Status.ReadyReplicas
	if readyReplicas == int32(0) {
		reason := fmt.Sprintf(
			"StatefulSet %s/%s has 0 replicas ready",
			statefulSet.ObjectMeta.Namespace, statefulSet.ObjectMeta.Name,
		)
		return WorkloadInstanceStatusDown, reason, nil
	}
	if readyReplicas < *desiredReplicas {
		reason := fmt.Sprintf(
			"StatefulSet %s/%s is configured to have %d replicas but has %d ready",
			statefulSet.ObjectMeta.Namespace, statefulSet.ObjectMeta.Name,
			desiredReplicas, readyReplicas,
		)
		return WorkloadInstanceStatusUnhealthy, reason, nil
	}

	return WorkloadInstanceStatusHealthy, "", nil
}
