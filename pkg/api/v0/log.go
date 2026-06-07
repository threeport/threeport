package v0

// LogBackend is where the log messages are stored.
type LogBackend struct {
	Common `swaggerignore:"true" mapstructure:",squash"`

	// The unique name of a logging back end.
	Name *string `json:",omitempty" validate:"required" gorm:"not null"`

	// The network address to connect to for storing log messages.
	Destination *string `json:",omitempty" validate:"required" gorm:"not null"`

	// The storage definitions using the log backend for log storage.
	LogStorageDefinitions []*LogStorageDefinition `json:",omitempty" validate:"optional,association" gorm:"many2many:v0_log_backends_v0_log_storage_definitions;"`
}

// LogStorageDefinition provides  configuration for the retention of log output
// from workloads to one or more log storage back ends.
type LogStorageDefinition struct {
	Common     `swaggerignore:"true" mapstructure:",squash"`
	Definition `mapstructure:",squash"`

	// The backend storage mechanisms for retaining logs.
	LogBackends []*LogBackend `json:",omitempty" validate:"optional,association" gorm:"many2many:v0_log_backends_v0_log_storage_definitions;"`

	// The associated log storage instances that are derived from this definition.
	LogStorageInstances []*LogStorageInstance `json:",omitempty" validate:"optional,association"`
}

// LogStorageInstance is an instance of log storage deployed to a compute space cluster.
type LogStorageInstance struct {
	Common   `swaggerignore:"true" mapstructure:",squash"`
	Instance `mapstructure:",squash"`

	// The definition used to define the instance.
	LogStorageDefinitionID *uint `json:",omitempty" validate:"optional,association"`

	// The cluster from which log messages are being aggregated to send to a log
	// back end.
	ClusterID *uint `json:",omitempty" validate:"optional,association" relationship:"requires;type:KubernetesRuntimeInstance"`
}
