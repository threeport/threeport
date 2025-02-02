// generated by 'threeport-sdk gen' - do not edit

package v0

import (
	"encoding/json"
	"fmt"
	notifications "github.com/threeport/threeport/pkg/notifications/v0"
)

const (
	ObjectTypeAttachedObjectReference string = "AttachedObjectReference"

	PathAttachedObjectReferenceVersions = "/attached-object-references/versions"
	PathAttachedObjectReferences        = "/v0/attached-object-references"
)

// NotificationPayload returns the notification payload that is delivered to the
// controller when a change is made.  It includes the object as presented by the
// client when the change was made.
func (aor *AttachedObjectReference) NotificationPayload(
	operation notifications.NotificationOperation,
	requeue bool,
	creationTime int64,
) (*[]byte, error) {
	notif := notifications.Notification{
		CreationTime:  &creationTime,
		Object:        aor,
		ObjectVersion: aor.GetVersion(),
		Operation:     operation,
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

// GetId returns the unique ID for the object.
func (aor *AttachedObjectReference) GetId() uint {
	return *aor.ID
}

// Type returns the object type.
func (aor *AttachedObjectReference) GetType() string {
	return "AttachedObjectReference"
}

// Version returns the version of the API object.
func (aor *AttachedObjectReference) GetVersion() string {
	return "v0"
}
