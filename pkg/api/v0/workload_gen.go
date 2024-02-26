// generated by 'threeport-sdk codegen api-model' - do not edit

package v0

import (
	"encoding/json"
	"fmt"
	notifications "github.com/threeport/threeport/pkg/notifications/v0"
)

const (
	ObjectTypeWorkloadDefinition         ObjectType = "WorkloadDefinition"
	ObjectTypeWorkloadResourceDefinition ObjectType = "WorkloadResourceDefinition"
	ObjectTypeWorkloadInstance           ObjectType = "WorkloadInstance"
	ObjectTypeAttachedObjectReference    ObjectType = "AttachedObjectReference"
	ObjectTypeWorkloadResourceInstance   ObjectType = "WorkloadResourceInstance"
	ObjectTypeWorkloadEvent              ObjectType = "WorkloadEvent"

	WorkloadStreamName = "workloadStream"

	WorkloadDefinitionSubject       = "workloadDefinition.*"
	WorkloadDefinitionCreateSubject = "workloadDefinition.create"
	WorkloadDefinitionUpdateSubject = "workloadDefinition.update"
	WorkloadDefinitionDeleteSubject = "workloadDefinition.delete"

	WorkloadResourceDefinitionSubject       = "workloadResourceDefinition.*"
	WorkloadResourceDefinitionCreateSubject = "workloadResourceDefinition.create"
	WorkloadResourceDefinitionUpdateSubject = "workloadResourceDefinition.update"
	WorkloadResourceDefinitionDeleteSubject = "workloadResourceDefinition.delete"

	WorkloadInstanceSubject       = "workloadInstance.*"
	WorkloadInstanceCreateSubject = "workloadInstance.create"
	WorkloadInstanceUpdateSubject = "workloadInstance.update"
	WorkloadInstanceDeleteSubject = "workloadInstance.delete"

	AttachedObjectReferenceSubject       = "attachedObjectReference.*"
	AttachedObjectReferenceCreateSubject = "attachedObjectReference.create"
	AttachedObjectReferenceUpdateSubject = "attachedObjectReference.update"
	AttachedObjectReferenceDeleteSubject = "attachedObjectReference.delete"

	WorkloadResourceInstanceSubject       = "workloadResourceInstance.*"
	WorkloadResourceInstanceCreateSubject = "workloadResourceInstance.create"
	WorkloadResourceInstanceUpdateSubject = "workloadResourceInstance.update"
	WorkloadResourceInstanceDeleteSubject = "workloadResourceInstance.delete"

	WorkloadEventSubject       = "workloadEvent.*"
	WorkloadEventCreateSubject = "workloadEvent.create"
	WorkloadEventUpdateSubject = "workloadEvent.update"
	WorkloadEventDeleteSubject = "workloadEvent.delete"

	PathWorkloadDefinitions         = "/v0/workload-definitions"
	PathWorkloadResourceDefinitions = "/v0/workload-resource-definitions"
	PathWorkloadInstances           = "/v0/workload-instances"
	PathAttachedObjectReferences    = "/v0/attached-object-references"
	PathWorkloadResourceInstances   = "/v0/workload-resource-instances"
	PathWorkloadEvents              = "/v0/workload-events"
)

// GetWorkloadDefinitionSubjects returns the NATS subjects
// for workload definitions.
func GetWorkloadDefinitionSubjects() []string {
	return []string{
		WorkloadDefinitionCreateSubject,
		WorkloadDefinitionUpdateSubject,
		WorkloadDefinitionDeleteSubject,
	}
}

// GetWorkloadResourceDefinitionSubjects returns the NATS subjects
// for workload resource definitions.
func GetWorkloadResourceDefinitionSubjects() []string {
	return []string{
		WorkloadResourceDefinitionCreateSubject,
		WorkloadResourceDefinitionUpdateSubject,
		WorkloadResourceDefinitionDeleteSubject,
	}
}

