// generated by 'threeport-sdk gen' - do not edit

package v0

import (
	"encoding/json"
	"fmt"
	notifications "github.com/threeport/threeport/pkg/notifications/v0"
)

const (
	ObjectTypeProfile string = "Profile"
	ObjectTypeTier    string = "Tier"

	PathProfileVersions = "/profiles/versions"
	PathProfiles        = "/v0/profiles"
	PathTierVersions    = "/tiers/versions"
	PathTiers           = "/v0/tiers"
)

// NotificationPayload returns the notification payload that is delivered to the
// controller when a change is made.  It includes the object as presented by the
// client when the change was made.
func (p *Profile) NotificationPayload(
	operation notifications.NotificationOperation,
	requeue bool,
	creationTime int64,
) (*[]byte, error) {
	notif := notifications.Notification{
		CreationTime:  &creationTime,
		Object:        p,
		ObjectVersion: p.GetVersion(),
		Operation:     operation,
	}

	payload, err := json.Marshal(notif)
	if err != nil {
		return &payload, fmt.Errorf("failed to marshal notification payload %+v: %w", p, err)
	}

	return &payload, nil
}

// DecodeNotifObject takes the threeport object in the form of a
// map[string]interface and returns the typed object by marshalling into JSON
// and then unmarshalling into the typed object.  We are not using the
// mapstructure library here as that requires custom decode hooks to manage
// fields with non-native go types.
func (p *Profile) DecodeNotifObject(object interface{}) error {
	jsonObject, err := json.Marshal(object)
	if err != nil {
		return fmt.Errorf("failed to marshal object map from consumed notification message: %w", err)
	}
	if err := json.Unmarshal(jsonObject, &p); err != nil {
		return fmt.Errorf("failed to unmarshal json object to typed object: %w", err)
	}
	return nil
}

// GetId returns the unique ID for the object.
func (p *Profile) GetId() uint {
	return *p.ID
}

// Type returns the object type.
func (p *Profile) GetType() string {
	return "Profile"
}

// Version returns the version of the API object.
func (p *Profile) GetVersion() string {
	return "v0"
}

// NotificationPayload returns the notification payload that is delivered to the
// controller when a change is made.  It includes the object as presented by the
// client when the change was made.
func (t *Tier) NotificationPayload(
	operation notifications.NotificationOperation,
	requeue bool,
	creationTime int64,
) (*[]byte, error) {
	notif := notifications.Notification{
		CreationTime:  &creationTime,
		Object:        t,
		ObjectVersion: t.GetVersion(),
		Operation:     operation,
	}

	payload, err := json.Marshal(notif)
	if err != nil {
		return &payload, fmt.Errorf("failed to marshal notification payload %+v: %w", t, err)
	}

	return &payload, nil
}

// DecodeNotifObject takes the threeport object in the form of a
// map[string]interface and returns the typed object by marshalling into JSON
// and then unmarshalling into the typed object.  We are not using the
// mapstructure library here as that requires custom decode hooks to manage
// fields with non-native go types.
func (t *Tier) DecodeNotifObject(object interface{}) error {
	jsonObject, err := json.Marshal(object)
	if err != nil {
		return fmt.Errorf("failed to marshal object map from consumed notification message: %w", err)
	}
	if err := json.Unmarshal(jsonObject, &t); err != nil {
		return fmt.Errorf("failed to unmarshal json object to typed object: %w", err)
	}
	return nil
}

// GetId returns the unique ID for the object.
func (t *Tier) GetId() uint {
	return *t.ID
}

// Type returns the object type.
func (t *Tier) GetType() string {
	return "Tier"
}

// Version returns the version of the API object.
func (t *Tier) GetVersion() string {
	return "v0"
}
