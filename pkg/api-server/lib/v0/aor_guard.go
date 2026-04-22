package v0

import (
	"fmt"

	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	"gorm.io/gorm"
)

// CountBlockingAORs returns the number of blocking AORs pointing at the
// given base object. A non-zero count means the base cannot be deleted
// until the attachers release their references.
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
