//go:generate threeport-codegen api-model --filename $GOFILE --package $GOPACKAGE
//go:generate threeport-codegen controller --filename $GOFILE
package v0

// +threeport-codegen:reconciler
// ObservabilityStackDefinition defines an observability stack.
type ObservabilityStackDefinition struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Definition     `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// Dashboard
	// The observability dashboard definition that belongs to this resource.
	ObservabilityDashboardDefinitionID *uint `json:"ObservabilityDashboardDefinitionID,omitempty" query:"observabilitydashboarddefinitionid" validate:"optional"`

	// The version of the grafana helm chart to use from the helm repo, e.g. 1.2.3
	GrafanaHelmChartVersion *string `json:"GrafanaHelmChartVersion,omitempty" query:"grafanahelmchartversion" validate:"optional"`

	// Optional Helm workload definition values that can be provided to configure the
	// underlying grafana chart.
	GrafanaHelmValuesDocument *string `json:"GrafanaHelmValuesDocument,omitempty" query:"grafanahelmvaluesdocument" validate:"optional"`

	// Metrics
	// The metrics definition that belongs to this resource.
	MetricsDefinitionID *uint `json:"MetricsDefinitionID,omitempty" query:"metricsdefinitionid" validate:"optional"`

	// The version of the kube-prometheus-stack helm chart to use from the helm repo, e.g. 1.2.3
	KubePrometheusStackHelmChartVersion *string `json:"KubePrometheusStackHelmChartVersion,omitempty" query:"kubeprometheusstackhelmchartversion" validate:"optional"`

	// Optional Helm workload definition values that can be provided to configure the
	// underlying kube-prometheus-stack chart.
	KubePrometheusStackHelmValuesDocument *string `json:"KubePrometheusStackHelmValuesDocument,omitempty" query:"kubeprometheusstackhelmvaluesdocument" validate:"optional"`

	// Logging
	// The logging definition that belongs to this resource.
	LoggingDefinitionID *uint `json:"LoggingDefinitionID,omitempty" query:"loggingdefinitionid" validate:"optional"`

	// The version of the loki helm chart to use from the helm repo, e.g. 1.2.3
	LokiHelmChartVersion *string `json:"LokiHelmChartVersion,omitempty" query:"lokihelmchartversion" validate:"optional"`

	// Optional Helm workload definition values that can be provided to configure the
	// underlying loki chart.
	LokiHelmValuesDocument *string `json:"LokiHelmValuesDocument,omitempty" query:"lokihelmvaluesdocument" validate:"optional"`

	// The version of the promtail helm chart to use from the helm repo, e.g. 1.2.3
	PromtailHelmChartVersion *string `json:"PromtailHelmChartVersion,omitempty" query:"promtailhelmchartversion" validate:"optional"`

	// Optional Helm workload definition values that can be provided to configure the
	// underlying promtail chart.
	PromtailHelmValuesDocument *string `json:"PromtailHelmValuesDocument,omitempty" query:"promtailhelmvaluesdocument" validate:"optional"`

	// The associated observability stack instances that are deployed from this definition.
	ObservabilityStackInstances []*ObservabilityStackInstance `json:"ObservabilityStackInstances,omitempty" validate:"optional,association"`
}

// +threeport-codegen:reconciler
// ObservabilityStackInstances defines an instance of an observability stack.
type ObservabilityStackInstance struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Instance       `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// ObservabilityStackDefinitionID is the definition used to configure the workload instance.
	ObservabilityStackDefinitionID *uint `json:"ObservabilityStackDefinitionID,omitempty" query:"observabilitystackdefinitionid" gorm:"not null" validate:"required"`

	// The kubernetes runtime where the ingress layer is installed.
	KubernetesRuntimeInstanceID *uint `json:"KubernetesRuntimeInstanceID,omitempty" query:"kubernetesruntimeinstanceid" gorm:"not null" validate:"required"`

	// If true, metrics will be enabled for the observability stack.
	MetricsEnabled *bool `json:"MetricsEnabled,omitempty" query:"metricsenabled" gorm:"default:true" validate:"optional"`

	// If true, logging will be enabled for the observability stack.
	LoggingEnabled *bool `json:"LoggingEnabled,omitempty" query:"loggingenabled" gorm:"default:true" validate:"optional"`

	// Dashboard
	// The observability dashboard instance that belongs to this resource.
	ObservabilityDashboardInstanceID *uint `json:"ObservabilityDashboardInstanceID,omitempty" query:"observabilitydashboardinstanceid" validate:"optional"`

	// Optional Helm workload instance values that can be provided to configure the
	// underlying grafana chart.
	GrafanaHelmValuesDocument *string `json:"GrafanaHelmValuesDocument,omitempty" query:"grafanahelmvaluesdocument" validate:"optional"`

	// Metrics
	// The metrics instance that belongs to this resource.
	MetricsInstanceID *uint `json:"MetricsInstanceID,omitempty" query:"metricsinstanceid" validate:"optional"`

	// Optional Helm workload instance values that can be provided to configure the
	// underlying kube-prometheus-stack chart.
	KubePrometheusStackHelmValuesDocument *string `json:"KubePrometheusStackHelmValuesDocument,omitempty" query:"kubeprometheusstackhelmvaluesdocument" validate:"optional"`

	// Logging
	// The logging instance that belongs to this resource.
	LoggingInstanceID *uint `json:"LoggingInstanceID,omitempty" query:"logginginstanceid" validate:"optional"`

	// Optional Helm workload instance values that can be provided to configure the
	// underlying loki chart.
	LokiHelmValuesDocument *string `json:"LokiHelmValuesDocument,omitempty" query:"lokihelmvaluesdocument" validate:"optional"`

	// Optional Helm workload Instancehat can be provided to configure the
	// underlying promtail chart.
	PromtailHelmValuesDocument *string `json:"PromtailHelmValuesDocument,omitempty" query:"promtailhelmvaluesdocument" validate:"optional"`
}

