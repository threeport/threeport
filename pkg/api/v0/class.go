// +threeport-codegen route-exclude
package v0

// Definition includes a set of fields for every definition object.
type Definition struct {
	// An arbitrary name for the definition.
	Name *string `json:"Name,omitempty" query:"name" gorm:"not null" validate:"required"`

	// Required if no CompanyID.  The user that owns the object.
	UserID *uint `json:"UserID,omitempty" query:"userid" validate:"optional"`

	// Required if no UserID.  The company that owns the object.
	CompanyID *uint `json:"CompanyID,omitempty" query:"companyid" validate:"optional"`

	// The profile to associate with the definition.  Profile is a named
	// standard configuration for a definition object.
	ProfileID *uint `json:"ProfileID,omitempty" validate:"optional,association"`

	// The tier to associate with the definition.  Tier is a level of
	// criticality for access control.
	TierID *uint `json:"TierID,omitempty" validate:"optional,association"`
}

// Instance includes a set of fields for every instance object.
type Instance struct {
	// An arbitrary name the instance
	Name *string `json:"Name,omitempty" query:"name" gorm:"not null"  validate:"required"`

	// Required if no CompanyID.  The user that owns the object.
	UserID *uint `json:"UserID,omitempty" query:"userid" validate:"optional"`

	// Required if no UserID.  The company that owns the object.
	CompanyID *uint `json:"CompanyID,omitempty" query:"companyid" validate:"optional"`

	// The status of the instance.
	//TODO: use a custom type
	Status *string `json:"Status,omitempty" query:"status" validate:"optional"`
}
