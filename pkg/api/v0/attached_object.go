package v0

// AttachedObjectReference is a reference to an attached object.
type AttachedObjectReference struct {
	Common `swaggerignore:"true" mapstructure:",squash"`

	// The object type of the base object.
	ObjectType *string `json:"ObjectType,omitempty" query:"objecttype" gorm:"not null;uniqueIndex:idx_attached_object_unique" validate:"required"`

	// The object ID of the base object.
	ObjectID *uint `json:"ObjectID,omitempty" query:"objectid" gorm:"not null;uniqueIndex:idx_attached_object_unique" validate:"required"`

	// The object type of the attached object.
	AttachedObjectType *string `json:"AttachedObjectType,omitempty" query:"attachedobjecttype" gorm:"not null;uniqueIndex:idx_attached_object_unique" validate:"required"`

	// The object ID of the attached object.
	AttachedObjectID *uint `json:"AttachedObjectID,omitempty" query:"attachedobjectid" gorm:"not null;uniqueIndex:idx_attached_object_unique" validate:"required"`
}
