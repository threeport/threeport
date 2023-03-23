//go:generate ../../../bin/threeport-codegen api-model --filename $GOFILE --package $GOPACKAGE
package v0

// LogBackend is where the log messages are stored.  This is referenced
type LogBackend struct {
	Common `swaggerignore:"true" mapstructure:",squash"`

	// The network address to connect to for storing log messages.
	Destination *string `json:"Destination,omitempty" query:"destination" gorm:"not null" validate:"required"`
}

// LogStorage provides retention of log output from a workload in one or more
// log storage back ends.
type LogStorageDefinition struct {
	Common     `swaggerignore:"true" mapstructure:",squash"`
	Definition `mapstructure:",squash"`

	// The backend storage mechanisms for retaining logs.
	LogBackends *[]LogBackend `json:"LogBackends,omitempty" validate:"optional,association"`
}

type LogStorageInstance struct {
	Common   `swaggerignore:"true" mapstructure:",squash"`
	Instance `mapstructure:",squash"`

	LogStorageDefinitionID *uint `json:"LogStorageDefinitionID,omitempty" validate:"optional,association"`

	ClusterID *uint `json:"ClusterID,omitempty" validate:"optional,association"`
}
