package v0

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/iancoleman/strcase"
	apilib "github.com/threeport/threeport/pkg/api/lib/v0"
	api "github.com/threeport/threeport/pkg/api/v0"
	client_v0 "github.com/threeport/threeport/pkg/client/v0"
	tp_errors "github.com/threeport/threeport/pkg/errors/v0"
	notifications "github.com/threeport/threeport/pkg/notifications/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

const (
	// Default event reasons for crud operations.
	ReasonCreateInProgress = "CreateInProgress"
	ReasonCreateSuccessful = "CreateSuccessful"
	ReasonCreateFailed     = "CreateFailed"

	ReasonUpdateInProgress = "UpdateInProgress"
	ReasonUpdateSuccessful = "UpdateSuccessful"
	ReasonUpdateFailed     = "UpdateFailed"

	ReasonDeleteInProgress = "DeleteInProgress"
	ReasonDeleteSuccessful = "DeleteSuccessful"
	ReasonDeleteFailed     = "DeleteFailed"

	// Default event types
	TypeNormal  = "Normal"
	TypeWarning = "Warning"
)

// EventRecorder records events to the backend.
type EventRecorder struct {

	// APIClient is the HTTP client used to make requests to the Threeport API.
	APIClient *http.Client

	// APIServer is the endpoint to reach Threeport REST API.
	// format: [protocol]://[hostname]:[port]
	APIServer string

	// Name of the controller that emitted this Event.
	ReportingController string
}

// RecordEvent posts an event about the given object. The api server
// dedups on insert against the unique index over the event's subject
// and content, so an emit repeating an event already on file bumps that
// row's Count instead of adding a row. fullyQualifiedObjectType must be
// the form returned by GetFullyQualifiedType.
func (r *EventRecorder) RecordEvent(
	event *api.Event,
	objectId uint,
	fullyQualifiedObjectType string,
) error {
	// stamp reporter, timestamps, and count on the in-memory event.
	// count is 1 on the wire; the api server's conflict handling
	// increments the stored count when the row is already on file.
	now := time.Now()
	event.ReportingController = &r.ReportingController
	event.EventTime = util.Ptr(now)
	event.LastObservedTime = util.Ptr(now)
	event.Count = util.Ptr(uint(1))
	if event.Note == nil {
		event.Note = util.Ptr("")
	}

	// carry the subject on the in-memory Event. ObjectType and ObjectID
	// are columns on the event row and part of the dedup index, so the
	// api server's beforeCreate validates both and the insert conflicts
	// against any row sharing this subject and content.
	event.ObjectType = util.Ptr(fullyQualifiedObjectType)
	event.ObjectID = util.Ptr(objectId)

	if _, err := client_v0.CreateEvent(r.APIClient, r.APIServer, event); err != nil {
		return fmt.Errorf("failed to create event: %w", err)
	}
	return nil
}

// HandleEventOverride records the given event unless the error is an
// ErrWithEvent, in which case it records the event carried by the
// error. errors.As unwraps ErrWithEvent through wrapped error chains,
// so a handler that wraps the returned error still routes the specific
// event through the same substitution path. fullyQualifiedObjectType
// must be the form returned by GetFullyQualifiedType.
func (r *EventRecorder) HandleEventOverride(
	event *api.Event,
	objectId uint,
	fullyQualifiedObjectType string,
	err error,
	log *logr.Logger,
) {
	var errWithEvent *tp_errors.ErrWithEvent
	switch {
	case errors.As(err, &errWithEvent):
		if err := r.RecordEvent(
			&errWithEvent.Event,
			objectId,
			fullyQualifiedObjectType,
		); err != nil {
			log.Error(err, "failed to record event")
		}
	default:
		// wrap the operation error into Note so the failure surfaces
		// on the recorded event; cap at 500 chars to bound row size.
		if err != nil && event != nil {
			wrapped := fmt.Sprintf("%s: %v", util.Deref(event.Note), err)
			if len(wrapped) > 500 {
				wrapped = wrapped[:500]
			}
			event.Note = util.Ptr(wrapped)
		}
		if recordErr := r.RecordEvent(
			event,
			objectId,
			fullyQualifiedObjectType,
		); recordErr != nil {
			log.Error(recordErr, "failed to record event")
		}
	}
}

// GetSuccessReasonForOperation returns the default reason for the operation.
func GetSuccessReasonForOperation(operation notifications.NotificationOperation) string {
	switch operation {
	case notifications.NotificationOperationCreated:
		return ReasonCreateSuccessful
	case notifications.NotificationOperationUpdated:
		return ReasonUpdateSuccessful
	case notifications.NotificationOperationDeleted:
		return ReasonDeleteSuccessful
	default:
		return ""
	}
}

// CreateNote returns the Note text for a CreateInProgress event. The
// format is:
//
//	"creating[; owns X, Y][; associates A, B][; marries M][; required by R]"
//
// Sources per clause:
//
//   - owns: foreign-key fields tagged relationship:"owns" from
//     RelationshipTaggedForeignKeys()
//   - associates: AssociationTypes() (has-many slices where the child
//     does not require the owner back)
//   - marries: foreign-key fields tagged relationship:"marries" plus
//     any caller-supplied extras
//   - required by: AssociationRequiredByTypes() (has-many slices where
//     the child requires the owner)
//
// A type that appears both as a tagged foreign key and in an association
// slice is emitted once via the foreign-key path.
func CreateNote(
	owner api.RelationshipTaggedForeignKeyProvider,
	marriesExtras ...string,
) string {
	owns, marries := foreignKeyKinds(owner)
	marries = append(marries, marriesExtras...)
	associates := associationKinds(owner, owns)
	requiredBy := associationRequiredByKinds(owner)

	parts := []string{"creating"}
	if len(owns) > 0 {
		parts = append(parts, ownsClause(owns))
	}
	if len(associates) > 0 {
		parts = append(parts, associatesClause(associates))
	}
	if len(marries) > 0 {
		parts = append(parts, marriesClause(marries))
	}
	if len(requiredBy) > 0 {
		parts = append(parts, requiredByClause(requiredBy))
	}
	return strings.Join(parts, "; ")
}

