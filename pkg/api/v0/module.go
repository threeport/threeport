package v0

// ModuleApi represents an API server for a Threeport module.
type ModuleApi struct {
	Common `swaggerignore:"true" mapstructure:",squash"`

	// An arbitrary name for the module API.
	Name *string `json:"Name,omitempty" gorm:"not null" query:"name" validate:"required"`

	// The module API server's endpoint to proxy requests to for module
	// objects.
	Endpoint *string `json:"Endpoint,omitempty" gorm:"not null" query:"endpoint" validate:"required"`

	// The routes as URL paths to proxy requests to the API server's endpoint.
	// All supported routes for an extension API should be added so that it is
	// proxied.
	ModuleApiRoutes []*ModuleApiRoute `json:"ModuleApiRoutes,omitempty" validate:"optional,association"`
}

// ModuleApiRoute represents a route supported by a module API.
type ModuleApiRoute struct {
	Common `swaggerignore:"true" mapstructure:",squash"`

	// The URL path supported by the module API.
	Path *string `json:"Path,omitempty" query:"path" validate:"required" gorm:"not null"`

	// The module Api this route belongs to.
	ModuleApiID *uint `json:"ModuleApiID,omitempty" query:"moduleapiid" validate:"required" gorm:"not null"`
}
