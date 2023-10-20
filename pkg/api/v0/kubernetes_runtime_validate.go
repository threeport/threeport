// +threeport-codegen route-exclude
// +threeport-codegen database-exclude
package v0

import (
	"errors"
	"fmt"
	"os"
	"reflect"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"github.com/threeport/threeport/internal/kubernetes-runtime/mapping"
	"github.com/threeport/threeport/pkg/encryption/v0"
)

// KubernetesRuntimeInfraProvider indicates which infrastructure provider is being
// used to run the kubernetes cluster for the threeport control plane.
type KubernetesRuntimeInfraProvider string

const (
	KubernetesRuntimeInfraProviderKind = "kind"
	KubernetesRuntimeInfraProviderEKS  = "eks"
)

// SupportedInfraProviders returns all supported infra providers.
func SupportedInfraProviders() []KubernetesRuntimeInfraProvider {
	return []KubernetesRuntimeInfraProvider{
		KubernetesRuntimeInfraProviderKind,
		KubernetesRuntimeInfraProviderEKS,
	}
}

// KubernetesRuntimeDefinitionValidationErr is an error that accepts a custom
// message when validation errors occur for the KubernetesRuntimeDefinition
// object.
type KubernetesRuntimeDefinitionValidationErr struct {
	Message string
}

// Error returns the custom message generated during validation.
func (e *KubernetesRuntimeDefinitionValidationErr) Error() string {
	return e.Message
}

// KubernetesRuntimeInstanceValidationErr is an error that accepts a custom
// message when validation errors occur for the KubernetesRuntimeInstance
// object.
type KubernetesRuntimeInstanceValidationErr struct {
	Message string
}

// Error returns the custom message generated during validation.
func (e *KubernetesRuntimeInstanceValidationErr) Error() string {
	return e.Message
}

// BeforeCreate validates a KubernetesRuntimeDefinition object before creating
// in the database.
func (k *KubernetesRuntimeDefinition) BeforeCreate(tx *gorm.DB) error {
	// validate infra provider is one of the supported types
	infraProviders := SupportedInfraProviders()
	providerValid := false
	for _, provider := range infraProviders {
		if *k.InfraProvider == string(provider) {
			providerValid = true
			break
		}
	}
	if !providerValid {
		msg := fmt.Sprintf(
			"%s provider is not valid, valid providers: %s",
			*k.InfraProvider,
			infraProviders,
		)
		return &KubernetesRuntimeDefinitionValidationErr{msg}
	}

	return nil
}

// BeforeCreate validates a KubernetesRuntimeInstance before persisting to the
// database.
func (k *KubernetesRuntimeInstance) BeforeCreate(tx *gorm.DB) error {
	// validate location
	if !mapping.ValidLocation(*k.Location) {
		msg := fmt.Sprintf("location %s is not a supported threeport location for a kubernetes runtime instance", *k.Location)
		return &KubernetesRuntimeInstanceValidationErr{msg}
	}

	// encrypt sensitive values
	var encryptionKey = os.Getenv("ENCRYPTION_KEY")
	if encryptionKey == "" {
		return errors.New("environment variable ENCRYPTION_KEY is not set")
	}
	createdObj := *k
	objVal := reflect.ValueOf(&createdObj).Elem()
	objType := objVal.Type()
	for i := 0; i < objType.NumField(); i++ {
		field := objType.Field(i)
		fieldVal := objVal.Field(i)
		encrypt := field.Tag.Get("encrypt")
		if encrypt == "true" {
			if fieldVal.Kind() == reflect.Ptr && !fieldVal.IsNil() {
				underlyingVal := fieldVal.Elem()
				createdVal, ok := underlyingVal.Interface().(string)
				if !ok {
					return fmt.Errorf("%s field tagged for encryption but not a string value", field.Name)
				}
				encryptedVal, err := encryption.Encrypt(encryptionKey, createdVal)
				if err != nil {
					return fmt.Errorf("failed to encrypt %s for storage: %w", field.Name, err)
				}
				// use gorm to get column name from field name
				ns := schema.NamingStrategy{}
				columnName := ns.ColumnName("", field.Name)
				tx.Statement.SetColumn(columnName, encryptedVal)
			}
		}
	}

	return nil
}

// BeforeUpdate validates that no immutable fields are attempting to be changed
// before updates are persisted.
func (k *KubernetesRuntimeInstance) BeforeUpdate(tx *gorm.DB) error {
	// ensure runtime location is not changed
	if tx.Statement.Changed("Location") {
		msg := fmt.Sprintf("kubernetes runtime instances cannot be moved - location %s is immutable", *k.Location)
		return &KubernetesRuntimeInstanceValidationErr{msg}
	}

	// encrypt sensitive values
	var encryptionKey = os.Getenv("ENCRYPTION_KEY")
	if encryptionKey == "" {
		return errors.New("environment variable ENCRYPTION_KEY is not set")
	}
	updatedObj := tx.Statement.Dest.(KubernetesRuntimeInstance)
	objVal := reflect.ValueOf(&updatedObj).Elem()
	objType := objVal.Type()
	for i := 0; i < objType.NumField(); i++ {
		field := objType.Field(i)
		fieldVal := objVal.Field(i)
		encrypt := field.Tag.Get("encrypt")
		if encrypt == "true" && tx.Statement.Changed(field.Name) {
			if fieldVal.Kind() == reflect.Ptr && !fieldVal.IsNil() {
				underlyingVal := fieldVal.Elem()
				updatedVal, ok := underlyingVal.Interface().(string)
				if !ok {
					return fmt.Errorf("%s field tagged for encryption but not a string value", field.Name)
				}
				encryptedVal, err := encryption.Encrypt(encryptionKey, updatedVal)
				if err != nil {
					return fmt.Errorf("failed to encrypt %s for storage: %w", field.Name, err)
				}
				// use gorm to get column name from field name
				ns := schema.NamingStrategy{}
				columnName := ns.ColumnName("", field.Name)
				tx.Statement.SetColumn(columnName, encryptedVal)
			}
		}
	}

	return nil
}

// BeforeDelete validates a delete request on a kubernetes runtime instance
// deletion to ensure deletion is possible.
func (k *KubernetesRuntimeInstance) BeforeDelete(tx *gorm.DB) error {
	// validate that no workloads exist or that ForceDelete is true
	var workloadInstances []WorkloadInstance
	if result := tx.Where(&WorkloadInstance{KubernetesRuntimeInstanceID: k.ID}).Find(&workloadInstances); result.Error != nil {
		msg := fmt.Sprintf("failed to query workload instances for kubernetes runtime instance %s", *k.Name)
		return &KubernetesRuntimeInstanceValidationErr{msg}
	}

	if len(workloadInstances) > 0 {
		msg := fmt.Sprintf(
			"kubernetes runtime instance %s cannot be deleted until workloads are removed",
			*k.Name,
		)
		return &KubernetesRuntimeInstanceValidationErr{msg}
	}

	return nil
}
