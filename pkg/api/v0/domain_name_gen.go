// generated by 'threeport-codegen api-model' - do not edit

package v0

import (
	"encoding/json"
	"fmt"
	notifications "github.com/threeport/threeport/pkg/notifications/v0"
)

const (
	ObjectTypeDomainNameDefinition ObjectType = "DomainNameDefinition"
	ObjectTypeDomainNameInstance   ObjectType = "DomainNameInstance"

	DomainNameStreamName = "domainNameStream"

	DomainNameDefinitionSubject       = "domainNameDefinition.*"
	DomainNameDefinitionCreateSubject = "domainNameDefinition.create"
	DomainNameDefinitionUpdateSubject = "domainNameDefinition.update"
	DomainNameDefinitionDeleteSubject = "domainNameDefinition.delete"

	DomainNameInstanceSubject       = "domainNameInstance.*"
	DomainNameInstanceCreateSubject = "domainNameInstance.create"
	DomainNameInstanceUpdateSubject = "domainNameInstance.update"
	DomainNameInstanceDeleteSubject = "domainNameInstance.delete"

	PathDomainNameDefinitions = "/v0/domain-name-definitions"
	PathDomainNameInstances   = "/v0/domain-name-instances"
)

// GetDomainNameDefinitionSubjects returns the NATS subjects
// for domain name definitions.
func GetDomainNameDefinitionSubjects() []string {
	return []string{
		DomainNameDefinitionCreateSubject,
		DomainNameDefinitionUpdateSubject,
		DomainNameDefinitionDeleteSubject,
	}
}

// GetDomainNameInstanceSubjects returns the NATS subjects
// for domain name instances.
func GetDomainNameInstanceSubjects() []string {
	return []string{
		DomainNameInstanceCreateSubject,
		DomainNameInstanceUpdateSubject,
		DomainNameInstanceDeleteSubject,
	}
}

// GetDomainNameSubjects returns the NATS subjects
// for all domain name objects.
func GetDomainNameSubjects() []string {
	var domainNameSubjects []string

	domainNameSubjects = append(domainNameSubjects, GetDomainNameDefinitionSubjects()...)
	domainNameSubjects = append(domainNameSubjects, GetDomainNameInstanceSubjects()...)

	return domainNameSubjects
}

// NotificationPayload returns the notification payload that is delivered to the
// controller when a change is made.  It includes the object as presented by the
// client when the change was made.
func (dnd *DomainNameDefinition) NotificationPayload(
	operation notifications.NotificationOperation,
	requeue bool,
	lastDelay int64,
) (*[]byte, error) {
	notif := notifications.Notification{
		LastRequeueDelay: &lastDelay,
		Object:           dnd,
		Operation:        operation,
		Requeue:          requeue,
	}

	payload, err := json.Marshal(notif)
	if err != nil {
		return &payload, fmt.Errorf("failed to marshal notification payload %+v: %w", dnd, err)
	}

	return &payload, nil
}

// DecodeNotifObject takes the threeport object in the form of a
// map[string]interface and returns the typed object by marshalling into JSON
// and then unmarshalling into the typed object.  We are not using the
// mapstructure library here as that requires custom decode hooks to manage
// fields with non-native go types.
func (dnd *DomainNameDefinition) DecodeNotifObject(object interface{}) error {
	jsonObject, err := json.Marshal(object)
	if err != nil {
		return fmt.Errorf("failed to marshal object map from consumed notification message", err)
	}
	if err := json.Unmarshal(jsonObject, &dnd); err != nil {
		return fmt.Errorf("failed to unmarshal json object to typed object", err)
	}
	return nil
}

// GetID returns the unique ID for the object.
func (dnd *DomainNameDefinition) GetID() uint {
	return *dnd.ID
}

// String returns a string representation of the ojbect.
func (dnd DomainNameDefinition) String() string {
	return fmt.Sprintf("v0.DomainNameDefinition")
}

// NotificationPayload returns the notification payload that is delivered to the
// controller when a change is made.  It includes the object as presented by the
// client when the change was made.
func (dni *DomainNameInstance) NotificationPayload(
	operation notifications.NotificationOperation,
	requeue bool,
	lastDelay int64,
) (*[]byte, error) {
	notif := notifications.Notification{
		LastRequeueDelay: &lastDelay,
		Object:           dni,
		Operation:        operation,
		Requeue:          requeue,
	}

	payload, err := json.Marshal(notif)
	if err != nil {
		return &payload, fmt.Errorf("failed to marshal notification payload %+v: %w", dni, err)
	}

	return &payload, nil
}

// DecodeNotifObject takes the threeport object in the form of a
// map[string]interface and returns the typed object by marshalling into JSON
// and then unmarshalling into the typed object.  We are not using the
// mapstructure library here as that requires custom decode hooks to manage
// fields with non-native go types.
func (dni *DomainNameInstance) DecodeNotifObject(object interface{}) error {
	jsonObject, err := json.Marshal(object)
	if err != nil {
		return fmt.Errorf("failed to marshal object map from consumed notification message", err)
	}
	if err := json.Unmarshal(jsonObject, &dni); err != nil {
		return fmt.Errorf("failed to unmarshal json object to typed object", err)
	}
	return nil
}

// GetID returns the unique ID for the object.
func (dni *DomainNameInstance) GetID() uint {
	return *dni.ID
}

// String returns a string representation of the ojbect.
func (dni DomainNameInstance) String() string {
	return fmt.Sprintf("v0.DomainNameInstance")
}
