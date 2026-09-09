package v0

const (
	PathModuleApiRouteWithModuleObjectReferences = "/v0/module-api-route-with-module-object-references"
	PathModuleObjectsWithModuleApiRoutes         = "/v0/module-objects-with-module-api-routes"
)

// Most API types unique-index Name among undeleted rows. The types here
// do not all follow that: ModuleApi is unique on (Name, ApiNamespace),
// ModuleObject on (Name, Version, ModuleApiID) so two versions of the
// same object can coexist on one module API. ModuleController
// unique-indexes Name alone, like most types.

// ModuleApi represents an API server for a Threeport module. The
// (Name, ApiNamespace) pair is unique.
type ModuleApi struct {
	Common `swaggerignore:"true" mapstructure:",squash"`

	// An arbitrary name for the module API.
	Name *string `validate:"required" gorm:"not null;uniqueIndex:idx_module_api_identity,where:deleted_at IS NULL"`

	// If true, represents the core Threeport API.
	Core *bool `validate:"optional" gorm:"default:false"`

	// The reverse-DNS namespace identifying this module API (e.g. "example.com").
	ApiNamespace *string `validate:"optional" gorm:"uniqueIndex:idx_module_api_identity,where:deleted_at IS NULL"`

	// The module API server's endpoint to proxy requests to for module
	// objects.
	Endpoint *string `validate:"required" gorm:"not null"`

	// The routes as URL paths to proxy requests to the API server's endpoint.
	// All supported routes for an module API should be added so that it is
	// proxied.
	ModuleApiRoutes []*ModuleApiRoute `validate:"optional,association"`

	// The controllers that are serviced by this module API.
	ModuleControllers []*ModuleController `validate:"optional,association"`

	// The API objects that are handled by this module API.
	ModuleObjects []*ModuleObject `validate:"optional,association"`
}

// ModuleApiRoute represents a route supported by a module API.
type ModuleApiRoute struct {
	Common `swaggerignore:"true" mapstructure:",squash"`

	// The URL path supported by the module API.
	Path *string `validate:"required" gorm:"not null"`

	// The module API this route belongs to.
	ModuleApiID *uint `validate:"required" gorm:"not null" relationship:"requires"`

	// The module object this route serves.
	ModuleObjects []*ModuleObject `validate:"optional,association" gorm:"many2many:v0_module_api_routes_module_objects;"`
}

// ModuleController represents a distinct controller that is a part of the Threeport control plane.
type ModuleController struct {
	Common `swaggerignore:"true" mapstructure:",squash"`

	// The name of the controller.
	Name *string `validate:"required" gorm:"not null;uniqueIndex:,where:deleted_at IS NULL"`

	// The K8s deployment name for the controller.  This allows actions to be executed against the
	// the controller workload.  Examples:
	// * disable a controller altogether when the API objects it manages are not in use.
	// * allow the Threeport agent to watch and scale-to-zero the controller.
	DeploymentName *string `validate:"required" gorm:"not null"`

	// The module API this controller is connected to.
	ModuleApiID *uint `validate:"required" gorm:"not null" relationship:"requires"`
}

// ModuleObject is an API object that is managed by a module in Threeport.  This provides
// central registry of all API objects across all modules for each Threeport control plane.
// The (Name, Version, ModuleApiID) combination is unique.
type ModuleObject struct {
	Common `swaggerignore:"true" mapstructure:",squash"`

	// The name of the API object.
	Name *string `validate:"required" gorm:"not null;uniqueIndex:idx_module_object_identity,where:deleted_at IS NULL"`

	// The version of the API object, expressed as `v0`, `v1`, `v2`, etc.
	Version *string `validate:"required" gorm:"not null;uniqueIndex:idx_module_object_identity,where:deleted_at IS NULL"`

	// A description of the API object.
	Description *string `validate:"optional"`

	// The module API this controller is connected to.
	ModuleApiID *uint `validate:"required" gorm:"not null;uniqueIndex:idx_module_object_identity,where:deleted_at IS NULL" relationship:"requires"`

	// The controller that reconciles state for this API object, if applicable.  Note: some API objects
	// do not require reconciliation by a controller - this field will be null in those cases.
	ModuleControllerID *uint `validate:"optional" relationship:"requires"`

	// The routes that service this module object.
	ModuleApiRoutes []*ModuleApiRoute `validate:"optional,association" gorm:"many2many:v0_module_api_routes_module_objects;"`
}
