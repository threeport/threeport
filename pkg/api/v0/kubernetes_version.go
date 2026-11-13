package v0

import (
	"fmt"
	"slices"
	"strings"

	"gorm.io/gorm"

	lib "github.com/threeport/threeport/pkg/api/lib/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// SupportedKubernetesVersions are the Kubernetes minor versions Threeport
// creates EKS, GKE and OKE clusters on, oldest first.  Each is offered for new
// clusters by all three: in standard support on EKS, in the REGULAR and STABLE
// release channels on GKE, and as a production release on OKE.
var SupportedKubernetesVersions = []string{"1.35", "1.36"}

// DefaultKubernetesVersion is the Kubernetes minor version of a cluster whose
// definition sets none.
const DefaultKubernetesVersion = "1.36"

// ErrMsgKubernetesVersionImmutable is the error a change to a definition's
// KubernetesVersion returns.  Threeport can't upgrade a cluster yet, so a
// changed version would record one its clusters don't run.
const ErrMsgKubernetesVersionImmutable = "KubernetesVersion cannot be changed after creation: upgrading a cluster is not supported yet"

// KubernetesVersionOrDefault returns version, or DefaultKubernetesVersion when
// it is unset.
func KubernetesVersionOrDefault(version *string) string {
	if version == nil || *version == "" {
		return DefaultKubernetesVersion
	}

	return *version
}

// ValidateKubernetesVersion checks a definition's KubernetesVersion: unset, or
// one of SupportedKubernetesVersions on a provider whose version Threeport
// sets.  A kind cluster's version comes with its node image, so it takes none.
func ValidateKubernetesVersion(infraProvider string, version *string) error {
	if version == nil {
		return nil
	}
	if infraProvider == KubernetesRuntimeInfraProviderKind {
		return fmt.Errorf("KubernetesVersion cannot be set on a kind cluster, whose version comes with its node image")
	}
	if !slices.Contains(SupportedKubernetesVersions, *version) {
		return fmt.Errorf(
			"KubernetesVersion %q is not supported: use one of %s, written as a minor version such as %q",
			*version, strings.Join(SupportedKubernetesVersions, ", "), DefaultKubernetesVersion,
		)
	}

	return nil
}

// validateKubernetesVersionCreate checks the KubernetesVersion of a definition
// being created.
func validateKubernetesVersionCreate(infraProvider string, version *string) error {
	if err := ValidateKubernetesVersion(infraProvider, version); err != nil {
		return util.NewBadRequestError(err.Error())
	}

	return nil
}

// validateKubernetesVersionUpdate refuses an update that changes a
// definition's KubernetesVersion.
func validateKubernetesVersionUpdate(tx *gorm.DB) error {
	changed, err := lib.IsFieldChanged(tx, "KubernetesVersion")
	if err != nil {
		return err
	}
	if changed {
		return util.NewBadRequestError(ErrMsgKubernetesVersionImmutable)
	}

	return nil
}
