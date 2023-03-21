// +threeport-codegen route-exclude
package v0

// Definition includes a set of fields for every definition object.
type Definition struct {
	// An arbitrary name for the definition.
	Name *string `json:"Name,omitempty" query:"name" gorm:"not null" validate:"required"`

	// The profile to associate with the definition.  Profile is a named
	// standard configuration for a definition object.
	ProfileID *uint `json:"ProfileID,omitempty" validate:"optional,association"`

	// The tier to associate with the definition.  Tier is a level of
	// criticality for access control.
	TierID *uint `json:"TierID,omitempty" validate:"optional,association"`
}
