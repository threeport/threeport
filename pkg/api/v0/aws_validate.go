package v0

import (
	"errors"
	"fmt"
	"os"

	"github.com/threeport/threeport/pkg/encryption/v0"
	"gorm.io/gorm"
)

// BeforeCreate validates a KubernetesRuntimeInstance before persisting to the
// database.
func (a *AwsAccount) BeforeCreate(tx *gorm.DB) error {
	// encrypt sensitive values
	var encryptionKey = os.Getenv("ENCRYPTION_KEY")
	if encryptionKey == "" {
		return errors.New("environment variable ENCRYPTION_KEY is not set")
	}
	if a.AccessKeyID != nil {
		encryptedVal, err := encryption.Encrypt(encryptionKey, *a.AccessKeyID)
		if err != nil {
			return fmt.Errorf("failed to encrypt AWS access key ID for storage: %w", err)
		}
		a.AccessKeyID = &encryptedVal
	}
	if a.SecretAccessKey != nil {
		encryptedVal, err := encryption.Encrypt(encryptionKey, *a.SecretAccessKey)
		if err != nil {
			return fmt.Errorf("failed to encrypt AWS account secret access key for storage: %w", err)
		}
		a.SecretAccessKey = &encryptedVal
	}

	return nil
}

// BeforeUpdate validates that no immutable fields are attempting to be changed
// before updates are persisted.
func (a *AwsAccount) BeforeUpdate(tx *gorm.DB) error {
	// encrypt sensitive values
	var encryptionKey = os.Getenv("ENCRYPTION_KEY")
	if encryptionKey == "" {
		return errors.New("environment variable ENCRYPTION_KEY is not set")
	}
	updatedObj := tx.Statement.Dest.(AwsAccount)
	if tx.Statement.Changed("AccessKeyID") {
		encryptedVal, err := encryption.Encrypt(
			encryptionKey,
			*updatedObj.AccessKeyID,
		)
		if err != nil {
			return fmt.Errorf("failed to encrypt AWS access key ID for storage: %w", err)
		}
		tx.Statement.SetColumn("access_key_id", encryptedVal)
	}
	if tx.Statement.Changed("SecretAccessKey") {
		encryptedVal, err := encryption.Encrypt(
			encryptionKey,
			*updatedObj.SecretAccessKey,
		)
		if err != nil {
			return fmt.Errorf("failed to encrypt AWS account secret access key for storage: %w", err)
		}
		tx.Statement.SetColumn("secret_access_key", encryptedVal)
	}

	return nil
}
