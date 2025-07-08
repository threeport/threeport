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
	// All supported routes for an module API should be added so that it is
	// proxied.
	ModuleApiRoutes []*ModuleApiRoute `json:"ModuleApiRoutes,omitempty" validate:"optional,association"`

	// The controllers that are serviced by this module API.
	ModuleControllers []*ModuleController `json:"ModuleControllers,omitempty" validate:"optional,association"`

	// The API objects that are handled by this module API.
	ModuleObjects []*ModuleObject `json:"ModuleObjects,omitempty" validate:"optional,association"`
}

// ModuleApiRoute represents a route supported by a module API.
type ModuleApiRoute struct {
	Common `swaggerignore:"true" mapstructure:",squash"`

	// The URL path supported by the module API.
	Path *string `json:"Path,omitempty" query:"path" validate:"required" gorm:"not null"`

	// The module API this route belongs to.
	ModuleApiID *uint `json:"ModuleApiID,omitempty" query:"moduleapiid" validate:"required" gorm:"not null"`
}

// ModuleController represents a distinct controller that is a part of the Threeport control plane.
type ModuleController struct {
	Common `swaggerignore:"true" mapstructure:",squash"`

	// The name of the controller.
	Name *string `json:"Name,omitempty" query:"name" validate:"required" gorm:"not null"`

	// The K8s deployment name for the controller.  This allows actions to be executed against the
	// the controller workload.  Examples:
	// * disable a controller altogether when the API objects it manages are not in use.
	// * allow the Threeport agent to watch and scale-to-zero the controller.
	DeploymentName *string `json:"DeploymentName,omitempty" query:"deploymentname" validate:"required" gorm:"not null"`

	// The module API this controller is connected to.
	ModuleApiID *uint `json:"ModuleApiID,omitempty" query:"moduleapiid" validate:"required" gorm:"not null"`
}

// ModuleObject is an API object that is managed by a module in Threeport.  This provides
// central registry of all API objects across all modules for each Threeport control plane.
type ModuleObject struct {
	Common `swaggerignore:"true" mapstructure:",squash"`

	// The name of the API object.
	Name *string `json:"Name,omitempty" query:"name" validate:"required" gorm:"not null"`

	// The version of the API object, expressed as `v0`, `v1`, `v2`, etc.
	Version *string `json:"Version,omitempty" query:"version" validate:"required" gorm:"not null"`

	// If true, only accessible to internal controllers that are a part of the
	// Threeport control plane.  Threeport users cannot access if true.
	InternalOnly *bool `json:"InternalOnly,omitempty" query:"internalonly" validate:"optional" gorm:"defatul:false"`

	// The module API this controller is connected to.
	ModuleApiID *uint `json:"ModuleApiID,omitempty" query:"moduleapiid" validate:"required" gorm:"not null"`

	// The controller that reconciles state for this API object, if applicable.  Note: some API objects
	// do not require reconciliation by a controller - this field will be null in those cases.
	ModuleControllerID *uint `json:"ModuleControllerID,omitempty" query:"modulecontrollerid" validate:"optional"`
}
