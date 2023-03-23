//go:generate ../../../bin/threeport-codegen api-model --filename $GOFILE --package $GOPACKAGE
package v0

import (
	"time"
)

// User is a human individual that owns - or represents a company that owns -
// shares in pools of staked nodes.
type User struct {
	Common `swaggerignore:"true"`

	// User's email address.
	Email *string `json:"Email,omitempty" query:"email" gorm:"unique;not null" validate:"required"`

	// User's account password.
	Password *string `json:"Password,omitempty" gorm:"not null" validate:"required"`

	// User's first name.
	FirstName *string `json:"FirstName,omitempty" query:"firstname" gorm:"not null" validate:"required"`

	// User's last name.
	LastName *string `json:"LastName,omitempty" query:"lastname" gorm:"not null" validate:"required"`

	// User's date of birth.  Format: `2006-01-02T00:00:00Z`
	DateOfBirth *time.Time `json:"DateOfBirth,omitempty" query:"dateofbirth" gorm:"not null" validate:"required"`

	// Company that the user represents.
	CompanyID *uint `json:"CompanyID,omitempty" query:"companyid" validate:"optional"`

	// Country where user resides.
	CountryOfResidence *string `json:"CountryOfResidence,omitempty" query:"country" gorm:"not null" validate:"required"`

	// Country of which user is a citizen.
	Nationality *string `json:"Nationality,omitempty" query:"nationality" gorm:"not null" validate:"required"`
}

// Company is an organization that owns shares in pools of nodes.
type Company struct {
	Common `swaggerignore:"true"`

	// Company's legal name.
	Name *string `json:"Name,omitempty" query:"name" gorm:"not null" validate:"required"`

	// Users that represent the company.
	Users []*User `json:"Users,omitempty"  validate:"optional,association"`
}
