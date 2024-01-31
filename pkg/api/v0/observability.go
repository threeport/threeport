//go:generate threeport-codegen api-model --filename $GOFILE --package $GOPACKAGE
//go:generate threeport-codegen controller --filename $GOFILE
package v0

// +threeport-codegen:reconciler
// MetricsDefinition defines a metrics aggregation layer for a workload.
type MetricsDefinition struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Definition     `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The Grafana Helm workload definition that belongs to this resource.
	GrafanaHelmWorkloadDefinitionID *uint `json:"GrafanaHelmWorkloadDefinitionID,omitempty" query:"grafanahelmworkloaddefinitionid" validate:"optional"`

	// The kube-prometheus-stack Helm workload definition that belongs to this resource.
	KubePrometheusStackHelmWorkloadDefinitionID *uint `json:"KubePrometheusStackHelmWorkloadDefinitionID,omitempty" query:"kubeprometheusstackhelmworkloaddefinitionid" validate:"optional"`

	// // Optional Helm workload definition values that can be provided to configure the
	// // underlying grafana chart.
	// GrafanaHelmValues *string `json:"GrafanaHelmValues,omitempty" query:"grafanahelmvalues" validate:"optional"`

	// // Optional Helm workload definition values that can be provided to configure the
	// // underlying kube-prometheus-stack chart.
	// KubePrometheusStackHelmValues *string
	// `json:"KubePrometheusStackHelmValues,omitempty"
	// query:"kubeprometheusstackhelmvalues" validate:"optional"`

	// The associated metrics instances that are deployed from this definition.
	MetricsInstances []*MetricsInstance `json:"MetricsInstances,omitempty" validate:"optional,association"`
}

// +threeport-codegen:reconciler
// MetricsInstances defines an instance of a metrics aggregation layer for a workload.
type MetricsInstance struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Instance       `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// MetricsDefinitionID is the definition used to configure the workload instance.
	MetricsDefinitionID *uint `json:"MetricsDefinitionID,omitempty" query:"metricsdefinitionid" gorm:"not null" validate:"required"`

	// The kubernetes runtime where the ingress layer is installed.
	KubernetesRuntimeInstanceID *uint `json:"KubernetesRuntimeInstanceID,omitempty" query:"kubernetesruntimeinstanceid" gorm:"not null" validate:"required"`

	// The Grafana Helm workload definition that belongs to this resource.
	GrafanaHelmWorkloadInstanceID *uint `json:"GrafanaHelmWorkloadInstanceID,omitempty" query:"grafanahelmworkloadinstanceid" validate:"optional"`

	// The kube-prometheus-stack Helm workload definition that belongs to this resource.
	KubePrometheusStackHelmWorkloadInstanceID *uint `json:"KubePrometheusStackHelmWorkloadInstanceID,omitempty" query:"kubeprometheusstackhelmworkloadinstanceid" validate:"optional"`

	// Optional Helm workload definition values that can be provided to configure the
	// underlying grafana chart.
	GrafanaHelmValues *string `json:"GrafanaHelmValues,omitempty" query:"grafanahelmvalues" validate:"optional"`

	// Optional Helm workload definition values that can be provided to configure the
	// underlying kube-prometheus-stack chart.
	KubePrometheusStackHelmValues *string `json:"KubePrometheusStackHelmValues,omitempty" query:"kubeprometheusstackhelmvalues" validate:"optional"`
}

// +threeport-codegen:reconciler
// MetricsDefinition defines a metrics aggregation layer for a workload.
type LoggingDefinition struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Definition     `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The Grafana Helm workload definition that belongs to this resource.
	GrafanaHelmWorkloadDefinitionID *uint `json:"GrafanaHelmWorkloadDefinitionID,omitempty" query:"grafanahelmworkloaddefinitionid" validate:"optional"`

	// The kube-prometheus-stack Helm workload definition that belongs to this resource.
	LokiHelmWorkloadDefinitionID *uint `json:"LokiHelmWorkloadDefinitionID,omitempty" query:"lokihelmworkloaddefinitionid" validate:"optional"`

	// The kube-prometheus-stack Helm workload definition that belongs to this resource.
	PromtailHelmWorkloadDefinitionID *uint `json:"PromtailHelmWorkloadDefinitionID,omitempty" query:"promtailhelmworkloaddefinitionid" validate:"optional"`

	// // Optional Helm workload definition values that can be provided to configure the
	// // underlying grafana chart.
	// GrafanaHelmValues *string `json:"GrafanaHelmValues,omitempty" query:"grafanahelmvalues" validate:"optional"`

	// // Optional Helm workload definition values that can be provided to configure the
	// // underlying kube-prometheus-stack chart.
	// KubePrometheusStackHelmValues *string
	// `json:"KubePrometheusStackHelmValues,omitempty"
	// query:"kubeprometheusstackhelmvalues" validate:"optional"`

	// The associated metrics instances that are deployed from this definition.
	MetricsInstances []*MetricsInstance `json:"MetricsInstances,omitempty" validate:"optional,association"`
}

// +threeport-codegen:reconciler
// MetricsInstances defines an instance of a metrics aggregation layer for a workload.
type LoggingInstance struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Instance       `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// MetricsDefinitionID is the definition used to configure the workload instance.
	LoggingDefinitionID *uint `json:"LoggingDefinitionID,omitempty" query:"loggingdefinitionid" gorm:"not null" validate:"required"`

	// The kubernetes runtime where the ingress layer is installed.
	KubernetesRuntimeInstanceID *uint `json:"KubernetesRuntimeInstanceID,omitempty" query:"kubernetesruntimeinstanceid" gorm:"not null" validate:"required"`

	// The Grafana Helm workload definition that belongs to this resource.
	GrafanaHelmWorkloadInstanceID *uint `json:"GrafanaHelmWorkloadInstanceID,omitempty" query:"grafanahelmworkloadinstanceid" validate:"optional"`

	// The kube-prometheus-stack Helm workload definition that belongs to this resource.
	LokiHelmWorkloadInstanceID *uint `json:"LokiHelmWorkloadInstanceID,omitempty" query:"lokihelmworkloadinstanceid" validate:"optional"`

	// The kube-prometheus-stack Helm workload definition that belongs to this resource.
	PromtailHelmWorkloadInstanceID *uint `json:"PromtailHelmWorkloadInstanceID,omitempty" query:"promtailhelmworkloadinstanceid" validate:"optional"`

	// Optional Helm workload definition values that can be provided to configure the
	// underlying grafana chart.
	GrafanaHelmValues *string `json:"GrafanaHelmValues,omitempty" query:"grafanahelmvalues" validate:"optional"`

	// Optional Helm workload definition values that can be provided to configure the
	// underlying kube-prometheus-stack chart.
	LokiHelmValues *string `json:"LokiHelmValues,omitempty" query:"lokihelmvalues" validate:"optional"`

	// Optional Helm workload definition values that can be provided to configure the
	// underlying kube-prometheus-stack chart.
	PromtailHelmValues *string `json:"PromtailHelmValues,omitempty" query:"promtailhelmvalues" validate:"optional"`
}