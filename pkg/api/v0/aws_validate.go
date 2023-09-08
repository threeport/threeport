package v0

import (
	"errors"
	"fmt"
	"os"
	"reflect"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"github.com/threeport/threeport/pkg/encryption/v0"
)

// SupportedAwsRelationalDatabaseEngines returns the supported database engines
// for AWS RDS.
func SupportedAwsRelationalDatabaseEngines() []string {
	return []string{
		"mariadb",
		"mysql",
		"postgres",
	}
}

// SupportedAwsRelationalDatabaseEngineVersions returns the valid versions for a
// given database engine.
func SupportedAwsRelationalDatabaseEngineVersions(engine string) []string {
	var supportedVersions []string
	switch engine {
	case "mariadb":
		supportedVersions = []string{
			"10.3",
			"10.4",
			"10.5",
			"10.6",
			"10.11",
		}
	case "mysql":
		supportedVersions = []string{
			"5.7",
			"8.0",
		}
	case "postgres":
		supportedVersions = []string{
			"11.16",
			"11.17",
			"11.18",
			"11.19",
			"11.20",
			"11.21",
			"12.11",
			"12.12",
			"12.13",
			"12.14",
			"12.15",
			"12.16",
			"13.7",
			"13.8",
			"13.9",
			"13.10",
			"13.11",
			"13.12",
			"14.3",
			"14.4",
			"14.5",
			"14.6",
			"14.7",
			"14.8",
			"14.9",
			"15.2",
			"15.3",
			"15.4",
		}
	}

	return supportedVersions
}

// BeforeCreate validates an AWS Relational Database Definition before
// persisting to the database.
func (a *AwsRelationalDatabaseDefinition) BeforeCreate(tx *gorm.DB) error {
	// validate databse engine is supported
	supportedEngines := SupportedAwsRelationalDatabaseEngines()
	engineValid := false
	for _, engine := range supportedEngines {
		if *a.Engine == engine {
			engineValid = true
			break
		}
	}
	if !engineValid {
		return errors.New(fmt.Sprintf(
			"%s engine is not supported, valid engines: %s",
			*a.Engine,
			supportedEngines,
		))
	}

	// validate database engine version
	supportedVersions := SupportedAwsRelationalDatabaseEngineVersions(*a.Engine)
	versionValid := false
	for _, version := range supportedVersions {
		if *a.EngineVersion == version {
			versionValid = true
			break
		}
	}
	if !versionValid {
		return errors.New(fmt.Sprintf(
			"%s version is not support for engine %s, valid versions: %s",
			*a.EngineVersion,
			*a.Engine,
			supportedVersions,
		))
	}

	return nil
}

// BeforeCreate validates a AWS Account before persisting to the
// database.
func (a *AwsAccount) BeforeCreate(tx *gorm.DB) error {
	// encrypt sensitive values
	var encryptionKey = os.Getenv("ENCRYPTION_KEY")
	if encryptionKey == "" {
		return errors.New("environment variable ENCRYPTION_KEY is not set")
	}
	createdObj := *a
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
func (a *AwsAccount) BeforeUpdate(tx *gorm.DB) error {
	// encrypt sensitive values
	var encryptionKey = os.Getenv("ENCRYPTION_KEY")
	if encryptionKey == "" {
		return errors.New("environment variable ENCRYPTION_KEY is not set")
	}
	updatedObj := tx.Statement.Dest.(AwsAccount)
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
