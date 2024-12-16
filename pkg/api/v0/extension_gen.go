// generated by 'threeport-sdk gen' - do not edit

package v0

import (
	"encoding/json"
	"fmt"
	notifications "github.com/threeport/threeport/pkg/notifications/v0"
)

const (
	ObjectTypeExtensionApi      string = "ExtensionApi"
	ObjectTypeExtensionApiRoute string = "ExtensionApiRoute"

	PathExtensionApiVersions      = "/extension-apis/versions"
	PathExtensionApis             = "/v0/extension-apis"
	PathExtensionApiRouteVersions = "/extension-api-routes/versions"
	PathExtensionApiRoutes        = "/v0/extension-api-routes"
)

// NotificationPayload returns the notification payload that is delivered to the
// controller when a change is made.  It includes the object as presented by the
// client when the change was made.
func (ea *ExtensionApi) NotificationPayload(
	operation notifications.NotificationOperation,
	requeue bool,
	creationTime int64,
) (*[]byte, error) {
	notif := notifications.Notification{
		CreationTime:  &creationTime,
		Object:        ea,
		ObjectVersion: ea.GetVersion(),
		Operation:     operation,
	}

	payload, err := json.Marshal(notif)
	if err != nil {
		return &payload, fmt.Errorf("failed to marshal notification payload %+v: %w", ea, err)
	}

	return &payload, nil
}

// DecodeNotifObject takes the threeport object in the form of a
// map[string]interface and returns the typed object by marshalling into JSON
// and then unmarshalling into the typed object.  We are not using the
// mapstructure library here as that requires custom decode hooks to manage
// fields with non-native go types.
func (ea *ExtensionApi) DecodeNotifObject(object interface{}) error {
	jsonObject, err := json.Marshal(object)
	if err != nil {
		return fmt.Errorf("failed to marshal object map from consumed notification message: %w", err)
	}
	if err := json.Unmarshal(jsonObject, &ea); err != nil {
		return fmt.Errorf("failed to unmarshal json object to typed object: %w", err)
	}
	return nil
}

// GetId returns the unique ID for the object.
func (ea *ExtensionApi) GetId() uint {
	return *ea.ID
}

// Type returns the object type.
func (ea *ExtensionApi) GetType() string {
	return "ExtensionApi"
}

// Version returns the version of the API object.
func (ea *ExtensionApi) GetVersion() string {
	return "v0"
}

// NotificationPayload returns the notification payload that is delivered to the
// controller when a change is made.  It includes the object as presented by the
// client when the change was made.
func (ear *ExtensionApiRoute) NotificationPayload(
	operation notifications.NotificationOperation,
	requeue bool,
	creationTime int64,
) (*[]byte, error) {
	notif := notifications.Notification{
		CreationTime:  &creationTime,
		Object:        ear,
		ObjectVersion: ear.GetVersion(),
		Operation:     operation,
	}

	payload, err := json.Marshal(notif)
	if err != nil {
		return &payload, fmt.Errorf("failed to marshal notification payload %+v: %w", ear, err)
	}

	return &payload, nil
}

// DecodeNotifObject takes the threeport object in the form of a
// map[string]interface and returns the typed object by marshalling into JSON
// and then unmarshalling into the typed object.  We are not using the
// mapstructure library here as that requires custom decode hooks to manage
// fields with non-native go types.
func (ear *ExtensionApiRoute) DecodeNotifObject(object interface{}) error {
	jsonObject, err := json.Marshal(object)
	if err != nil {
		return fmt.Errorf("failed to marshal object map from consumed notification message: %w", err)
	}
	if err := json.Unmarshal(jsonObject, &ear); err != nil {
		return fmt.Errorf("failed to unmarshal json object to typed object: %w", err)
	}
	return nil
}

// GetId returns the unique ID for the object.
func (ear *ExtensionApiRoute) GetId() uint {
	return *ear.ID
}

// Type returns the object type.
func (ear *ExtensionApiRoute) GetType() string {
	return "ExtensionApiRoute"
}

// Version returns the version of the API object.
func (ear *ExtensionApiRoute) GetVersion() string {
	return "v0"
}