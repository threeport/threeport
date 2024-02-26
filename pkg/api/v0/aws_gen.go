// generated by 'threeport-sdk codegen api-model' - do not edit

package v0

import (
	"encoding/json"
	"fmt"
	notifications "github.com/threeport/threeport/pkg/notifications/v0"
)

const (
	ObjectTypeAwsAccount                        ObjectType = "AwsAccount"
	ObjectTypeAwsEksKubernetesRuntimeDefinition ObjectType = "AwsEksKubernetesRuntimeDefinition"
	ObjectTypeAwsEksKubernetesRuntimeInstance   ObjectType = "AwsEksKubernetesRuntimeInstance"
	ObjectTypeAwsRelationalDatabaseDefinition   ObjectType = "AwsRelationalDatabaseDefinition"
	ObjectTypeAwsRelationalDatabaseInstance     ObjectType = "AwsRelationalDatabaseInstance"
	ObjectTypeAwsObjectStorageBucketDefinition  ObjectType = "AwsObjectStorageBucketDefinition"
	ObjectTypeAwsObjectStorageBucketInstance    ObjectType = "AwsObjectStorageBucketInstance"

	AwsStreamName = "awsStream"

	AwsAccountSubject       = "awsAccount.*"
	AwsAccountCreateSubject = "awsAccount.create"
	AwsAccountUpdateSubject = "awsAccount.update"
	AwsAccountDeleteSubject = "awsAccount.delete"

	AwsEksKubernetesRuntimeDefinitionSubject       = "awsEksKubernetesRuntimeDefinition.*"
	AwsEksKubernetesRuntimeDefinitionCreateSubject = "awsEksKubernetesRuntimeDefinition.create"
	AwsEksKubernetesRuntimeDefinitionUpdateSubject = "awsEksKubernetesRuntimeDefinition.update"
	AwsEksKubernetesRuntimeDefinitionDeleteSubject = "awsEksKubernetesRuntimeDefinition.delete"

	AwsEksKubernetesRuntimeInstanceSubject       = "awsEksKubernetesRuntimeInstance.*"
	AwsEksKubernetesRuntimeInstanceCreateSubject = "awsEksKubernetesRuntimeInstance.create"
	AwsEksKubernetesRuntimeInstanceUpdateSubject = "awsEksKubernetesRuntimeInstance.update"
	AwsEksKubernetesRuntimeInstanceDeleteSubject = "awsEksKubernetesRuntimeInstance.delete"

	AwsRelationalDatabaseDefinitionSubject       = "awsRelationalDatabaseDefinition.*"
	AwsRelationalDatabaseDefinitionCreateSubject = "awsRelationalDatabaseDefinition.create"
	AwsRelationalDatabaseDefinitionUpdateSubject = "awsRelationalDatabaseDefinition.update"
	AwsRelationalDatabaseDefinitionDeleteSubject = "awsRelationalDatabaseDefinition.delete"

	AwsRelationalDatabaseInstanceSubject       = "awsRelationalDatabaseInstance.*"
	AwsRelationalDatabaseInstanceCreateSubject = "awsRelationalDatabaseInstance.create"
	AwsRelationalDatabaseInstanceUpdateSubject = "awsRelationalDatabaseInstance.update"
	AwsRelationalDatabaseInstanceDeleteSubject = "awsRelationalDatabaseInstance.delete"

	AwsObjectStorageBucketDefinitionSubject       = "awsObjectStorageBucketDefinition.*"
	AwsObjectStorageBucketDefinitionCreateSubject = "awsObjectStorageBucketDefinition.create"
	AwsObjectStorageBucketDefinitionUpdateSubject = "awsObjectStorageBucketDefinition.update"
	AwsObjectStorageBucketDefinitionDeleteSubject = "awsObjectStorageBucketDefinition.delete"

	AwsObjectStorageBucketInstanceSubject       = "awsObjectStorageBucketInstance.*"
	AwsObjectStorageBucketInstanceCreateSubject = "awsObjectStorageBucketInstance.create"
	AwsObjectStorageBucketInstanceUpdateSubject = "awsObjectStorageBucketInstance.update"
	AwsObjectStorageBucketInstanceDeleteSubject = "awsObjectStorageBucketInstance.delete"

	PathAwsAccounts                        = "/v0/aws-accounts"
	PathAwsEksKubernetesRuntimeDefinitions = "/v0/aws-eks-kubernetes-runtime-definitions"
	PathAwsEksKubernetesRuntimeInstances   = "/v0/aws-eks-kubernetes-runtime-instances"
	PathAwsRelationalDatabaseDefinitions   = "/v0/aws-relational-database-definitions"
	PathAwsRelationalDatabaseInstances     = "/v0/aws-relational-database-instances"
	PathAwsObjectStorageBucketDefinitions  = "/v0/aws-object-storage-bucket-definitions"
	PathAwsObjectStorageBucketInstances    = "/v0/aws-object-storage-bucket-instances"
)

