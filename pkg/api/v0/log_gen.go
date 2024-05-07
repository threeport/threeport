// generated by 'threeport-sdk gen' for API model boilerplate' - do not edit

package v0

import (
	"encoding/json"
	"fmt"
	notifications "github.com/threeport/threeport/pkg/notifications/v0"
)

const (
	ObjectTypeLogBackend           string = "LogBackend"
	ObjectTypeLogStorageDefinition string = "LogStorageDefinition"
	ObjectTypeLogStorageInstance   string = "LogStorageInstance"

	LogStreamName = "logStream"

	LogBackendSubject       = "logBackend.*"
	LogBackendCreateSubject = "logBackend.create"
	LogBackendUpdateSubject = "logBackend.update"
	LogBackendDeleteSubject = "logBackend.delete"

	LogStorageDefinitionSubject       = "logStorageDefinition.*"
	LogStorageDefinitionCreateSubject = "logStorageDefinition.create"
	LogStorageDefinitionUpdateSubject = "logStorageDefinition.update"
	LogStorageDefinitionDeleteSubject = "logStorageDefinition.delete"

	LogStorageInstanceSubject       = "logStorageInstance.*"
	LogStorageInstanceCreateSubject = "logStorageInstance.create"
	LogStorageInstanceUpdateSubject = "logStorageInstance.update"
	LogStorageInstanceDeleteSubject = "logStorageInstance.delete"

	PathLogBackends           = "/v0/log-backends"
	PathLogStorageDefinitions = "/v0/log-storage-definitions"
	PathLogStorageInstances   = "/v0/log-storage-instances"
)

// GetLogBackendSubjects returns the NATS subjects
// for log backends.
func GetLogBackendSubjects() []string {
	return []string{
		LogBackendCreateSubject,
		LogBackendUpdateSubject,
		LogBackendDeleteSubject,
	}
}

// GetLogStorageDefinitionSubjects returns the NATS subjects
// for log storage definitions.
func GetLogStorageDefinitionSubjects() []string {
	return []string{
		LogStorageDefinitionCreateSubject,
		LogStorageDefinitionUpdateSubject,
		LogStorageDefinitionDeleteSubject,
	}
}

// GetLogStorageInstanceSubjects returns the NATS subjects
// for log storage instances.
func GetLogStorageInstanceSubjects() []string {
	return []string{
		LogStorageInstanceCreateSubject,
		LogStorageInstanceUpdateSubject,
		LogStorageInstanceDeleteSubject,
	}
}

// GetLogSubjects returns the NATS subjects
// for all log objects.
func GetLogSubjects() []string {
	var logSubjects []string

	logSubjects = append(logSubjects, GetLogBackendSubjects()...)
	logSubjects = append(logSubjects, GetLogStorageDefinitionSubjects()...)
	logSubjects = append(logSubjects, GetLogStorageInstanceSubjects()...)

	return logSubjects
}

// NotificationPayload returns the notification payload that is delivered to the
// controller when a change is made.  It includes the object as presented by the
// client when the change was made.
func (lb *LogBackend) NotificationPayload(
	operation notifications.NotificationOperation,
	requeue bool,
	creationTime int64,
) (*[]byte, error) {
	notif := notifications.Notification{
		CreationTime: &creationTime,
		Object:       lb,
		Operation:    operation,
	}

	payload, err := json.Marshal(notif)
	if err != nil {
		return &payload, fmt.Errorf("failed to marshal notification payload %+v: %w", lb, err)
	}

	return &payload, nil
}

// DecodeNotifObject takes the threeport object in the form of a
// map[string]interface and returns the typed object by marshalling into JSON
// and then unmarshalling into the typed object.  We are not using the
// mapstructure library here as that requires custom decode hooks to manage
// fields with non-native go types.
func (lb *LogBackend) DecodeNotifObject(object interface{}) error {
	jsonObject, err := json.Marshal(object)
	if err != nil {
		return fmt.Errorf("failed to marshal object map from consumed notification message: %w", err)
	}
	if err := json.Unmarshal(jsonObject, &lb); err != nil {
		return fmt.Errorf("failed to unmarshal json object to typed object: %w", err)
	}
	return nil
}

// GetID returns the unique ID for the object.
func (lb *LogBackend) GetID() uint {
	return *lb.ID
}

// String returns a string representation of the ojbect.
func (lb LogBackend) String() string {
	return "v0.LogBackend"
}

// NotificationPayload returns the notification payload that is delivered to the
// controller when a change is made.  It includes the object as presented by the
// client when the change was made.
func (lsd *LogStorageDefinition) NotificationPayload(
	operation notifications.NotificationOperation,
	requeue bool,
	creationTime int64,
) (*[]byte, error) {
	notif := notifications.Notification{
		CreationTime: &creationTime,
		Object:       lsd,
		Operation:    operation,
	}

	payload, err := json.Marshal(notif)
	if err != nil {
		return &payload, fmt.Errorf("failed to marshal notification payload %+v: %w", lsd, err)
	}

	return &payload, nil
}

// DecodeNotifObject takes the threeport object in the form of a
// map[string]interface and returns the typed object by marshalling into JSON
// and then unmarshalling into the typed object.  We are not using the
// mapstructure library here as that requires custom decode hooks to manage
// fields with non-native go types.
func (lsd *LogStorageDefinition) DecodeNotifObject(object interface{}) error {
	jsonObject, err := json.Marshal(object)
	if err != nil {
		return fmt.Errorf("failed to marshal object map from consumed notification message: %w", err)
	}
	if err := json.Unmarshal(jsonObject, &lsd); err != nil {
		return fmt.Errorf("failed to unmarshal json object to typed object: %w", err)
	}
	return nil
}

// GetID returns the unique ID for the object.
func (lsd *LogStorageDefinition) GetID() uint {
	return *lsd.ID
}

// String returns a string representation of the ojbect.
func (lsd LogStorageDefinition) String() string {
	return "v0.LogStorageDefinition"
}

// NotificationPayload returns the notification payload that is delivered to the
// controller when a change is made.  It includes the object as presented by the
// client when the change was made.
func (lsi *LogStorageInstance) NotificationPayload(
	operation notifications.NotificationOperation,
	requeue bool,
	creationTime int64,
) (*[]byte, error) {
	notif := notifications.Notification{
		CreationTime: &creationTime,
		Object:       lsi,
		Operation:    operation,
	}

	payload, err := json.Marshal(notif)
	if err != nil {
		return &payload, fmt.Errorf("failed to marshal notification payload %+v: %w", lsi, err)
	}

	return &payload, nil
}

// DecodeNotifObject takes the threeport object in the form of a
// map[string]interface and returns the typed object by marshalling into JSON
// and then unmarshalling into the typed object.  We are not using the
// mapstructure library here as that requires custom decode hooks to manage
// fields with non-native go types.
func (lsi *LogStorageInstance) DecodeNotifObject(object interface{}) error {
	jsonObject, err := json.Marshal(object)
	if err != nil {
		return fmt.Errorf("failed to marshal object map from consumed notification message: %w", err)
	}
	if err := json.Unmarshal(jsonObject, &lsi); err != nil {
		return fmt.Errorf("failed to unmarshal json object to typed object: %w", err)
	}
	return nil
}

// GetID returns the unique ID for the object.
func (lsi *LogStorageInstance) GetID() uint {
	return *lsi.ID
}

// String returns a string representation of the ojbect.
func (lsi LogStorageInstance) String() string {
	return "v0.LogStorageInstance"
}
