package v0

// LogBackend is where the log messages are stored.
type LogBackend struct {
	Common `swaggerignore:"true" mapstructure:",squash"`

	// The unique name of a logging back end.
	Name *string `json:"Name,omitempty" query:"name" gorm:"not null" validate:"required"`

	// The network address to connect to for storing log messages.
	Destination *string `json:"Destination,omitempty" query:"destination" gorm:"not null" validate:"required"`

	// The storage definitions using the log backend for log storage.
	LogStorageDefinitions []*LogStorageDefinition `json:"LogStorageDefinitions,omitempty" query:"logstoragedefinitions" gorm:"many2many:v0_log_backends_v0_log_storage_definitions;" validate:"optional,association"`
}

// LogStorageDefinition provides  configuration for the retention of log output
// from workloads to one or more log storage back ends.
type LogStorageDefinition struct {
	Common     `swaggerignore:"true" mapstructure:",squash"`
	Definition `mapstructure:",squash"`

	// The backend storage mechanisms for retaining logs.
	LogBackends []*LogBackend `json:"LogBackends,omitempty" query:"logbackends" gorm:"many2many:v0_log_backends_v0_log_storage_definitions;" validate:"optional,association"`

	// The associated log storage instances that are derived from this definition.
	LogStorageInstances []*LogStorageInstance `json:"LogStorageInstances,omitempty" validate:"optional,association"`
}

// An instance of log storage deployed to a compute space cluster.
type LogStorageInstance struct {
	Common   `swaggerignore:"true" mapstructure:",squash"`
	Instance `mapstructure:",squash"`

	// The definition used to define the instance.
	LogStorageDefinitionID *uint `json:"LogStorageDefinitionID,omitempty" validate:"optional,association"`

	// The cluster from which log messages are being aggregated to send to a log
	// back end.
	ClusterID *uint `json:"ClusterID,omitempty" validate:"optional,association"`
}
