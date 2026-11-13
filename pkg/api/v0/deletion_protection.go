package v0

import (
	"errors"
	"fmt"

	"gorm.io/gorm"

	lib "github.com/threeport/threeport/pkg/api/lib/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// DeleteRefuser is implemented by an API object whose delete the API server
// refuses before scheduling it.  The generated delete handler calls
// RefuseDelete on an object that implements it, and answers 409 when it
// returns a *DeleteRefusedError and 500 when it returns any other error.  A
// delete already scheduled isn't asked again, so one the object's controller
// is carrying out is finished.
type DeleteRefuser interface {
	RefuseDelete(db *gorm.DB) error
}

// DeleteRefusedError is the reason a DeleteRefuser refuses its delete.
type DeleteRefusedError struct {
	Err error
}

func (e *DeleteRefusedError) Error() string { return e.Err.Error() }

func (e *DeleteRefusedError) Unwrap() error { return e.Err }

// CheckDeleteRefused returns why object's delete is refused, if it is a
// DeleteRefuser that refuses it, and nil otherwise.
func CheckDeleteRefused(db *gorm.DB, object any) error {
	refuser, ok := object.(DeleteRefuser)
	if !ok {
		return nil
	}

	return refuser.RefuseDelete(db)
}

// DeletionProtectedError reports that a cluster can't be deleted because the
// EKS, GKE or OKE definition it was created from has DeletionProtection set.
type DeletionProtectedError struct {
	// The definition's object type as tptctl names it, such as
	// "aws-eks-kubernetes-runtime-definition".
	DefinitionType string

	// The definition's name.
	DefinitionName string
}

func (e *DeletionProtectedError) Error() string {
	return fmt.Sprintf(
		"%[1]s %[2]s has DeletionProtection set, so the clusters of its runtime instances can't be deleted. "+
			"To delete one, set DeletionProtection: false on the definition first: "+
			"write its fields, shown by 'tptctl get %[1]s --name %[2]s -o yaml', to a config file with DeletionProtection: false, "+
			"and run 'tptctl replace %[1]s --config <file> --name %[2]s'. "+
			"That lets every runtime instance of the definition be deleted",
		e.DefinitionType, e.DefinitionName,
	)
}

// ProviderDefinitionDeletionProtection returns the error a delete of one of
// definition's clusters is refused with: a *DeletionProtectedError when the
// definition has DeletionProtection set, and nil otherwise.  definition is an
// AwsEksKubernetesRuntimeDefinition, a GcpGkeKubernetesRuntimeDefinition or
// an OciOkeKubernetesRuntimeDefinition; anything else protects nothing.
func ProviderDefinitionDeletionProtection(definition any) error {
	var definitionType string
	var name *string
	var protection *bool
	switch d := definition.(type) {
	case *AwsEksKubernetesRuntimeDefinition:
		definitionType, name, protection = "aws-eks-kubernetes-runtime-definition", d.Name, d.DeletionProtection
	case *GcpGkeKubernetesRuntimeDefinition:
		definitionType, name, protection = "gcp-gke-kubernetes-runtime-definition", d.Name, d.DeletionProtection
	case *OciOkeKubernetesRuntimeDefinition:
		definitionType, name, protection = "oci-oke-kubernetes-runtime-definition", d.Name, d.DeletionProtection
	default:
		return nil
	}
	if protection == nil || !*protection {
		return nil
	}
	definitionName := ""
	if name != nil {
		definitionName = *name
	}

	return &DeletionProtectedError{DefinitionType: definitionType, DefinitionName: definitionName}
}

// refuseProtectedDelete refuses a delete while the provider definition found
// by query has DeletionProtection set.  A definition that isn't found
// protects nothing.
func refuseProtectedDelete(definition any, query func(dest any) *gorm.DB) error {
	if result := query(definition); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil
		}
		return fmt.Errorf("failed to read the definition's deletion protection: %w", result.Error)
	}
	if err := ProviderDefinitionDeletionProtection(definition); err != nil {
		return &DeleteRefusedError{Err: err}
	}

	return nil
}