// GetWorkloadInstanceSubjects returns the NATS subjects
// for workload instances.
func GetWorkloadInstanceSubjects() []string {
	return []string{
		WorkloadInstanceCreateSubject,
		WorkloadInstanceUpdateSubject,
		WorkloadInstanceDeleteSubject,
	}
}

// GetAttachedObjectReferenceSubjects returns the NATS subjects
// for attached object references.
func GetAttachedObjectReferenceSubjects() []string {
	return []string{
		AttachedObjectReferenceCreateSubject,
		AttachedObjectReferenceUpdateSubject,
		AttachedObjectReferenceDeleteSubject,
	}
}

// GetWorkloadResourceInstanceSubjects returns the NATS subjects
// for workload resource instances.
func GetWorkloadResourceInstanceSubjects() []string {
	return []string{
		WorkloadResourceInstanceCreateSubject,
		WorkloadResourceInstanceUpdateSubject,
		WorkloadResourceInstanceDeleteSubject,
	}
}

// GetWorkloadEventSubjects returns the NATS subjects
// for workload events.
func GetWorkloadEventSubjects() []string {
	return []string{
		WorkloadEventCreateSubject,
		WorkloadEventUpdateSubject,
		WorkloadEventDeleteSubject,
	}
}

// GetWorkloadSubjects returns the NATS subjects
// for all workload objects.
func GetWorkloadSubjects() []string {
	var workloadSubjects []string

	workloadSubjects = append(workloadSubjects, GetWorkloadDefinitionSubjects()...)
	workloadSubjects = append(workloadSubjects, GetWorkloadResourceDefinitionSubjects()...)
	workloadSubjects = append(workloadSubjects, GetWorkloadInstanceSubjects()...)
	workloadSubjects = append(workloadSubjects, GetAttachedObjectReferenceSubjects()...)
	workloadSubjects = append(workloadSubjects, GetWorkloadResourceInstanceSubjects()...)
	workloadSubjects = append(workloadSubjects, GetWorkloadEventSubjects()...)

	return workloadSubjects
}

// NotificationPayload returns the notification payload that is delivered to the
// controller when a change is made.  It includes the object as presented by the
// client when the change was made.
func (wd *WorkloadDefinition) NotificationPayload(
	operation notifications.NotificationOperation,
	requeue bool,
	creationTime int64,
) (*[]byte, error) {
	notif := notifications.Notification{
		CreationTime: &creationTime,
		Object:       wd,
		Operation:    operation,
	}

	payload, err := json.Marshal(notif)
	if err != nil {
		return &payload, fmt.Errorf("failed to marshal notification payload %+v: %w", wd, err)
	}

	return &payload, nil
}

// DecodeNotifObject takes the threeport object in the form of a
// map[string]interface and returns the typed object by marshalling into JSON
// and then unmarshalling into the typed object.  We are not using the
// mapstructure library here as that requires custom decode hooks to manage
// fields with non-native go types.
func (wd *WorkloadDefinition) DecodeNotifObject(object interface{}) error {
	jsonObject, err := json.Marshal(object)
	if err != nil {
		return fmt.Errorf("failed to marshal object map from consumed notification message: %w", err)
	}
	if err := json.Unmarshal(jsonObject, &wd); err != nil {
		return fmt.Errorf("failed to unmarshal json object to typed object: %w", err)
	}
	return nil
}

// GetID returns the unique ID for the object.
func (wd *WorkloadDefinition) GetID() uint {
	return *wd.ID
}

// String returns a string representation of the ojbect.
func (wd WorkloadDefinition) String() string {
	return fmt.Sprintf("v0.WorkloadDefinition")
}

