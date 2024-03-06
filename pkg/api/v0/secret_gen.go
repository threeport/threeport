// generated by 'threeport-sdk codegen api-model' - do not edit

package v0

import (
	"encoding/json"
	"fmt"
	notifications "github.com/threeport/threeport/pkg/notifications/v0"
)

const (
	ObjectTypeSecretDefinition ObjectType = "SecretDefinition"
	ObjectTypeSecretInstance   ObjectType = "SecretInstance"

	SecretStreamName = "secretStream"

	SecretDefinitionSubject       = "secretDefinition.*"
	SecretDefinitionCreateSubject = "secretDefinition.create"
	SecretDefinitionUpdateSubject = "secretDefinition.update"
	SecretDefinitionDeleteSubject = "secretDefinition.delete"

	SecretInstanceSubject       = "secretInstance.*"
	SecretInstanceCreateSubject = "secretInstance.create"
	SecretInstanceUpdateSubject = "secretInstance.update"
	SecretInstanceDeleteSubject = "secretInstance.delete"

	PathSecretDefinitions = "/v0/secret-definitions"
	PathSecretInstances   = "/v0/secret-instances"
)

// GetSecretDefinitionSubjects returns the NATS subjects
// for secret definitions.
func GetSecretDefinitionSubjects() []string {
	return []string{
		SecretDefinitionCreateSubject,
		SecretDefinitionUpdateSubject,
		SecretDefinitionDeleteSubject,
	}
}

// GetSecretInstanceSubjects returns the NATS subjects
// for secret instances.
func GetSecretInstanceSubjects() []string {
	return []string{
		SecretInstanceCreateSubject,
		SecretInstanceUpdateSubject,
		SecretInstanceDeleteSubject,
	}
}

// GetSecretSubjects returns the NATS subjects
// for all secret objects.
func GetSecretSubjects() []string {
	var secretSubjects []string

	secretSubjects = append(secretSubjects, GetSecretDefinitionSubjects()...)
	secretSubjects = append(secretSubjects, GetSecretInstanceSubjects()...)

	return secretSubjects
}

// NotificationPayload returns the notification payload that is delivered to the
// controller when a change is made.  It includes the object as presented by the
// client when the change was made.
func (sd *SecretDefinition) NotificationPayload(
	operation notifications.NotificationOperation,
	requeue bool,
	creationTime int64,
) (*[]byte, error) {
	notif := notifications.Notification{
		CreationTime: &creationTime,
		Object:       sd,
		Operation:    operation,
	}

	payload, err := json.Marshal(notif)
	if err != nil {
		return &payload, fmt.Errorf("failed to marshal notification payload %+v: %w", sd, err)
	}

	return &payload, nil
}

// DecodeNotifObject takes the threeport object in the form of a
// map[string]interface and returns the typed object by marshalling into JSON
// and then unmarshalling into the typed object.  We are not using the
// mapstructure library here as that requires custom decode hooks to manage
// fields with non-native go types.
func (sd *SecretDefinition) DecodeNotifObject(object interface{}) error {
	jsonObject, err := json.Marshal(object)
	if err != nil {
		return fmt.Errorf("failed to marshal object map from consumed notification message: %w", err)
	}
	if err := json.Unmarshal(jsonObject, &sd); err != nil {
		return fmt.Errorf("failed to unmarshal json object to typed object: %w", err)
	}
	return nil
}

// GetID returns the unique ID for the object.
func (sd *SecretDefinition) GetID() uint {
	return *sd.ID
}

// String returns a string representation of the ojbect.
func (sd SecretDefinition) String() string {
	return fmt.Sprintf("v0.SecretDefinition")
}

// NotificationPayload returns the notification payload that is delivered to the
// controller when a change is made.  It includes the object as presented by the
// client when the change was made.
func (si *SecretInstance) NotificationPayload(
	operation notifications.NotificationOperation,
	requeue bool,
	creationTime int64,
) (*[]byte, error) {
	notif := notifications.Notification{
		CreationTime: &creationTime,
		Object:       si,
		Operation:    operation,
	}

	payload, err := json.Marshal(notif)
	if err != nil {
		return &payload, fmt.Errorf("failed to marshal notification payload %+v: %w", si, err)
	}

	return &payload, nil
}

// DecodeNotifObject takes the threeport object in the form of a
// map[string]interface and returns the typed object by marshalling into JSON
// and then unmarshalling into the typed object.  We are not using the
// mapstructure library here as that requires custom decode hooks to manage
// fields with non-native go types.
func (si *SecretInstance) DecodeNotifObject(object interface{}) error {
	jsonObject, err := json.Marshal(object)
	if err != nil {
		return fmt.Errorf("failed to marshal object map from consumed notification message: %w", err)
	}
	if err := json.Unmarshal(jsonObject, &si); err != nil {
		return fmt.Errorf("failed to unmarshal json object to typed object: %w", err)
	}
	return nil
}

// GetID returns the unique ID for the object.
func (si *SecretInstance) GetID() uint {
	return *si.ID
}

// String returns a string representation of the ojbect.
func (si SecretInstance) String() string {
	return fmt.Sprintf("v0.SecretInstance")
}
