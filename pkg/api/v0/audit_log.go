package v0

import (
	"fmt"

	"gorm.io/gorm"

	lib "github.com/threeport/threeport/pkg/api/lib/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// ErrMsgAuditLogImmutable is the error an update that changes an EKS, GKE or
// OKE definition's AuditLog returns.
const ErrMsgAuditLogImmutable = "AuditLog can't be changed after creation: a cluster's audit log is set when the cluster is created"

// validateAuditLogUpdate refuses an update that changes an EKS, GKE or OKE
// definition's AuditLog.  The audit log is set on a cluster when it is
// created, so a change would leave the definition saying something about its
// clusters that isn't so.  An unset AuditLog is false, so a replace from a
// client that doesn't send the field changes nothing on a definition without
// an audit log.  auditLog reads the field from a definition.
func validateAuditLogUpdate(tx *gorm.DB, auditLog func(any) (*bool, bool)) error {
	incoming, ok := auditLog(lib.IncomingValues(tx))
	if !ok {
		return fmt.Errorf("cannot validate AuditLog: update carries %T", lib.IncomingValues(tx))
	}
	// a PATCH without the field leaves it as it is
	if incoming == nil && !lib.IsFullReplace(tx) {
		return nil
	}

	current, err := lib.LockObjectInDB(tx)
	if err != nil {
		return err
	}
	stored, ok := auditLog(current)
	if !ok {
		return fmt.Errorf("cannot validate AuditLog: loaded %T", current)
	}
	if util.Deref(incoming) != util.Deref(stored) {
		return util.NewBadRequestError(ErrMsgAuditLogImmutable)
	}

	return nil
}