// NotificationPayload returns the notification payload that is delivered to the
// controller when a change is made.  It includes the object as presented by the
// client when the change was made.
func (wrd *WorkloadResourceDefinition) NotificationPayload(
	operation notifications.NotificationOperation,
	requeue bool,
	creationTime int64,
) (*[]byte, error) {
	notif := notifications.Notification{
		CreationTime: &creationTime,
		Object:       wrd,
		Operation:    operation,
	}

	payload, err := json.Marshal(notif)
	if err != nil {
		return &payload, fmt.Errorf("failed to marshal notification payload %+v: %w", wrd, err)
	}

	return &payload, nil
}

// DecodeNotifObject takes the threeport object in the form of a
// map[string]interface and returns the typed object by marshalling into JSON
// and then unmarshalling into the typed object.  We are not using the
// mapstructure library here as that requires custom decode hooks to manage
// fields with non-native go types.
func (wrd *WorkloadResourceDefinition) DecodeNotifObject(object interface{}) error {
	jsonObject, err := json.Marshal(object)
	if err != nil {
		return fmt.Errorf("failed to marshal object map from consumed notification message: %w", err)
	}
	if err := json.Unmarshal(jsonObject, &wrd); err != nil {
		return fmt.Errorf("failed to unmarshal json object to typed object: %w", err)
	}
	return nil
}

// GetID returns the unique ID for the object.
func (wrd *WorkloadResourceDefinition) GetID() uint {
	return *wrd.ID
}

// String returns a string representation of the ojbect.
func (wrd WorkloadResourceDefinition) String() string {
	return fmt.Sprintf("v0.WorkloadResourceDefinition")
}

// NotificationPayload returns the notification payload that is delivered to the
// controller when a change is made.  It includes the object as presented by the
// client when the change was made.
func (wi *WorkloadInstance) NotificationPayload(
	operation notifications.NotificationOperation,
	requeue bool,
	creationTime int64,
) (*[]byte, error) {
	notif := notifications.Notification{
		CreationTime: &creationTime,
		Object:       wi,
		Operation:    operation,
	}

	payload, err := json.Marshal(notif)
	if err != nil {
		return &payload, fmt.Errorf("failed to marshal notification payload %+v: %w", wi, err)
	}

	return &payload, nil
}

// DecodeNotifObject takes the threeport object in the form of a
// map[string]interface and returns the typed object by marshalling into JSON
// and then unmarshalling into the typed object.  We are not using the
// mapstructure library here as that requires custom decode hooks to manage
// fields with non-native go types.
func (wi *WorkloadInstance) DecodeNotifObject(object interface{}) error {
	jsonObject, err := json.Marshal(object)
	if err != nil {
		return fmt.Errorf("failed to marshal object map from consumed notification message: %w", err)
	}
	if err := json.Unmarshal(jsonObject, &wi); err != nil {
		return fmt.Errorf("failed to unmarshal json object to typed object: %w", err)
	}
	return nil
}

// GetID returns the unique ID for the object.
func (wi *WorkloadInstance) GetID() uint {
	return *wi.ID
}

// String returns a string representation of the ojbect.
func (wi WorkloadInstance) String() string {
	return fmt.Sprintf("v0.WorkloadInstance")
}

// NotificationPayload returns the notification payload that is delivered to the
// controller when a change is made.  It includes the object as presented by the
// client when the change was made.
func (aor *AttachedObjectReference) NotificationPayload(
	operation notifications.NotificationOperation,
	requeue bool,
	creationTime int64,
) (*[]byte, error) {
	notif := notifications.Notification{
		CreationTime: &creationTime,
		Object:       aor,
		Operation:    operation,
	}

	payload, err := json.Marshal(notif)
	if err != nil {
		return &payload, fmt.Errorf("failed to marshal notification payload %+v: %w", aor, err)
	}

	return &payload, nil
}

