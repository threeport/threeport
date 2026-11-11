package v0

import (
	"fmt"
	"slices"
	"strconv"
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

// ErrMsgKubernetesVersionImmutable is the error a change to the
// KubernetesVersion of a definition on a provider Threeport can't upgrade
// clusters on yet returns, since a changed version would record one its
// clusters don't run.
const ErrMsgKubernetesVersionImmutable = "KubernetesVersion cannot be changed after creation on this provider: upgrading its clusters is not supported yet"

// KubernetesVersionUpgradeProviders are the providers whose clusters
// Threeport upgrades when their definition's KubernetesVersion changes.
var KubernetesVersionUpgradeProviders = []string{
	KubernetesRuntimeInfraProviderEKS,
	KubernetesRuntimeInfraProviderGKE,
}

// parseKubernetesMinorVersion returns the major and minor numbers of a minor
// version such as "1.36".
func parseKubernetesMinorVersion(version string) (int, int, error) {
	major, minor, found := strings.Cut(version, ".")
	if !found {
		return 0, 0, fmt.Errorf("%q is not a Kubernetes minor version such as %q", version, DefaultKubernetesVersion)
	}
	majorNumber, majorErr := strconv.Atoi(major)
	minorNumber, minorErr := strconv.Atoi(minor)
	if majorErr != nil || minorErr != nil || majorNumber < 0 || minorNumber < 0 {
		return 0, 0, fmt.Errorf("%q is not a Kubernetes minor version such as %q", version, DefaultKubernetesVersion)
	}

	return majorNumber, minorNumber, nil
}

// CompareKubernetesVersions compares two minor versions such as "1.35" and
// "1.36" by number, returning -1, 0 or 1 as a is older than, the same as or
// newer than b.
func CompareKubernetesVersions(a, b string) (int, error) {
	aMajor, aMinor, err := parseKubernetesMinorVersion(a)
	if err != nil {
		return 0, err
	}
	bMajor, bMinor, err := parseKubernetesMinorVersion(b)
	if err != nil {
		return 0, err
	}
	if aMajor != bMajor {
		return compareInts(aMajor, bMajor), nil
	}

	return compareInts(aMinor, bMinor), nil
}

// compareInts returns -1, 0 or 1 as a is less than, equal to or greater than b.
func compareInts(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

// NextKubernetesVersion returns the minor version after version, such as
// "1.36" after "1.35": the one a cluster on version is upgraded to.
func NextKubernetesVersion(version string) (string, error) {
	major, minor, err := parseKubernetesMinorVersion(version)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%d.%d", major, minor+1), nil
}

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

// validateKubernetesVersionUpgrade checks an update that changes a
// definition's KubernetesVersion.  It is allowed on a provider Threeport
// upgrades clusters on, to a supported version no older than the one it
// replaces, since a cluster can't be downgraded.  version reads the field from
// the update's incoming values and from the stored definition.
func validateKubernetesVersionUpgrade(
	tx *gorm.DB,
	infraProvider string,
	version func(any) (*string, bool),
) error {
	changed, err := lib.IsFieldChanged(tx, "KubernetesVersion")
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	if !slices.Contains(KubernetesVersionUpgradeProviders, infraProvider) {
		return util.NewBadRequestError(ErrMsgKubernetesVersionImmutable)
	}

	incoming, ok := version(lib.IncomingValues(tx))
	if !ok {
		return fmt.Errorf("cannot validate KubernetesVersion: update carries %T", lib.IncomingValues(tx))
	}
	if incoming == nil {
		return util.NewBadRequestError("KubernetesVersion cannot be cleared: its clusters run the version it records")
	}
	if err := ValidateKubernetesVersion(infraProvider, incoming); err != nil {
		return util.NewBadRequestError(err.Error())
	}

	current, err := lib.LockObjectInDB(tx)
	if err != nil {
		return err
	}
	stored, ok := version(current)
	if !ok {
		return fmt.Errorf("cannot validate KubernetesVersion: loaded %T", current)
	}
	storedVersion := KubernetesVersionOrDefault(stored)
	comparison, err := CompareKubernetesVersions(*incoming, storedVersion)
	if err != nil {
		return err
	}
	if comparison < 0 {
		return util.NewBadRequestError(fmt.Sprintf(
			"KubernetesVersion %q is older than %q: a cluster can't be downgraded", *incoming, storedVersion,
		))
	}

	return nil
}

// ErrMsgKubernetesRuntimeVersionImmutable is the error a change to a
// kubernetes runtime definition's KubernetesVersion returns.  It records the
// version asked for when the definition was created; the version its clusters
// run, and are upgraded to, is on its EKS, GKE or OKE definition.
const ErrMsgKubernetesRuntimeVersionImmutable = "KubernetesVersion cannot be changed on a kubernetes runtime definition: change it on its EKS or GKE definition, which upgrades the clusters"

// validateKubernetesRuntimeVersionUpdate refuses an update that changes a
// kubernetes runtime definition's KubernetesVersion.
func validateKubernetesRuntimeVersionUpdate(tx *gorm.DB) error {
	changed, err := lib.IsFieldChanged(tx, "KubernetesVersion")
	if err != nil {
		return err
	}
	if changed {
		return util.NewBadRequestError(ErrMsgKubernetesRuntimeVersionImmutable)
	}

	return nil
}

// validateKubernetesVersionUpdate refuses an update that changes a
// definition's KubernetesVersion, on a provider Threeport can't upgrade
// clusters on yet.
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