// GetAwsAccountSubjects returns the NATS subjects
// for aws accounts.
func GetAwsAccountSubjects() []string {
	return []string{
		AwsAccountCreateSubject,
		AwsAccountUpdateSubject,
		AwsAccountDeleteSubject,
	}
}

// GetAwsEksKubernetesRuntimeDefinitionSubjects returns the NATS subjects
// for aws eks kubernetes runtime definitions.
func GetAwsEksKubernetesRuntimeDefinitionSubjects() []string {
	return []string{
		AwsEksKubernetesRuntimeDefinitionCreateSubject,
		AwsEksKubernetesRuntimeDefinitionUpdateSubject,
		AwsEksKubernetesRuntimeDefinitionDeleteSubject,
	}
}

// GetAwsEksKubernetesRuntimeInstanceSubjects returns the NATS subjects
// for aws eks kubernetes runtime instances.
func GetAwsEksKubernetesRuntimeInstanceSubjects() []string {
	return []string{
		AwsEksKubernetesRuntimeInstanceCreateSubject,
		AwsEksKubernetesRuntimeInstanceUpdateSubject,
		AwsEksKubernetesRuntimeInstanceDeleteSubject,
	}
}

// GetAwsRelationalDatabaseDefinitionSubjects returns the NATS subjects
// for aws relational database definitions.
func GetAwsRelationalDatabaseDefinitionSubjects() []string {
	return []string{
		AwsRelationalDatabaseDefinitionCreateSubject,
		AwsRelationalDatabaseDefinitionUpdateSubject,
		AwsRelationalDatabaseDefinitionDeleteSubject,
	}
}

// GetAwsRelationalDatabaseInstanceSubjects returns the NATS subjects
// for aws relational database instances.
func GetAwsRelationalDatabaseInstanceSubjects() []string {
	return []string{
		AwsRelationalDatabaseInstanceCreateSubject,
		AwsRelationalDatabaseInstanceUpdateSubject,
		AwsRelationalDatabaseInstanceDeleteSubject,
	}
}

// GetAwsObjectStorageBucketDefinitionSubjects returns the NATS subjects
// for aws object storage bucket definitions.
func GetAwsObjectStorageBucketDefinitionSubjects() []string {
	return []string{
		AwsObjectStorageBucketDefinitionCreateSubject,
		AwsObjectStorageBucketDefinitionUpdateSubject,
		AwsObjectStorageBucketDefinitionDeleteSubject,
	}
}

// GetAwsObjectStorageBucketInstanceSubjects returns the NATS subjects
// for aws object storage bucket instances.
func GetAwsObjectStorageBucketInstanceSubjects() []string {
	return []string{
		AwsObjectStorageBucketInstanceCreateSubject,
		AwsObjectStorageBucketInstanceUpdateSubject,
		AwsObjectStorageBucketInstanceDeleteSubject,
	}
}

