// +threeport-codegen route-exclude
package v0

type Instance struct {
	// An arbitrary name the instance
	Name *string `json:"Name,omitempty" query:"name" gorm:"not null"  validate:"required"`

	// The status of the instance.
	//TODO: use a custom type
	Status *string `json:"Status,omitempty" query:"status" gorm:"not null" validate:"required"`
}