// UpdateNote returns the Note text for an UpdateInProgress event. An
// update does not change the owner's composition, so no clauses are
// appended.
func UpdateNote() string {
	return "updating"
}

// DeleteNote returns the Note text for a DeleteInProgress event. The
// format is:
//
//	"deleting[; cascades to owned X, Y, Z][; married M]"
//
// Owns, associates, and required-by kinds fold into a single "cascades
// to owned" list because the reconciler eagerly cascades all three.
// Marries stays separate to preserve the bidirectional 1:1 semantic.
// The Reason column carries the DeleteInProgress action, so the note
// omits the redundant "delete" verb.
func DeleteNote(
	owner api.RelationshipTaggedForeignKeyProvider,
	marriesExtras ...string,
) string {
	owns, marries := foreignKeyKinds(owner)
	marries = append(marries, marriesExtras...)
	associates := associationKinds(owner, owns)
	requiredBy := associationRequiredByKinds(owner)

	cascades := make([]string, 0, len(owns)+len(associates)+len(requiredBy))
	seen := map[string]bool{}
	for _, kind := range owns {
		if !seen[kind] {
			cascades = append(cascades, kind)
			seen[kind] = true
		}
	}
	for _, kind := range associates {
		if !seen[kind] {
			cascades = append(cascades, kind)
			seen[kind] = true
		}
	}
	for _, kind := range requiredBy {
		if !seen[kind] {
			cascades = append(cascades, kind)
			seen[kind] = true
		}
	}

	parts := []string{"deleting"}
	if len(cascades) > 0 {
		parts = append(parts, cascadeOwnedClause(cascades))
	}
	if len(marries) > 0 {
		parts = append(parts, marriedClause(marries))
	}
	return strings.Join(parts, "; ")
}

// foreignKeyKinds splits the owner's relationship-tagged foreign keys
// into the owns and marries kind lists. Owns is deduped by kebab kind;
// marries is returned in declaration order.
func foreignKeyKinds(owner api.RelationshipTaggedForeignKeyProvider) (owns, marries []string) {
	seenOwns := map[string]bool{}
	for _, fk := range owner.RelationshipTaggedForeignKeys() {
		kind := kebabKindFromQualifiedType(fk.ObjectType)
		switch fk.Relationship {
		case api.RelationshipOwns:
			if !seenOwns[kind] {
				owns = append(owns, kind)
				seenOwns[kind] = true
			}
		case api.RelationshipMarries:
			marries = append(marries, kind)
		}
	}
	return owns, marries
}

// associationKinds returns the kebab kinds from AssociationTypes(),
// skipping any kind already present in ownsSeen. Returns nil when the
// owner does not implement AssociationTypesProvider.
func associationKinds(owner api.RelationshipTaggedForeignKeyProvider, ownsSeen []string) []string {
	provider, ok := owner.(api.AssociationTypesProvider)
	if !ok {
		return nil
	}
	skip := map[string]bool{}
	for _, kind := range ownsSeen {
		skip[kind] = true
	}
	var kinds []string
	for _, qualifiedType := range provider.AssociationTypes() {
		kind := kebabKindFromQualifiedType(qualifiedType)
		if skip[kind] {
			continue
		}
		kinds = append(kinds, kind)
		skip[kind] = true
	}
	return kinds
}

// associationRequiredByKinds returns the kebab kinds from
// AssociationRequiredByTypes(). Returns nil when the owner does not
// implement AssociationRequiredByTypesProvider.
func associationRequiredByKinds(owner api.RelationshipTaggedForeignKeyProvider) []string {
	provider, ok := owner.(api.AssociationRequiredByTypesProvider)
	if !ok {
		return nil
	}
	seen := map[string]bool{}
	var kinds []string
	for _, qualifiedType := range provider.AssociationRequiredByTypes() {
		kind := kebabKindFromQualifiedType(qualifiedType)
		if seen[kind] {
			continue
		}
		kinds = append(kinds, kind)
		seen[kind] = true
	}
	return kinds
}

// ownsClause formats "owns X, Y".
func ownsClause(kinds []string) string {
	return "owns " + strings.Join(kinds, ", ")
}

// associatesClause formats "associates A, B".
func associatesClause(kinds []string) string {
	return "associates " + strings.Join(kinds, ", ")
}

// marriesClause formats "marries M".
func marriesClause(kinds []string) string {
	return "marries " + strings.Join(kinds, ", ")
}

// requiredByClause formats "required by R".
func requiredByClause(kinds []string) string {
	return "required by " + strings.Join(kinds, ", ")
}

// cascadeOwnedClause formats "cascades to owned X, Y, Z".
func cascadeOwnedClause(kinds []string) string {
	return "cascades to owned " + strings.Join(kinds, ", ")
}

// marriedClause formats "married M".
func marriedClause(kinds []string) string {
	return "married " + strings.Join(kinds, ", ")
}

// kebabKindFromQualifiedType parses a fully-qualified type name like
// "threeport.io/v0.MachineRuntimeDefinition" and returns the kebab-case
// kind ("machine-runtime-definition"). Falls back to the raw input on
// parse failure.
func kebabKindFromQualifiedType(qualifiedType string) string {
	_, _, typeName, ok := apilib.ParseQualifiedType(qualifiedType)
	if !ok {
		return qualifiedType
	}
	return strcase.ToKebab(typeName)
}