// GetAwsSubjects returns the NATS subjects
// for all aws objects.
func GetAwsSubjects() []string {
	var awsSubjects []string

	awsSubjects = append(awsSubjects, GetAwsAccountSubjects()...)
	awsSubjects = append(awsSubjects, GetAwsEksKubernetesRuntimeDefinitionSubjects()...)
	awsSubjects = append(awsSubjects, GetAwsEksKubernetesRuntimeInstanceSubjects()...)
	awsSubjects = append(awsSubjects, GetAwsRelationalDatabaseDefinitionSubjects()...)
	awsSubjects = append(awsSubjects, GetAwsRelationalDatabaseInstanceSubjects()...)
	awsSubjects = append(awsSubjects, GetAwsObjectStorageBucketDefinitionSubjects()...)
	awsSubjects = append(awsSubjects, GetAwsObjectStorageBucketInstanceSubjects()...)

	return awsSubjects
}

// NotificationPayload returns the notification payload that is delivered to the
// controller when a change is made.  It includes the object as presented by the
// client when the change was made.
func (aa *AwsAccount) NotificationPayload(
	operation notifications.NotificationOperation,
	requeue bool,
	creationTime int64,
) (*[]byte, error) {
	notif := notifications.Notification{
		CreationTime: &creationTime,
		Object:       aa,
		Operation:    operation,
	}

	payload, err := json.Marshal(notif)
	if err != nil {
		return &payload, fmt.Errorf("failed to marshal notification payload %+v: %w", aa, err)
	}

	return &payload, nil
}

// DecodeNotifObject takes the threeport object in the form of a
// map[string]interface and returns the typed object by marshalling into JSON
// and then unmarshalling into the typed object.  We are not using the
// mapstructure library here as that requires custom decode hooks to manage
// fields with non-native go types.
func (aa *AwsAccount) DecodeNotifObject(object interface{}) error {
	jsonObject, err := json.Marshal(object)
	if err != nil {
		return fmt.Errorf("failed to marshal object map from consumed notification message: %w", err)
	}
	if err := json.Unmarshal(jsonObject, &aa); err != nil {
		return fmt.Errorf("failed to unmarshal json object to typed object: %w", err)
	}
	return nil
}

// GetID returns the unique ID for the object.
func (aa *AwsAccount) GetID() uint {
	return *aa.ID
}

// String returns a string representation of the ojbect.
func (aa AwsAccount) String() string {
	return fmt.Sprintf("v0.AwsAccount")
}

// NotificationPayload returns the notification payload that is delivered to the
// controller when a change is made.  It includes the object as presented by the
// client when the change was made.
func (aekrd *AwsEksKubernetesRuntimeDefinition) NotificationPayload(
	operation notifications.NotificationOperation,
	requeue bool,
	creationTime int64,
) (*[]byte, error) {
	notif := notifications.Notification{
		CreationTime: &creationTime,
		Object:       aekrd,
		Operation:    operation,
	}

	payload, err := json.Marshal(notif)
	if err != nil {
		return &payload, fmt.Errorf("failed to marshal notification payload %+v: %w", aekrd, err)
	}

	return &payload, nil
}

// DecodeNotifObject takes the threeport object in the form of a
// map[string]interface and returns the typed object by marshalling into JSON
// and then unmarshalling into the typed object.  We are not using the
// mapstructure library here as that requires custom decode hooks to manage
// fields with non-native go types.
func (aekrd *AwsEksKubernetesRuntimeDefinition) DecodeNotifObject(object interface{}) error {
	jsonObject, err := json.Marshal(object)
	if err != nil {
		return fmt.Errorf("failed to marshal object map from consumed notification message: %w", err)
	}
	if err := json.Unmarshal(jsonObject, &aekrd); err != nil {
		return fmt.Errorf("failed to unmarshal json object to typed object: %w", err)
	}
	return nil
}

// GetID returns the unique ID for the object.
func (aekrd *AwsEksKubernetesRuntimeDefinition) GetID() uint {
	return *aekrd.ID
}

// String returns a string representation of the ojbect.
func (aekrd AwsEksKubernetesRuntimeDefinition) String() string {
	return fmt.Sprintf("v0.AwsEksKubernetesRuntimeDefinition")
}

