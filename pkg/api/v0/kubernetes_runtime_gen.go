// generated by 'threeport-codegen api-model' - do not edit

package v0

import (
	"encoding/json"
	"fmt"
	notifications "github.com/threeport/threeport/pkg/notifications/v0"
)

const (
	ObjectTypeKubernetesRuntimeDefinition ObjectType = "KubernetesRuntimeDefinition"
	ObjectTypeKubernetesRuntimeInstance   ObjectType = "KubernetesRuntimeInstance"

	KubernetesRuntimeStreamName = "kubernetesRuntimeStream"

	KubernetesRuntimeDefinitionSubject       = "kubernetesRuntimeDefinition.*"
	KubernetesRuntimeDefinitionCreateSubject = "kubernetesRuntimeDefinition.create"
	KubernetesRuntimeDefinitionUpdateSubject = "kubernetesRuntimeDefinition.update"
	KubernetesRuntimeDefinitionDeleteSubject = "kubernetesRuntimeDefinition.delete"

	KubernetesRuntimeInstanceSubject       = "kubernetesRuntimeInstance.*"
	KubernetesRuntimeInstanceCreateSubject = "kubernetesRuntimeInstance.create"
	KubernetesRuntimeInstanceUpdateSubject = "kubernetesRuntimeInstance.update"
	KubernetesRuntimeInstanceDeleteSubject = "kubernetesRuntimeInstance.delete"

	PathKubernetesRuntimeDefinitions = "/v0/kubernetes-runtime-definitions"
	PathKubernetesRuntimeInstances   = "/v0/kubernetes-runtime-instances"
)

// GetKubernetesRuntimeDefinitionSubjects returns the NATS subjects
// for kubernetes runtime definitions.
func GetKubernetesRuntimeDefinitionSubjects() []string {
	return []string{
		KubernetesRuntimeDefinitionCreateSubject,
		KubernetesRuntimeDefinitionUpdateSubject,
		KubernetesRuntimeDefinitionDeleteSubject,
	}
}

// GetKubernetesRuntimeInstanceSubjects returns the NATS subjects
// for kubernetes runtime instances.
func GetKubernetesRuntimeInstanceSubjects() []string {
	return []string{
		KubernetesRuntimeInstanceCreateSubject,
		KubernetesRuntimeInstanceUpdateSubject,
		KubernetesRuntimeInstanceDeleteSubject,
	}
}

// GetKubernetesRuntimeSubjects returns the NATS subjects
// for all kubernetes runtime objects.
func GetKubernetesRuntimeSubjects() []string {
	var kubernetesRuntimeSubjects []string

	kubernetesRuntimeSubjects = append(kubernetesRuntimeSubjects, GetKubernetesRuntimeDefinitionSubjects()...)
	kubernetesRuntimeSubjects = append(kubernetesRuntimeSubjects, GetKubernetesRuntimeInstanceSubjects()...)

	return kubernetesRuntimeSubjects
}

// NotificationPayload returns the notification payload that is delivered to the
// controller when a change is made.  It includes the object as presented by the
// client when the change was made.
func (krd *KubernetesRuntimeDefinition) NotificationPayload(
	operation notifications.NotificationOperation,
	requeue bool,
	lastDelay int64,
) (*[]byte, error) {
	notif := notifications.Notification{
		LastRequeueDelay: &lastDelay,
		Object:           krd,
		Operation:        operation,
		Requeue:          requeue,
	}

	payload, err := json.Marshal(notif)
	if err != nil {
		return &payload, fmt.Errorf("failed to marshal notification payload %+v: %w", krd, err)
	}

	return &payload, nil
}

// GetID returns the unique ID for the object.
func (krd *KubernetesRuntimeDefinition) GetID() uint {
	return *krd.ID
}

// String returns a string representation of the ojbect.
func (krd KubernetesRuntimeDefinition) String() string {
	return fmt.Sprintf("v0.KubernetesRuntimeDefinition")
}

// NotificationPayload returns the notification payload that is delivered to the
// controller when a change is made.  It includes the object as presented by the
// client when the change was made.
func (kri *KubernetesRuntimeInstance) NotificationPayload(
	operation notifications.NotificationOperation,
	requeue bool,
	lastDelay int64,
) (*[]byte, error) {
	notif := notifications.Notification{
		LastRequeueDelay: &lastDelay,
		Object:           kri,
		Operation:        operation,
		Requeue:          requeue,
	}

	payload, err := json.Marshal(notif)
	if err != nil {
		return &payload, fmt.Errorf("failed to marshal notification payload %+v: %w", kri, err)
	}

	return &payload, nil
}

// GetID returns the unique ID for the object.
func (kri *KubernetesRuntimeInstance) GetID() uint {
	return *kri.ID
}

// String returns a string representation of the ojbect.
func (kri KubernetesRuntimeInstance) String() string {
	return fmt.Sprintf("v0.KubernetesRuntimeInstance")
}