// DecodeNotifObject takes the threeport object in the form of a
// map[string]interface and returns the typed object by marshalling into JSON
// and then unmarshalling into the typed object.  We are not using the
// mapstructure library here as that requires custom decode hooks to manage
// fields with non-native go types.
func (aor *AttachedObjectReference) DecodeNotifObject(object interface{}) error {
	jsonObject, err := json.Marshal(object)
	if err != nil {
		return fmt.Errorf("failed to marshal object map from consumed notification message: %w", err)
	}
	if err := json.Unmarshal(jsonObject, &aor); err != nil {
		return fmt.Errorf("failed to unmarshal json object to typed object: %w", err)
	}
	return nil
}

// GetID returns the unique ID for the object.
func (aor *AttachedObjectReference) GetID() uint {
	return *aor.ID
}

// String returns a string representation of the ojbect.
func (aor AttachedObjectReference) String() string {
	return fmt.Sprintf("v0.AttachedObjectReference")
}

// NotificationPayload returns the notification payload that is delivered to the
// controller when a change is made.  It includes the object as presented by the
// client when the change was made.
func (wri *WorkloadResourceInstance) NotificationPayload(
	operation notifications.NotificationOperation,
	requeue bool,
	creationTime int64,
) (*[]byte, error) {
	notif := notifications.Notification{
		CreationTime: &creationTime,
		Object:       wri,
		Operation:    operation,
	}

	payload, err := json.Marshal(notif)
	if err != nil {
		return &payload, fmt.Errorf("failed to marshal notification payload %+v: %w", wri, err)
	}

	return &payload, nil
}

// DecodeNotifObject takes the threeport object in the form of a
// map[string]interface and returns the typed object by marshalling into JSON
// and then unmarshalling into the typed object.  We are not using the
// mapstructure library here as that requires custom decode hooks to manage
// fields with non-native go types.
func (wri *WorkloadResourceInstance) DecodeNotifObject(object interface{}) error {
	jsonObject, err := json.Marshal(object)
	if err != nil {
		return fmt.Errorf("failed to marshal object map from consumed notification message: %w", err)
	}
	if err := json.Unmarshal(jsonObject, &wri); err != nil {
		return fmt.Errorf("failed to unmarshal json object to typed object: %w", err)
	}
	return nil
}

// GetID returns the unique ID for the object.
func (wri *WorkloadResourceInstance) GetID() uint {
	return *wri.ID
}

// String returns a string representation of the ojbect.
func (wri WorkloadResourceInstance) String() string {
	return fmt.Sprintf("v0.WorkloadResourceInstance")
}

// NotificationPayload returns the notification payload that is delivered to the
// controller when a change is made.  It includes the object as presented by the
// client when the change was made.
func (we *WorkloadEvent) NotificationPayload(
	operation notifications.NotificationOperation,
	requeue bool,
	creationTime int64,
) (*[]byte, error) {
	notif := notifications.Notification{
		CreationTime: &creationTime,
		Object:       we,
		Operation:    operation,
	}

	payload, err := json.Marshal(notif)
	if err != nil {
		return &payload, fmt.Errorf("failed to marshal notification payload %+v: %w", we, err)
	}

	return &payload, nil
}

// DecodeNotifObject takes the threeport object in the form of a
// map[string]interface and returns the typed object by marshalling into JSON
// and then unmarshalling into the typed object.  We are not using the
// mapstructure library here as that requires custom decode hooks to manage
// fields with non-native go types.
func (we *WorkloadEvent) DecodeNotifObject(object interface{}) error {
	jsonObject, err := json.Marshal(object)
	if err != nil {
		return fmt.Errorf("failed to marshal object map from consumed notification message: %w", err)
	}
	if err := json.Unmarshal(jsonObject, &we); err != nil {
		return fmt.Errorf("failed to unmarshal json object to typed object: %w", err)
	}
	return nil
}

// GetID returns the unique ID for the object.
func (we *WorkloadEvent) GetID() uint {
	return *we.ID
}

// String returns a string representation of the ojbect.
func (we WorkloadEvent) String() string {
	return fmt.Sprintf("v0.WorkloadEvent")
}