// NotificationPayload returns the notification payload that is delivered to the
// controller when a change is made.  It includes the object as presented by the
// client when the change was made.
func (aekri *AwsEksKubernetesRuntimeInstance) NotificationPayload(
	operation notifications.NotificationOperation,
	requeue bool,
	creationTime int64,
) (*[]byte, error) {
	notif := notifications.Notification{
		CreationTime: &creationTime,
		Object:       aekri,
		Operation:    operation,
	}

	payload, err := json.Marshal(notif)
	if err != nil {
		return &payload, fmt.Errorf("failed to marshal notification payload %+v: %w", aekri, err)
	}

	return &payload, nil
}

// DecodeNotifObject takes the threeport object in the form of a
// map[string]interface and returns the typed object by marshalling into JSON
// and then unmarshalling into the typed object.  We are not using the
// mapstructure library here as that requires custom decode hooks to manage
// fields with non-native go types.
func (aekri *AwsEksKubernetesRuntimeInstance) DecodeNotifObject(object interface{}) error {
	jsonObject, err := json.Marshal(object)
	if err != nil {
		return fmt.Errorf("failed to marshal object map from consumed notification message: %w", err)
	}
	if err := json.Unmarshal(jsonObject, &aekri); err != nil {
		return fmt.Errorf("failed to unmarshal json object to typed object: %w", err)
	}
	return nil
}

// GetID returns the unique ID for the object.
func (aekri *AwsEksKubernetesRuntimeInstance) GetID() uint {
	return *aekri.ID
}

// String returns a string representation of the ojbect.
func (aekri AwsEksKubernetesRuntimeInstance) String() string {
	return fmt.Sprintf("v0.AwsEksKubernetesRuntimeInstance")
}

// NotificationPayload returns the notification payload that is delivered to the
// controller when a change is made.  It includes the object as presented by the
// client when the change was made.
func (ardd *AwsRelationalDatabaseDefinition) NotificationPayload(
	operation notifications.NotificationOperation,
	requeue bool,
	creationTime int64,
) (*[]byte, error) {
	notif := notifications.Notification{
		CreationTime: &creationTime,
		Object:       ardd,
		Operation:    operation,
	}

	payload, err := json.Marshal(notif)
	if err != nil {
		return &payload, fmt.Errorf("failed to marshal notification payload %+v: %w", ardd, err)
	}

	return &payload, nil
}

// DecodeNotifObject takes the threeport object in the form of a
// map[string]interface and returns the typed object by marshalling into JSON
// and then unmarshalling into the typed object.  We are not using the
// mapstructure library here as that requires custom decode hooks to manage
// fields with non-native go types.
func (ardd *AwsRelationalDatabaseDefinition) DecodeNotifObject(object interface{}) error {
	jsonObject, err := json.Marshal(object)
	if err != nil {
		return fmt.Errorf("failed to marshal object map from consumed notification message: %w", err)
	}
	if err := json.Unmarshal(jsonObject, &ardd); err != nil {
		return fmt.Errorf("failed to unmarshal json object to typed object: %w", err)
	}
	return nil
}

// GetID returns the unique ID for the object.
func (ardd *AwsRelationalDatabaseDefinition) GetID() uint {
	return *ardd.ID
}

// String returns a string representation of the ojbect.
func (ardd AwsRelationalDatabaseDefinition) String() string {
	return fmt.Sprintf("v0.AwsRelationalDatabaseDefinition")
}

// NotificationPayload returns the notification payload that is delivered to the
// controller when a change is made.  It includes the object as presented by the
// client when the change was made.
func (ardi *AwsRelationalDatabaseInstance) NotificationPayload(
	operation notifications.NotificationOperation,
	requeue bool,
	creationTime int64,
) (*[]byte, error) {
	notif := notifications.Notification{
		CreationTime: &creationTime,
		Object:       ardi,
		Operation:    operation,
	}

	payload, err := json.Marshal(notif)
	if err != nil {
		return &payload, fmt.Errorf("failed to marshal notification payload %+v: %w", ardi, err)
	}

	return &payload, nil
}

