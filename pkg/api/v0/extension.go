package v0

// ExtensionApi is a Threeport extension API server that needs to be registered
// with the Threeport API so that requests for extension-handled objects are
// proxied to it.
type ExtensionApi struct {
	Common `swaggerignore:"true" mapstructure:",squash"`

	// An arbitrary name for the extension API.
	Name *string `json:"Name,omitempty" gorm:"not null" query:"name" validate:"required"`

	// The extension API server's endpoint to proxy requests to for extension
	// objects.
	Endpoint *string `json:"Endpoint,omitempty" gorm:"not null" query:"endpoint" validate:"required"`

	// The routes as URL paths to proxy requests to the API server's endpoint.
	// All supported routes for an extension API should be added so that it is
	// proxied.
	ExtensionApiRoutes []*ExtensionApiRoute `json:"ExtensionApiRoutes,omitempty" validate:"optional,association"`
}

// Route defines a URL path that is proxied for an extension API.
type ExtensionApiRoute struct {
	Common `swaggerignore:"true" mapstructure:",squash"`

	// The URL path supported by the extension API.
	Path *string `json:"Path,omitempty" query:"path" validate:"required" gorm:"not null"`

	// The extension Api this route belongs to.
	ExtensionApiID *uint `json:"ExtensionApiId,omitempty" query:"extensionapiid" validate:"required" gorm:"not null"`
}
