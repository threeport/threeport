// +threeport-codegen route-exclude
// +threeport-codegen database-exclude
package v0

import (
	"time"

	"gorm.io/gorm"
)

// Common includes standard fields included in most objects.
type Common struct {
	ID        *uint           `json:"ID,omitempty" gorm:"primarykey"`
	CreatedAt *time.Time      `json:"CreatedAt,omitempty"`
	UpdatedAt *time.Time      `json:"UpdatedAt,omitempty"`
	DeletedAt *gorm.DeletedAt `json:"DeletedAt,omitempty" gorm:"index"`
}
