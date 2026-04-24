package v0

import (
	"bytes"
	"errors"
	"fmt"
	"text/tabwriter"

	"github.com/iancoleman/strcase"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	"gorm.io/gorm"
)

// FindBlockingAttachedObjectReferences returns attached object references that
// point at the given object and are marked blocking.
func FindBlockingAttachedObjectReferences(
	db *gorm.DB,
	objectType string,
	objectID *uint,
) ([]api_v0.AttachedObjectReference, error) {
	var attachedObjectReferences []api_v0.AttachedObjectReference
	if err := db.
		Where("object_type = ? AND object_id = ? AND blocking = true", objectType, objectID).
		Find(&attachedObjectReferences).Error; err != nil {
		return nil, fmt.Errorf("failed to list blocking attached object references: %w", err)
	}
	return attachedObjectReferences, nil
}

// FormatBlockingAttachedObjectReferencesError returns an error describing the
// blocking attached object references as an aligned two-column table,
// suitable for a 409 response body.
func FormatBlockingAttachedObjectReferencesError(
	attachedObjectReferences []api_v0.AttachedObjectReference,
) error {
	baseType := "object"
	if len(attachedObjectReferences) > 0 && attachedObjectReferences[0].ObjectType != nil {
		baseType = strcase.ToDelimited(*attachedObjectReferences[0].ObjectType, ' ')
	}

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

	msg := fmt.Sprintf(
		"%s cannot be deleted while %d object(s) still reference it:\n\n%s\nRemove dependents first.",
		baseType, len(attachedObjectReferences), buf.String(),
	)
	return errors.New(msg)
}
