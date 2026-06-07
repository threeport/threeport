package v0

const (
	PathModuleApiRouteWithModuleObjectReferences = "/v0/module-api-route-with-module-object-references"
	PathModuleObjectsWithModuleApiRoutes         = "/v0/module-objects-with-module-api-routes"
)

// ModuleApi represents an API server for a Threeport module. The
// (Name, ApiNamespace) pair is unique.
type ModuleApi struct {
	Common `swaggerignore:"true" mapstructure:",squash"`

	// An arbitrary name for the module API.
	Name *string `json:",omitempty" validate:"required" gorm:"not null;uniqueIndex:idx_module_api_identity"`

	// If true, represents the core Threeport API.
	Core *bool `json:",omitempty" validate:"optional" gorm:"default:false"`

	// The reverse-DNS namespace identifying this module API (e.g. "example.com").
	ApiNamespace *string `json:",omitempty" validate:"optional" gorm:"uniqueIndex:idx_module_api_identity"`

	// The module API server's endpoint to proxy requests to for module
	// objects.
	Endpoint *string `json:",omitempty" validate:"required" gorm:"not null"`

	// The routes as URL paths to proxy requests to the API server's endpoint.
	// All supported routes for an module API should be added so that it is
	// proxied.
	ModuleApiRoutes []*ModuleApiRoute `json:",omitempty" validate:"optional,association"`

	// The controllers that are serviced by this module API.
	ModuleControllers []*ModuleController `json:",omitempty" validate:"optional,association"`

	// The API objects that are handled by this module API.
	ModuleObjects []*ModuleObject `json:",omitempty" validate:"optional,association"`
}

// ModuleApiRoute represents a route supported by a module API.
type ModuleApiRoute struct {
	Common `swaggerignore:"true" mapstructure:",squash"`

	// The URL path supported by the module API.
	Path *string `json:",omitempty" validate:"required" gorm:"not null"`

	// The module API this route belongs to.
	ModuleApiID *uint `json:",omitempty" validate:"required" gorm:"not null" relationship:"requires"`

	// The module object this route serves.
	ModuleObjects []*ModuleObject `json:",omitempty" validate:"optional,association" gorm:"many2many:v0_module_api_routes_module_objects;"`
}

// ModuleController represents a distinct controller that is a part of the Threeport control plane.
type ModuleController struct {
	Common `swaggerignore:"true" mapstructure:",squash"`

	// The name of the controller.
	Name *string `json:",omitempty" validate:"required" gorm:"not null"`

	// The K8s deployment name for the controller.  This allows actions to be executed against the
	// the controller workload.  Examples:
	// * disable a controller altogether when the API objects it manages are not in use.
	// * allow the Threeport agent to watch and scale-to-zero the controller.
	DeploymentName *string `json:",omitempty" validate:"required" gorm:"not null"`

	// The module API this controller is connected to.
	ModuleApiID *uint `json:",omitempty" validate:"required" gorm:"not null" relationship:"requires"`
}

// ModuleObject is an API object that is managed by a module in Threeport.  This provides
// central registry of all API objects across all modules for each Threeport control plane.
type ModuleObject struct {
	Common `swaggerignore:"true" mapstructure:",squash"`

	// The name of the API object.
	Name *string `json:",omitempty" validate:"required" gorm:"not null"`

	// The version of the API object, expressed as `v0`, `v1`, `v2`, etc.
	Version *string `json:",omitempty" validate:"required" gorm:"not null"`

	// A description of the API object.
	Description *string `json:",omitempty" validate:"optional"`

	// The module API this controller is connected to.
	ModuleApiID *uint `json:",omitempty" validate:"required" gorm:"not null" relationship:"requires"`

	// The controller that reconciles state for this API object, if applicable.  Note: some API objects
	// do not require reconciliation by a controller - this field will be null in those cases.
	ModuleControllerID *uint `json:",omitempty" validate:"optional" relationship:"requires"`

	// The routes that service this module object.
	ModuleApiRoutes []*ModuleApiRoute `json:",omitempty" validate:"optional,association" gorm:"many2many:v0_module_api_routes_module_objects;"`
}
