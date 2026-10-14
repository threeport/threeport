package v0

import (
	"strings"
	"time"

	"gorm.io/gorm"

	notifications "github.com/threeport/threeport/pkg/notifications/v0"
)

// CoreApiNamespace is the api namespace used for core threeport types
// in the fully qualified type form "<api-namespace>/<version>.<TypeName>" and in any
// other context that needs a stable identifier for core.
const CoreApiNamespace = "threeport.io"

// CoreModuleName is the Name value stored on the core ModuleApi row.
const CoreModuleName = "threeport-core-api"

// ReconciledThreeportApiObject is the interface each reconciled object in the
// Threeport API must satisfy for compatibility with the controlllers.
type ReconciledThreeportApiObject interface {
	NotificationPayload(
		operation notifications.NotificationOperation,
		requeue bool,
		creationTime int64,
	) (*[]byte, error)
	DecodeNotifObject(object interface{}) error
	GetId() uint
	GetType() string
	GetFullyQualifiedType() string
	GetVersion() string
	ScheduledForDeletion() *time.Time
}

// AlreadyReconciled reports whether obj embeds Reconciliation and that field is already true.
func AlreadyReconciled(obj any) bool {
	marker, ok := obj.(interface{ AlreadyReconciled() bool })
	return ok && marker.AlreadyReconciled()
}

// FullyQualifiedTypeProvider is implemented by every API object. The
// returned string is "<api-namespace>/<version>.<TypeName>" - core
// types use "threeport.io" as the namespace, modules use their
// configured ApiNamespace. Used as the identity string in
// AttachedObjectReference rows so the table is unambiguous across
// modules.
type FullyQualifiedTypeProvider interface {
	GetFullyQualifiedType() string
}

// NewCleanSession returns a session on the existing transaction that
// does not inherit clauses (WHERE filters, model targets, etc.) from
// the parent statement. Use this from GORM hooks when issuing a query
// that is unrelated to the operation that triggered the hook - without
// it the surrounding statement's clauses would silently apply to the
// new query.
func NewCleanSession(tx *gorm.DB) *gorm.DB {
	return tx.Session(&gorm.Session{NewDB: true})
}

// ParseQualifiedType splits "<api-namespace>/<version>.<TypeName>" into
// its three parts. Returns ok=false for malformed inputs (every API
// type stores its fully qualified type in this shape - core types use the
// "threeport.io" namespace, modules use their own).
//
// For "example.com/v0.Widget":
//
//	namespace = "example.com", version = "v0", typeName = "Widget", ok = true
func ParseQualifiedType(objectType string) (namespace, version, typeName string, ok bool) {
	// find the slash that separates the namespace from the rest.
	// for "example.com/v0.Widget" this is at index 11
	slashIdx := strings.Index(objectType, "/")

	// reject malformed inputs: no slash, slash at position 0 ("/foo"),
	// or slash as the last char ("foo/"). either side of the slash
	// must be non-empty
	if slashIdx < 1 || slashIdx == len(objectType)-1 {
		return "", "", "", false
	}

	// everything before the slash is the api namespace.
	// for "example.com/v0.Widget": namespace = "example.com"
	namespace = objectType[:slashIdx]

	// everything after the slash is "<version>.<TypeName>".
	// for "example.com/v0.Widget": rest = "v0.Widget"
	rest := objectType[slashIdx+1:]

	// find the dot that separates the version from the type name.
	// for rest = "v0.Widget" this is at index 2
	dotIdx := strings.Index(rest, ".")

	// reject malformed inputs: no dot, dot at position 0 (".Widget"),
	// or dot as the last char ("v0."). either side of the dot must
	// be non-empty
	if dotIdx < 1 || dotIdx == len(rest)-1 {
		return "", "", "", false
	}

	// split rest into version and type name on the dot.
	// for "v0.Widget": version = "v0", typeName = "Widget"
	return namespace, rest[:dotIdx], rest[dotIdx+1:], true
}
