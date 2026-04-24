package v0

import (
	"bytes"
	"errors"
	"fmt"
	"text/tabwriter"

	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	"gorm.io/gorm"
)

// maxBlockingAttachedObjectReferencesInMessage caps the number of rows
// listed in the 409 body so the error message stays a readable size.
const maxBlockingAttachedObjectReferencesInMessage = 10

// FindBlockingAttachedObjectReferences returns attached object references that
// point at the given object and are marked blocking.  Results are soft-delete
// aware and ordered by ID.  When limit > 0, at most `limit` references are
// returned along with the unlimited total count.
func FindBlockingAttachedObjectReferences(
	db *gorm.DB,
	objectType string,
	objectID *uint,
	limit int,
) ([]api_v0.AttachedObjectReference, int64, error) {
	// count the total blocking references for the object
	var total int64
	if err := db.Model(&api_v0.AttachedObjectReference{}).
		Where(
			"object_type = ? AND object_id = ? AND blocking = true AND deleted_at IS NULL",
			objectType, objectID,
		).
		Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count blocking attached object references: %w", err)
	}
	if total == 0 {
		return nil, 0, nil
	}

	// fetch the blocking references up to the given limit
	var attachedObjectReferences []api_v0.AttachedObjectReference
	fetch := db.
		Where(
			"object_type = ? AND object_id = ? AND blocking = true AND deleted_at IS NULL",
			objectType, objectID,
		).
		Order("id")
	if limit > 0 {
		fetch = fetch.Limit(limit)
	}
	if err := fetch.Find(&attachedObjectReferences).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list blocking attached object references: %w", err)
	}

	return attachedObjectReferences, total, nil
}

// FormatBlockingAttachedObjectReferencesError returns an error describing the
// blocking attached object references as an aligned two-column table,
// suitable for a 409 response body.  Returns nil when totalCount is zero.
func FormatBlockingAttachedObjectReferencesError(
	baseTypeLabel string,
	attachedObjectReferences []api_v0.AttachedObjectReference,
	totalCount int64,
) error {
	if totalCount == 0 {
		return nil
	}

	// build the aligned table of blocking references
	var buf bytes.Buffer
	writer := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	fmt.Fprintln(writer, "  TYPE\tID")
	for _, attachedObjectReference := range attachedObjectReferences {
		attachedObjectType := "<unknown>"
		attachedObjectID := "<unknown>"
		if attachedObjectReference.AttachedObjectType != nil {
			attachedObjectType = *attachedObjectReference.AttachedObjectType
		}
		if attachedObjectReference.AttachedObjectID != nil {
			attachedObjectID = fmt.Sprintf("%d", *attachedObjectReference.AttachedObjectID)
		}
		fmt.Fprintf(writer, "  %s\t%s\n", attachedObjectType, attachedObjectID)
	}
	writer.Flush()

	// assemble the header, table, and trailer
	msg := fmt.Sprintf(
		"%s cannot be deleted while %d object(s) still reference it:\n\n%s",
		baseTypeLabel, totalCount, buf.String(),
	)
	if totalCount > int64(len(attachedObjectReferences)) {
		msg += fmt.Sprintf("\n...and %d more\n", totalCount-int64(len(attachedObjectReferences)))
	}
	msg += "\nRemove dependents first."

	return errors.New(msg)
}
