package v0

import (
	"fmt"
	"slices"
	"strings"

	"gorm.io/gorm"

	lib "github.com/threeport/threeport/pkg/api/lib/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// EksSupportTypes are what can happen to an EKS cluster when its Kubernetes
// version leaves EKS standard support.  With STANDARD, AWS upgrades the
// cluster's control plane to the next version.  With EXTENDED, it stays on
// that version in extended support, at a higher hourly charge.
var EksSupportTypes = []string{"STANDARD", "EXTENDED"}

// DefaultEksSupportType is the support type of a cluster whose definition
// sets none.  EKS's own default is EXTENDED: STANDARD keeps a cluster off
// the extended support charge unless its definition asks for it.
const DefaultEksSupportType = "STANDARD"

// ErrMsgEksSupportTypeImmutable is the error a change to an EKS definition's
// support type returns.  Threeport doesn't change a cluster's upgrade policy
// after creating it, so a changed one would record a policy its clusters
// don't have.
const ErrMsgEksSupportTypeImmutable = "SupportType cannot be changed after creation: changing a cluster's upgrade policy is not supported yet"

// EksSupportTypeOrDefault returns supportType, or DefaultEksSupportType when
// it is unset.
func EksSupportTypeOrDefault(supportType *string) string {
	if supportType == nil || *supportType == "" {
		return DefaultEksSupportType
	}

	return *supportType
}

// ValidateEksSupportType checks an EKS definition's support type, which is
// optional: an unset one takes the default.
func ValidateEksSupportType(supportType *string) error {
	if supportType != nil && !slices.Contains(EksSupportTypes, *supportType) {
		return fmt.Errorf(
			"SupportType %q is not supported: use one of %s",
			*supportType, strings.Join(EksSupportTypes, ", "),
		)
	}

	return nil
}

// validateEksSupportTypeCreate checks the support type of an EKS definition
// being created.
func validateEksSupportTypeCreate(a *AwsEksKubernetesRuntimeDefinition) error {
	if err := ValidateEksSupportType(a.SupportType); err != nil {
		return util.NewBadRequestError(err.Error())
	}

	return nil
}

// validateEksSupportTypeUpdate refuses an update that changes an EKS
// definition's support type.
func validateEksSupportTypeUpdate(tx *gorm.DB) error {
	changed, err := lib.IsFieldChanged(tx, "SupportType")
	if err != nil {
		return err
	}
	if changed {
		return util.NewBadRequestError(ErrMsgEksSupportTypeImmutable)
	}

	return nil
}
