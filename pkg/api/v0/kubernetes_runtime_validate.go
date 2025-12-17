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
	util "github.com/threeport/threeport/pkg/util/v0"
)

// KubernetesRuntimeInfraProvider indicates which infrastructure provider is being
// used to run the kubernetes cluster for the threeport control plane.
type KubernetesRuntimeInfraProvider string

const (
	KubernetesRuntimeInfraProviderKind = "kind"
	KubernetesRuntimeInfraProviderEKS  = "eks"
	KubernetesRuntimeInfraProviderOKE  = "oke"
	KubernetesRuntimeInfraProviderGKE  = "gke"
)

// SupportedInfraProviders returns all supported infra providers.
func SupportedInfraProviders() []KubernetesRuntimeInfraProvider {
	return []KubernetesRuntimeInfraProvider{
		KubernetesRuntimeInfraProviderKind,
		KubernetesRuntimeInfraProviderEKS,
		KubernetesRuntimeInfraProviderOKE,
		KubernetesRuntimeInfraProviderGKE,
	}
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
		return util.NewBadRequestError(
			fmt.Sprintf(
				"%s provider is not valid, valid providers: %s",
				*k.InfraProvider,
				infraProviders,
			),
		)
	}

	return nil
}

// BeforeCreate validates a KubernetesRuntimeInstance before persisting to the
// database.
func (k *KubernetesRuntimeInstance) BeforeCreate(tx *gorm.DB) error {
	// validate location
	if !mapping.ValidLocation(*k.Location) {
		return util.NewBadRequestError(
			fmt.Sprintf(
				"location %s is not supported for a kubernetes runtime instance",
				*k.Location,
			),
		)
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

		// skip nil fields
		if !util.IsNonNilPtr(fieldVal) {
			continue
		}

		encrypt := field.Tag.Get("encrypt")
		if encrypt == "true" {

			underlyingValue, err := util.GetPtrValue(fieldVal)
			if err != nil {
				return fmt.Errorf("failed to get string value for %s: %w", field.Name, err)
			}
			encryptedVal, err := encryption.Encrypt(encryptionKey, underlyingValue)
			if err != nil {
				return fmt.Errorf("failed to encrypt %s for storage: %w", field.Name, err)
			}
			// use gorm to get column name from field name
			ns := schema.NamingStrategy{}
			columnName := ns.ColumnName("", field.Name)
			tx.Statement.SetColumn(columnName, encryptedVal)
		}
	}

	return nil
}

// BeforeUpdate validates that no immutable fields are being changed
// before updates are persisted.
func (k *KubernetesRuntimeDefinition) BeforeUpdate(tx *gorm.DB) error {
	// ensure infra provider is not changed
	if tx.Statement.Changed("InfraProvider") {
		return util.NewBadRequestError(
			"kubernetes runtime definition infra provider cannot be changed after creation",
		)
	}

	// ensure high availability is not changed
	if tx.Statement.Changed("HighAvailability") {
		return util.NewBadRequestError(
			"kubernetes runtime definition high availability cannot be changed after creation",
		)
	}

	return nil
}

// BeforeUpdate validates that no immutable fields are being changed
// before updates are persisted.
func (k *KubernetesRuntimeInstance) BeforeUpdate(tx *gorm.DB) error {
	// ensure runtime location is not changed
	if tx.Statement.Changed("Location") {
		return util.NewBadRequestError(
			fmt.Sprintf(
				"kubernetes runtime instances cannot be moved - location %s is immutable",
				*k.Location,
			),
		)
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

		// skip nil fields
		if !util.IsNonNilPtr(fieldVal) {
			continue
		}

		encrypt := field.Tag.Get("encrypt")
		if encrypt == "true" && tx.Statement.Changed(field.Name) {
			underlyingValue, err := util.GetPtrValue(fieldVal)
			if err != nil {
				return fmt.Errorf("failed to get string value for %s: %w", field.Name, err)
			}
			encryptedVal, err := encryption.Encrypt(encryptionKey, underlyingValue)
			if err != nil {
				return fmt.Errorf("failed to encrypt %s for storage: %w", field.Name, err)
			}
			// use gorm to get column name from field name
			ns := schema.NamingStrategy{}
			columnName := ns.ColumnName("", field.Name)
			tx.Statement.SetColumn(columnName, encryptedVal)
		}
	}

	return nil
}

// BeforeDelete validates a delete request on a kubernetes runtime instance
// deletion to ensure deletion is possible.
func (k *KubernetesRuntimeInstance) BeforeDelete(tx *gorm.DB) error {
	// validate that no workloads exist or that ForceDelete is true
	var workloadInstances []WorkloadInstance
	if result := tx.Where(
		&WorkloadInstance{KubernetesRuntimeInstanceID: k.ID},
	).Find(&workloadInstances); result.Error != nil {
		return fmt.Errorf(
			"failed to query workload instances for kubernetes runtime instance %s",
			*k.Name,
		)
	}

	if len(workloadInstances) > 0 {
		return util.NewBadRequestError(
			fmt.Sprintf(
				"kubernetes runtime instance %s cannot be deleted until workloads are removed",
				*k.Name,
			),
		)
	}

	return nil
}
