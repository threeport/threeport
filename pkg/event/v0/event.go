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

// RecordEvent stamps the reporting controller, timestamps, and count 1,
// binds the event to the subject object, and stores it.
func (r *EventRecorder) RecordEvent(
	event *api.Event,
	objectId uint,
	fullyQualifiedObjectType string,
) error {
	// stamp controller, timestamps, count 1, and an empty note when unset
	now := time.Now()
	event.ReportingController = &r.ReportingController
	event.EventTime = util.Ptr(now)
	event.LastObservedTime = util.Ptr(now)
	event.Count = util.Ptr(uint(1))
	if event.Note == nil {
		event.Note = util.Ptr("")
	}

	// bind the event to the subject object
	event.ObjectType = util.Ptr(fullyQualifiedObjectType)
	event.ObjectID = util.Ptr(objectId)

	if _, err := client_v0.CreateEvent(r.APIClient, r.APIServer, event); err != nil {
		return fmt.Errorf("failed to create event: %w", err)
	}
	return nil
}

// HandleEventOverride records the event carried by a typed error, or the
// fallback event with the error appended to its note.
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
		// append the error to the fallback note, truncated to 500 bytes to bound row size
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

// CreateNote returns a create-progress note listing owned, associated,
// married, and required-by kinds. Extra married kinds may be supplied.
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

// UpdateNote returns the update-progress note. An update does not change
// the owner's composition, so no relationship clauses are appended.
func UpdateNote() string {
	return "updating"
}

// DeleteNote returns a delete-progress note listing cascade targets and
// married kinds. Extra married kinds may be supplied.
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

// foreignKeyKinds returns unique owned kinds and married kinds in
// declaration order from the owner's tagged foreign keys.
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

// associationKinds returns associated kinds that are not already listed
// as owned, or nil if the owner does not provide association types.
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

// associationRequiredByKinds returns unique kinds that list this owner
// as required, or nil if the owner does not provide them.
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

// ownsClause formats the owned-kinds fragment of a progress note.
func ownsClause(kinds []string) string {
	return "owns " + strings.Join(kinds, ", ")
}

// associatesClause formats the associated-kinds fragment of a progress note.
func associatesClause(kinds []string) string {
	return "associates " + strings.Join(kinds, ", ")
}

// marriesClause formats the married-kinds fragment of a create-progress note.
func marriesClause(kinds []string) string {
	return "marries " + strings.Join(kinds, ", ")
}

// requiredByClause formats the required-by-kinds fragment of a progress note.
func requiredByClause(kinds []string) string {
	return "required by " + strings.Join(kinds, ", ")
}

// cascadeOwnedClause formats the cascade-target fragment of a delete-progress note.
func cascadeOwnedClause(kinds []string) string {
	return "cascades to owned " + strings.Join(kinds, ", ")
}

// marriedClause formats the married-kinds fragment of a delete-progress note.
func marriedClause(kinds []string) string {
	return "married " + strings.Join(kinds, ", ")
}

// kebabKindFromQualifiedType returns the kebab-case type name from a
// qualified type such as threeport.io/v0.Widget, or the input if parsing fails.
func kebabKindFromQualifiedType(qualifiedType string) string {
	_, _, typeName, ok := apilib.ParseQualifiedType(qualifiedType)
	if !ok {
		return qualifiedType
	}
	return strcase.ToKebab(typeName)
}