// RefuseDelete refuses to delete a runtime instance while the EKS, GKE or OKE
// definition married to its definition has DeletionProtection set.
func (k *KubernetesRuntimeInstance) RefuseDelete(db *gorm.DB) error {
	if k.KubernetesRuntimeDefinitionID == nil {
		return nil
	}
	var definition KubernetesRuntimeDefinition
	if result := db.Session(&gorm.Session{NewDB: true}).First(&definition, *k.KubernetesRuntimeDefinitionID); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil
		}
		return fmt.Errorf("failed to read the runtime instance's definition: %w", result.Error)
	}
	if definition.InfraProvider == nil {
		return nil
	}

	var providerDefinition any
	switch *definition.InfraProvider {
	case KubernetesRuntimeInfraProviderEKS:
		providerDefinition = &AwsEksKubernetesRuntimeDefinition{}
	case KubernetesRuntimeInfraProviderGKE:
		providerDefinition = &GcpGkeKubernetesRuntimeDefinition{}
	case KubernetesRuntimeInfraProviderOKE:
		providerDefinition = &OciOkeKubernetesRuntimeDefinition{}
	default:
		return nil
	}

	return refuseProtectedDelete(providerDefinition, func(dest any) *gorm.DB {
		return db.Session(&gorm.Session{NewDB: true}).Where("kubernetes_runtime_definition_id = ?", *definition.ID).First(dest)
	})
}

// RefuseDelete refuses to delete an EKS runtime instance while its definition
// has DeletionProtection set.
func (a *AwsEksKubernetesRuntimeInstance) RefuseDelete(db *gorm.DB) error {
	if a.AwsEksKubernetesRuntimeDefinitionID == nil {
		return nil
	}

	return refuseProtectedDelete(&AwsEksKubernetesRuntimeDefinition{}, func(dest any) *gorm.DB {
		return db.Session(&gorm.Session{NewDB: true}).First(dest, *a.AwsEksKubernetesRuntimeDefinitionID)
	})
}

// RefuseDelete refuses to delete a GKE runtime instance while its definition
// has DeletionProtection set.
func (g *GcpGkeKubernetesRuntimeInstance) RefuseDelete(db *gorm.DB) error {
	if g.GcpGkeKubernetesRuntimeDefinitionID == nil {
		return nil
	}

	return refuseProtectedDelete(&GcpGkeKubernetesRuntimeDefinition{}, func(dest any) *gorm.DB {
		return db.Session(&gorm.Session{NewDB: true}).First(dest, *g.GcpGkeKubernetesRuntimeDefinitionID)
	})
}

// RefuseDelete refuses to delete an OKE runtime instance while its definition
// has DeletionProtection set.
func (o *OciOkeKubernetesRuntimeInstance) RefuseDelete(db *gorm.DB) error {
	if o.OciOkeKubernetesRuntimeDefinitionID == nil {
		return nil
	}

	return refuseProtectedDelete(&OciOkeKubernetesRuntimeDefinition{}, func(dest any) *gorm.DB {
		return db.Session(&gorm.Session{NewDB: true}).First(dest, *o.OciOkeKubernetesRuntimeDefinitionID)
	})
}

// ErrMsgDeletionProtectionTurnedOn is the error a definition with runtime
// instances is refused when an update turns its DeletionProtection on.
const ErrMsgDeletionProtectionTurnedOn = "DeletionProtection can't be turned on for a definition that has runtime instances: " +
	"a cluster is protected when it is created, so its existing clusters would stay unprotected. " +
	"Create the clusters to protect from a definition with DeletionProtection: true"

// validateDeletionProtectionUpdate refuses an update of an EKS, GKE or OKE
// definition that turns DeletionProtection on while the definition has
// runtime instances.  Protection is put on a cluster, in Pulumi and on EKS,
// when the cluster is created, so the definition would report its existing
// clusters protected while nothing protects them.  Turning it off is
// allowed, and so is turning it on for a definition with no instances yet.
// protection returns a definition's DeletionProtection and ID; instances is
// the model of its runtime instances, whose definitionColumn holds the ID.
func validateDeletionProtectionUpdate(
	tx *gorm.DB,
	protection func(any) (*bool, *uint, bool),
	instances any,
	definitionColumn string,
) error {
	incoming, _, ok := protection(lib.IncomingValues(tx))
	if !ok {
		return fmt.Errorf("cannot validate DeletionProtection: update carries %T", lib.IncomingValues(tx))
	}
	if incoming == nil || !*incoming {
		return nil
	}

	var stored any
	var err error
	if lib.IsFullReplace(tx) {
		stored, err = lib.LoadObjectFromDB(tx)
	} else {
		stored, err = lib.LockObjectInDB(tx)
	}
	if err != nil {
		return err
	}
	storedProtection, id, ok := protection(stored)
	if !ok {
		return fmt.Errorf("cannot validate DeletionProtection: loaded %T", stored)
	}
	if (storedProtection != nil && *storedProtection) || id == nil {
		return nil
	}

	var count int64
	if result := lib.NewCleanSession(tx).Model(instances).Where(definitionColumn+" = ?", *id).Count(&count); result.Error != nil {
		return fmt.Errorf("failed to count the definition's runtime instances: %w", result.Error)
	}
	if count > 0 {
		return util.NewBadRequestError(ErrMsgDeletionProtectionTurnedOn)
	}

	return nil
}
