package v0

import (
	"encoding/json"
	"fmt"
	"net/netip"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	lib "github.com/threeport/threeport/pkg/api/lib/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// The most entries each provider admits in the allowlist on a cluster's
// Kubernetes API.
const (
	// EKS: "Public endpoint access CIDR ranges per cluster", which AWS does
	// not raise.
	EksAllowedCidrsLimit = 40

	// GKE: authorized networks on a cluster whose control plane has a
	// public endpoint.
	GkePublicAllowedCidrsLimit = 50

	// GKE: authorized networks on a cluster whose control plane has only a
	// private endpoint.
	GkePrivateAllowedCidrsLimit = 100

	// OKE: security rules in the network security group on the cluster's
	// endpoint, one per CIDR block.
	OkeAllowedCidrsLimit = 120
)

// ErrMsgPrivateApiNotSupported is the rejection of PrivateApi: true on a
// provider that cannot make a Kubernetes API private yet.
const ErrMsgPrivateApiNotSupported = "PrivateApi is supported only on GKE for now: Threeport reaches a private GKE cluster through its DNS endpoint, while a private EKS or OKE cluster needs a network path from the control plane that Threeport does not set up yet - restrict the API with AllowedCidrs instead"

// ErrMsgKubernetesApiAccessImmutable is the rejection of an update that
// changes PrivateApi or AllowedCidrs.
const ErrMsgKubernetesApiAccessImmutable = "PrivateApi and AllowedCidrs cannot be changed after creation: the change would not be applied to the cluster"

// AllowedCidrsLimit returns how many CIDR blocks infraProvider admits in the
// allowlist on a cluster's Kubernetes API, and false for a provider whose
// Kubernetes API Threeport cannot restrict.
func AllowedCidrsLimit(infraProvider string, privateApi bool) (int, bool) {
	switch infraProvider {
	case KubernetesRuntimeInfraProviderEKS:
		return EksAllowedCidrsLimit, true
	case KubernetesRuntimeInfraProviderGKE:
		if privateApi {
			return GkePrivateAllowedCidrsLimit, true
		}
		return GkePublicAllowedCidrsLimit, true
	case KubernetesRuntimeInfraProviderOKE:
		return OkeAllowedCidrsLimit, true
	}

	return 0, false
}

// ParseCidrs reads field, a JSON array of IPv4 CIDR blocks such as
// ["203.0.113.0/24"].  Each block must be written as its network address,
// and none may repeat.  A field that is unset, null or an empty array holds
// no blocks.
func ParseCidrs(field string, raw *datatypes.JSON) ([]string, error) {
	if raw == nil || len(*raw) == 0 || string(*raw) == "null" {
		return nil, nil
	}

	var cidrs []string
	if err := json.Unmarshal(*raw, &cidrs); err != nil {
		return nil, fmt.Errorf(
			"%s must be a JSON array of CIDR blocks such as [\"203.0.113.0/24\"]: %w",
			field, err,
		)
	}

	seen := make(map[string]bool, len(cidrs))
	for _, cidr := range cidrs {
		prefix, err := netip.ParsePrefix(cidr)
		if err != nil {
			return nil, fmt.Errorf("%s entry %q is not a CIDR block: %w", field, cidr, err)
		}
		if !prefix.Addr().Is4() {
			return nil, fmt.Errorf("%s entry %q is not an IPv4 CIDR block", field, cidr)
		}
		if prefix.Masked() != prefix {
			return nil, fmt.Errorf(
				"%s entry %q has host bits set, write it as %q",
				field, cidr, prefix.Masked().String(),
			)
		}
		if seen[cidr] {
			return nil, fmt.Errorf("%s entry %q is listed more than once", field, cidr)
		}
		seen[cidr] = true
	}

	return cidrs, nil
}

// validateKubernetesApiAccess checks PrivateApi and AllowedCidrs as one
// runtime on infraProvider would hold them.
func validateKubernetesApiAccess(infraProvider string, privateApi *bool, allowedCidrs *datatypes.JSON) error {
	cidrs, err := ParseCidrs("AllowedCidrs", allowedCidrs)
	if err != nil {
		return util.NewBadRequestError(err.Error())
	}

	private := privateApi != nil && *privateApi
	limit, restrictable := AllowedCidrsLimit(infraProvider, private)
	if !restrictable {
		if private || len(cidrs) > 0 {
			return util.NewBadRequestError(fmt.Sprintf(
				"PrivateApi and AllowedCidrs are not supported on %s runtimes",
				infraProvider,
			))
		}
		return nil
	}

	// only the GKE program makes the endpoint private, so accepting
	// PrivateApi elsewhere would leave a cluster public that was asked to
	// be private
	if private && infraProvider != KubernetesRuntimeInfraProviderGKE {
		return util.NewBadRequestError(ErrMsgPrivateApiNotSupported)
	}

	if len(cidrs) > limit {
		return util.NewBadRequestError(fmt.Sprintf(
			"AllowedCidrs has %d entries and %s admits at most %d",
			len(cidrs), infraProvider, limit,
		))
	}

	return nil
}

// validateKubernetesApiAccessUpdate checks an update that sends PrivateApi
// or AllowedCidrs.  The pair the row will hold once the update lands is
// validated, then refused if it differs from the stored one: nothing applies a
// change to a runtime that exists, so accepting one would record a restriction
// the cluster does not have.  Values that mean the same, such as an unset list
// and an empty one or the same blocks in another order, are not a change.
// fields returns the pair an object of the hook's type holds, and false for
// any other type.
//
// A PUT writes every column, so the pair it carries is the pair the row will
// hold.  A PATCH writes only the fields it carries, over the row as it is when
// the write lands.  That isn't the receiver: the handler loaded the receiver
// before the transaction began, and another update may have changed the row
// since.  The row is read again inside the transaction and locked instead, so
// a concurrent update of the pair waits for this one and is then checked
// against its result.
func validateKubernetesApiAccessUpdate(
	tx *gorm.DB,
	infraProvider string,
	fields func(any) (*bool, *datatypes.JSON, bool),
) error {
	incomingPrivateApi, incomingAllowedCidrs, ok := fields(lib.IncomingValues(tx))
	if !ok {
		return fmt.Errorf(
			"cannot validate Kubernetes API access: update carries %T",
			lib.IncomingValues(tx),
		)
	}

	var stored any
	var privateApi *bool
	var allowedCidrs *datatypes.JSON
	if lib.IsFullReplace(tx) {
		privateApiChanged, err := lib.IsFieldChanged(tx, "PrivateApi")
		if err != nil {
			return err
		}
		allowedCidrsChanged, err := lib.IsFieldChanged(tx, "AllowedCidrs")
		if err != nil {
			return err
		}
		if !privateApiChanged && !allowedCidrsChanged {
			return nil
		}

		// a replace's receiver is the incoming object, so the stored row is
		// read back
		if stored, err = lib.LoadObjectFromDB(tx); err != nil {
			return err
		}
		privateApi, allowedCidrs = incomingPrivateApi, incomingAllowedCidrs
	} else {
		// a PATCH leaves the fields it doesn't carry as they are
		if incomingPrivateApi == nil && incomingAllowedCidrs == nil {
			return nil
		}

		var err error
		if stored, err = lib.LockObjectInDB(tx); err != nil {
			return err
		}
	}
	storedPrivateApi, storedAllowedCidrs, ok := fields(stored)
	if !ok {
		return fmt.Errorf("cannot validate Kubernetes API access: loaded %T", stored)
	}
	if !lib.IsFullReplace(tx) {
		privateApi, allowedCidrs = storedPrivateApi, storedAllowedCidrs
		if incomingPrivateApi != nil {
			privateApi = incomingPrivateApi
		}
		if incomingAllowedCidrs != nil {
			allowedCidrs = incomingAllowedCidrs
		}
	}

	if err := validateKubernetesApiAccess(infraProvider, privateApi, allowedCidrs); err != nil {
		return err
	}

	if !sameKubernetesApiAccess(storedPrivateApi, storedAllowedCidrs, privateApi, allowedCidrs) {
		return util.NewBadRequestError(ErrMsgKubernetesApiAccessImmutable)
	}

	return nil
}

// sameKubernetesApiAccess reports whether two pairs of PrivateApi and
// AllowedCidrs restrict the Kubernetes API in the same way.  A list that
// cannot be read is taken to differ.
func sameKubernetesApiAccess(privateApiA *bool, allowedCidrsA *datatypes.JSON, privateApiB *bool, allowedCidrsB *datatypes.JSON) bool {
	if (privateApiA != nil && *privateApiA) != (privateApiB != nil && *privateApiB) {
		return false
	}

	cidrsA, errA := ParseCidrs("AllowedCidrs", allowedCidrsA)
	cidrsB, errB := ParseCidrs("AllowedCidrs", allowedCidrsB)
	if errA != nil || errB != nil || len(cidrsA) != len(cidrsB) {
		return false
	}
	inA := make(map[string]bool, len(cidrsA))
	for _, cidr := range cidrsA {
		inA[cidr] = true
	}
	for _, cidr := range cidrsB {
		if !inA[cidr] {
			return false
		}
	}

	return true
}

// validateCidrsUpdate checks field on an update that changes it.
func validateCidrsUpdate(tx *gorm.DB, field string, incoming func(any) (*datatypes.JSON, bool)) error {
	changed, err := lib.IsFieldChanged(tx, field)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}

	cidrs, ok := incoming(lib.IncomingValues(tx))
	if !ok {
		return fmt.Errorf("cannot validate %s: update carries %T", field, lib.IncomingValues(tx))
	}
	if _, err := ParseCidrs(field, cidrs); err != nil {
		return util.NewBadRequestError(err.Error())
	}

	return nil
}