// +threeport-codegen:reconciler
// ObservabilityDashboardDefinition defines an dashboard.
type ObservabilityDashboardDefinition struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Definition     `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The Grafana Helm workload definition that belongs to this resource.
	GrafanaHelmWorkloadDefinitionID *uint `json:"GrafanaHelmWorkloadDefinitionID,omitempty" query:"grafanahelmworkloaddefinitionid" validate:"optional"`

	// The version of the grafana helm chart to use from the helm repo, e.g. 1.2.3
	GrafanaHelmChartVersion *string `json:"GrafanaHelmChartVersion,omitempty" query:"grafanahelmchartversion" gorm:"default:'7.2.1'" validate:"optional"`

	// Optional Helm workload definition values that can be provided to configure the
	// underlying grafana chart.
	GrafanaHelmValuesDocument *string `json:"GrafanaHelmValuesDocument,omitempty" query:"grafanahelmvaluesdocument" validate:"optional"`

	// The associated observability dashboard instances that are deployed from this definition.
	ObservabilityDashboardInstances []*ObservabilityDashboardInstance `json:"ObservabilityDashboardInstances,omitempty" validate:"optional,association"`
}

// +threeport-codegen:reconciler
// ObservabilityDashboardInstances defines an instance of an observability dashboard.
type ObservabilityDashboardInstance struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Instance       `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// ObservabilityDashboardDefinitionID is the definition used to configure the workload instance.
	ObservabilityDashboardDefinitionID *uint `json:"ObservabilityDashboardDefinitionID,omitempty" query:"observabilitydashboarddefinitionid" gorm:"not null" validate:"required"`

	// The kubernetes runtime where the ingress layer is installed.
	KubernetesRuntimeInstanceID *uint `json:"KubernetesRuntimeInstanceID,omitempty" query:"kubernetesruntimeinstanceid" gorm:"not null" validate:"required"`

	// The Grafana Helm workload definition that belongs to this resource.
	GrafanaHelmWorkloadInstanceID *uint `json:"GrafanaHelmWorkloadInstanceID,omitempty" query:"grafanahelmworkloadinstanceid" validate:"optional"`

	// Optional Helm workload definition values that can be provided to configure the
	// underlying grafana chart.
	GrafanaHelmValues *string `json:"GrafanaHelmValues,omitempty" query:"grafanahelmvalues" validate:"optional"`
}

