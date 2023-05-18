//go:generate ../../../bin/threeport-codegen api-model --filename $GOFILE --package $GOPACKAGE
package v0

// LogBackend is where the log messages are stored.  This is referenced
type LogBackend struct {
	Common `swaggerignore:"true" mapstructure:",squash"`

	// The unique name of a logging back end.
	Name *string `json:"Name,omitempty" query:"name" gorm:"not null" validate:"required"`

	// The network address to connect to for storing log messages.
	Destination *string `json:"Destination,omitempty" query:"destination" gorm:"not null" validate:"required"`
}

// LogStorage provides retention of log output from a workload in one or more
// log storage back ends.
type LogStorageDefinition struct {
	Common     `swaggerignore:"true" mapstructure:",squash"`
	Definition `mapstructure:",squash"`

	// The backend storage mechanisms for retaining logs.
	LogBackends []*LogBackend `json:"LogBackends,omitempty" query:"logbackends" validate:"optional,association"`
}

type LogStorageInstance struct {
	Common   `swaggerignore:"true" mapstructure:",squash"`
	Instance `mapstructure:",squash"`

	// The definition used to define the instance.
	LogStorageDefinitionID *uint `json:"LogStorageDefinitionID,omitempty" validate:"optional,association"`

	// The cluster from which log messages are being aggregated to send to a log
	// back end.
	ClusterID *uint `json:"ClusterID,omitempty" validate:"optional,association"`
}
