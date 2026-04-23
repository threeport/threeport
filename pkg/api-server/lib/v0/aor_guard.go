package v0

import (
	"fmt"
	"strings"

	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	"gorm.io/gorm"
)

// CountBlockingAORs returns the number of non-soft-deleted AOR rows with
// blocking=true that reference the given base object.
func CountBlockingAORs(db *gorm.DB, objectType string, objectID *uint) (int64, error) {
	var count int64
	result := db.Model(&api_v0.AttachedObjectReference{}).
		Where("object_type = ? AND object_id = ? AND blocking = true AND deleted_at IS NULL",
			objectType, objectID).
		Count(&count)
	if result.Error != nil {
		return 0, fmt.Errorf("failed to count blocking attached object references: %w", result.Error)
	}
	return count, nil
}

const maxBlockingAORsInMessage = 10

// BlockingAORsError returns (nil, nil) when nothing is blocking; otherwise
// the first error is suitable for a 409 response body.
func BlockingAORsError(db *gorm.DB, baseTypeLabel, objectType string, objectID *uint) (error, error) {
	var total int64
	if err := db.Model(&api_v0.AttachedObjectReference{}).
		Where("object_type = ? AND object_id = ? AND blocking = true AND deleted_at IS NULL",
			objectType, objectID).
		Count(&total).Error; err != nil {
		return nil, fmt.Errorf("failed to count blocking attached object references: %w", err)
	}
	if total == 0 {
		return nil, nil
	}

	var refs []api_v0.AttachedObjectReference
	if err := db.
		Where("object_type = ? AND object_id = ? AND blocking = true AND deleted_at IS NULL",
			objectType, objectID).
		Order("id").
		Limit(maxBlockingAORsInMessage).
		Find(&refs).Error; err != nil {
		return nil, fmt.Errorf("failed to list blocking attached object references: %w", err)
	}

	parts := make([]string, 0, len(refs))
	for _, r := range refs {
		t, id := "<unknown>", "<unknown>"
		if r.AttachedObjectType != nil {
			t = *r.AttachedObjectType
		}
		if r.AttachedObjectID != nil {
			id = fmt.Sprintf("%d", *r.AttachedObjectID)
		}
		parts = append(parts, fmt.Sprintf("%s(%s)", t, id))
	}

	msg := fmt.Sprintf(
		"%s cannot be deleted while %d object(s) still reference it: %s",
		baseTypeLabel, total, strings.Join(parts, ", "),
	)
	if total > int64(len(refs)) {
		msg = fmt.Sprintf("%s, ...and %d more", msg, total-int64(len(refs)))
	}
	msg += " - remove dependents first"
	return fmt.Errorf("%s", msg), nil
}