// +threeport-codegen:reconciler
// MetricsDefinition defines a metrics aggregation layer for a workload.
type MetricsDefinition struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Definition     `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The kube-prometheus-stack Helm workload definition that belongs to this resource.
	KubePrometheusStackHelmWorkloadDefinitionID *uint `json:"KubePrometheusStackHelmWorkloadDefinitionID,omitempty" query:"kubeprometheusstackhelmworkloaddefinitionid" validate:"optional"`

	// The version of the kube-prometheus-stack helm chart to use from the helm repo, e.g. 1.2.3
	KubePrometheusStackHelmChartVersion *string `json:"KubePrometheusStackHelmChartVersion,omitempty" query:"kubeprometheusstackhelmchartversion" gorm:"default:'55.8.1'" validate:"optional"`

	// Optional Helm workload definition values that can be provided to configure the
	// underlying kube-prometheus-stack chart.
	KubePrometheusStackHelmValuesDocument *string `json:"KubePrometheusStackHelmValuesDocument,omitempty" query:"kubeprometheusstackhelmvaluesdocument" validate:"optional"`

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

	// The kube-prometheus-stack Helm workload definition that belongs to this resource.
	KubePrometheusStackHelmWorkloadInstanceID *uint `json:"KubePrometheusStackHelmWorkloadInstanceID,omitempty" query:"kubeprometheusstackhelmworkloadinstanceid" validate:"optional"`

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

	// The loki Helm workload definition that belongs to this resource.
	LokiHelmWorkloadDefinitionID *uint `json:"LokiHelmWorkloadDefinitionID,omitempty" query:"lokihelmworkloaddefinitionid" validate:"optional"`

	// The promtail Helm workload definition that belongs to this resource.
	PromtailHelmWorkloadDefinitionID *uint `json:"PromtailHelmWorkloadDefinitionID,omitempty" query:"promtailhelmworkloaddefinitionid" validate:"optional"`

	// The version of the loki helm chart to use from the helm repo, e.g. 1.2.3
	LokiHelmChartVersion *string `json:"LokiHelmChartVersion,omitempty" query:"lokihelmchartversion" gorm:"default:'5.41.6'" validate:"optional"`

	// The version of the promtail helm chart to use from the helm repo, e.g. 1.2.3
	PromtailHelmChartVersion *string `json:"PromtailHelmChartVersion,omitempty" query:"promtailhelmchartversion" gorm:"default:'6.15.3'" validate:"optional"`

	// Optional Helm workload definition values that can be provided to configure the
	// underlying loki chart.
	LokiHelmValuesDocument *string `json:"LokiHelmValuesDocument,omitempty" query:"lokihelmvaluesdocument" validate:"optional"`

	// Optional Helm workload definition values that can be provided to configure the
	// underlying promtail chart.
	PromtailHelmValuesDocument *string `json:"PromtailHelmValuesDocument,omitempty" query:"promtailhelmvaluesdocument" validate:"optional"`

	// The associated metrics instances that are deployed from this definition.
	LoggingInstances []*LoggingInstance `json:"LoggingInstances,omitempty" validate:"optional,association"`
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

	// The loki Helm workload definition that belongs to this resource.
	LokiHelmWorkloadInstanceID *uint `json:"LokiHelmWorkloadInstanceID,omitempty" query:"lokihelmworkloadinstanceid" validate:"optional"`

	// The promtail Helm workload definition that belongs to this resource.
	PromtailHelmWorkloadInstanceID *uint `json:"PromtailHelmWorkloadInstanceID,omitempty" query:"promtailhelmworkloadinstanceid" validate:"optional"`

	// Optional Helm workload definition values that can be provided to configure the
	// underlying loki chart.
	LokiHelmValues *string `json:"LokiHelmValues,omitempty" query:"lokihelmvalues" validate:"optional"`

	// Optional Helm workload definition values that can be provided to configure the
	// underlying promtail chart.
	PromtailHelmValues *string `json:"PromtailHelmValues,omitempty" query:"promtailhelmvalues" validate:"optional"`
}
