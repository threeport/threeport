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
	if a.EncryptedAccessKeyID != nil {
		encryptedVal, err := encryption.Encrypt(encryptionKey, *a.EncryptedAccessKeyID)
		if err != nil {
			return fmt.Errorf("failed to encrypt AWS access key ID for storage: %w", err)
		}
		a.EncryptedAccessKeyID = &encryptedVal
	}
	if a.EncryptedSecretAccessKey != nil {
		encryptedVal, err := encryption.Encrypt(encryptionKey, *a.EncryptedSecretAccessKey)
		if err != nil {
			return fmt.Errorf("failed to encrypt AWS account secret access key for storage: %w", err)
		}
		a.EncryptedSecretAccessKey = &encryptedVal
	}

	return nil
}