// DecodeNotifObject takes the threeport object in the form of a
// map[string]interface and returns the typed object by marshalling into JSON
// and then unmarshalling into the typed object.  We are not using the
// mapstructure library here as that requires custom decode hooks to manage
// fields with non-native go types.
func (ardi *AwsRelationalDatabaseInstance) DecodeNotifObject(object interface{}) error {
	jsonObject, err := json.Marshal(object)
	if err != nil {
		return fmt.Errorf("failed to marshal object map from consumed notification message: %w", err)
	}
	if err := json.Unmarshal(jsonObject, &ardi); err != nil {
		return fmt.Errorf("failed to unmarshal json object to typed object: %w", err)
	}
	return nil
}

// GetID returns the unique ID for the object.
func (ardi *AwsRelationalDatabaseInstance) GetID() uint {
	return *ardi.ID
}

// String returns a string representation of the ojbect.
func (ardi AwsRelationalDatabaseInstance) String() string {
	return fmt.Sprintf("v0.AwsRelationalDatabaseInstance")
}

// NotificationPayload returns the notification payload that is delivered to the
// controller when a change is made.  It includes the object as presented by the
// client when the change was made.
func (aosbd *AwsObjectStorageBucketDefinition) NotificationPayload(
	operation notifications.NotificationOperation,
	requeue bool,
	creationTime int64,
) (*[]byte, error) {
	notif := notifications.Notification{
		CreationTime: &creationTime,
		Object:       aosbd,
		Operation:    operation,
	}

	payload, err := json.Marshal(notif)
	if err != nil {
		return &payload, fmt.Errorf("failed to marshal notification payload %+v: %w", aosbd, err)
	}

	return &payload, nil
}

// DecodeNotifObject takes the threeport object in the form of a
// map[string]interface and returns the typed object by marshalling into JSON
// and then unmarshalling into the typed object.  We are not using the
// mapstructure library here as that requires custom decode hooks to manage
// fields with non-native go types.
func (aosbd *AwsObjectStorageBucketDefinition) DecodeNotifObject(object interface{}) error {
	jsonObject, err := json.Marshal(object)
	if err != nil {
		return fmt.Errorf("failed to marshal object map from consumed notification message: %w", err)
	}
	if err := json.Unmarshal(jsonObject, &aosbd); err != nil {
		return fmt.Errorf("failed to unmarshal json object to typed object: %w", err)
	}
	return nil
}

// GetID returns the unique ID for the object.
func (aosbd *AwsObjectStorageBucketDefinition) GetID() uint {
	return *aosbd.ID
}

// String returns a string representation of the ojbect.
func (aosbd AwsObjectStorageBucketDefinition) String() string {
	return fmt.Sprintf("v0.AwsObjectStorageBucketDefinition")
}

// NotificationPayload returns the notification payload that is delivered to the
// controller when a change is made.  It includes the object as presented by the
// client when the change was made.
func (aosbi *AwsObjectStorageBucketInstance) NotificationPayload(
	operation notifications.NotificationOperation,
	requeue bool,
	creationTime int64,
) (*[]byte, error) {
	notif := notifications.Notification{
		CreationTime: &creationTime,
		Object:       aosbi,
		Operation:    operation,
	}

	payload, err := json.Marshal(notif)
	if err != nil {
		return &payload, fmt.Errorf("failed to marshal notification payload %+v: %w", aosbi, err)
	}

	return &payload, nil
}

// DecodeNotifObject takes the threeport object in the form of a
// map[string]interface and returns the typed object by marshalling into JSON
// and then unmarshalling into the typed object.  We are not using the
// mapstructure library here as that requires custom decode hooks to manage
// fields with non-native go types.
func (aosbi *AwsObjectStorageBucketInstance) DecodeNotifObject(object interface{}) error {
	jsonObject, err := json.Marshal(object)
	if err != nil {
		return fmt.Errorf("failed to marshal object map from consumed notification message: %w", err)
	}
	if err := json.Unmarshal(jsonObject, &aosbi); err != nil {
		return fmt.Errorf("failed to unmarshal json object to typed object: %w", err)
	}
	return nil
}

// GetID returns the unique ID for the object.
func (aosbi *AwsObjectStorageBucketInstance) GetID() uint {
	return *aosbi.ID
}

// String returns a string representation of the ojbect.
func (aosbi AwsObjectStorageBucketInstance) String() string {
	return fmt.Sprintf("v0.AwsObjectStorageBucketInstance")
}
