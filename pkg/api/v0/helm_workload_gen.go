// generated by 'threeport-sdk codegen api-model' - do not edit

package v0

import (
	"encoding/json"
	"fmt"
	notifications "github.com/threeport/threeport/pkg/notifications/v0"
)

const (
	ObjectTypeHelmWorkloadDefinition ObjectType = "HelmWorkloadDefinition"
	ObjectTypeHelmWorkloadInstance   ObjectType = "HelmWorkloadInstance"

	HelmWorkloadStreamName = "helmWorkloadStream"

	HelmWorkloadDefinitionSubject       = "helmWorkloadDefinition.*"
	HelmWorkloadDefinitionCreateSubject = "helmWorkloadDefinition.create"
	HelmWorkloadDefinitionUpdateSubject = "helmWorkloadDefinition.update"
	HelmWorkloadDefinitionDeleteSubject = "helmWorkloadDefinition.delete"

	HelmWorkloadInstanceSubject       = "helmWorkloadInstance.*"
	HelmWorkloadInstanceCreateSubject = "helmWorkloadInstance.create"
	HelmWorkloadInstanceUpdateSubject = "helmWorkloadInstance.update"
	HelmWorkloadInstanceDeleteSubject = "helmWorkloadInstance.delete"

	PathHelmWorkloadDefinitions = "/v0/helm-workload-definitions"
	PathHelmWorkloadInstances   = "/v0/helm-workload-instances"
)

// GetHelmWorkloadDefinitionSubjects returns the NATS subjects
// for helm workload definitions.
func GetHelmWorkloadDefinitionSubjects() []string {
	return []string{
		HelmWorkloadDefinitionCreateSubject,
		HelmWorkloadDefinitionUpdateSubject,
		HelmWorkloadDefinitionDeleteSubject,
	}
}

// GetHelmWorkloadInstanceSubjects returns the NATS subjects
// for helm workload instances.
func GetHelmWorkloadInstanceSubjects() []string {
	return []string{
		HelmWorkloadInstanceCreateSubject,
		HelmWorkloadInstanceUpdateSubject,
		HelmWorkloadInstanceDeleteSubject,
	}
}

// GetHelmWorkloadSubjects returns the NATS subjects
// for all helm workload objects.
func GetHelmWorkloadSubjects() []string {
	var helmWorkloadSubjects []string

	helmWorkloadSubjects = append(helmWorkloadSubjects, GetHelmWorkloadDefinitionSubjects()...)
	helmWorkloadSubjects = append(helmWorkloadSubjects, GetHelmWorkloadInstanceSubjects()...)

	return helmWorkloadSubjects
}

// NotificationPayload returns the notification payload that is delivered to the
// controller when a change is made.  It includes the object as presented by the
// client when the change was made.
func (hwd *HelmWorkloadDefinition) NotificationPayload(
	operation notifications.NotificationOperation,
	requeue bool,
	creationTime int64,
) (*[]byte, error) {
	notif := notifications.Notification{
		CreationTime: &creationTime,
		Object:       hwd,
		Operation:    operation,
	}

	payload, err := json.Marshal(notif)
	if err != nil {
		return &payload, fmt.Errorf("failed to marshal notification payload %+v: %w", hwd, err)
	}

	return &payload, nil
}

// DecodeNotifObject takes the threeport object in the form of a
// map[string]interface and returns the typed object by marshalling into JSON
// and then unmarshalling into the typed object.  We are not using the
// mapstructure library here as that requires custom decode hooks to manage
// fields with non-native go types.
func (hwd *HelmWorkloadDefinition) DecodeNotifObject(object interface{}) error {
	jsonObject, err := json.Marshal(object)
	if err != nil {
		return fmt.Errorf("failed to marshal object map from consumed notification message: %w", err)
	}
	if err := json.Unmarshal(jsonObject, &hwd); err != nil {
		return fmt.Errorf("failed to unmarshal json object to typed object: %w", err)
	}
	return nil
}

// GetID returns the unique ID for the object.
func (hwd *HelmWorkloadDefinition) GetID() uint {
	return *hwd.ID
}

// String returns a string representation of the ojbect.
func (hwd HelmWorkloadDefinition) String() string {
	return fmt.Sprintf("v0.HelmWorkloadDefinition")
}

// NotificationPayload returns the notification payload that is delivered to the
// controller when a change is made.  It includes the object as presented by the
// client when the change was made.
func (hwi *HelmWorkloadInstance) NotificationPayload(
	operation notifications.NotificationOperation,
	requeue bool,
	creationTime int64,
) (*[]byte, error) {
	notif := notifications.Notification{
		CreationTime: &creationTime,
		Object:       hwi,
		Operation:    operation,
	}

	payload, err := json.Marshal(notif)
	if err != nil {
		return &payload, fmt.Errorf("failed to marshal notification payload %+v: %w", hwi, err)
	}

	return &payload, nil
}

// DecodeNotifObject takes the threeport object in the form of a
// map[string]interface and returns the typed object by marshalling into JSON
// and then unmarshalling into the typed object.  We are not using the
// mapstructure library here as that requires custom decode hooks to manage
// fields with non-native go types.
func (hwi *HelmWorkloadInstance) DecodeNotifObject(object interface{}) error {
	jsonObject, err := json.Marshal(object)
	if err != nil {
		return fmt.Errorf("failed to marshal object map from consumed notification message: %w", err)
	}
	if err := json.Unmarshal(jsonObject, &hwi); err != nil {
		return fmt.Errorf("failed to unmarshal json object to typed object: %w", err)
	}
	return nil
}

// GetID returns the unique ID for the object.
func (hwi *HelmWorkloadInstance) GetID() uint {
	return *hwi.ID
}

// String returns a string representation of the ojbect.
func (hwi HelmWorkloadInstance) String() string {
	return fmt.Sprintf("v0.HelmWorkloadInstance")
}
