package v0

// ObservabilityStackDefinition defines an observability stack.
type ObservabilityStackDefinition struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Definition     `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// Dashboard
	// The observability dashboard definition that belongs to this resource.
	ObservabilityDashboardDefinitionID *uint `json:",omitempty" validate:"optional" relationship:"owns"`

	// The version of the grafana helm chart to use from the helm repo, e.g. 1.2.3
	GrafanaHelmChartVersion *string `json:",omitempty" validate:"optional"`

	// Optional Helm workload definition values that can be provided to configure the
	// underlying grafana chart.
	GrafanaHelmValuesDocument *string `json:",omitempty" validate:"optional"`

	// Metrics
	// The metrics definition that belongs to this resource.
	MetricsDefinitionID *uint `json:",omitempty" validate:"optional" relationship:"owns"`

	// The version of the kube-prometheus-stack helm chart to use from the helm repo, e.g. 1.2.3
	KubePrometheusStackHelmChartVersion *string `json:",omitempty" validate:"optional"`

	// Optional Helm workload definition values that can be provided to configure the
	// underlying kube-prometheus-stack chart.
	KubePrometheusStackHelmValuesDocument *string `json:",omitempty" validate:"optional"`

	// Logging
	// The logging definition that belongs to this resource.
	LoggingDefinitionID *uint `json:",omitempty" validate:"optional" relationship:"owns"`

	// The version of the loki helm chart to use from the helm repo, e.g. 1.2.3
	LokiHelmChartVersion *string `json:",omitempty" validate:"optional"`

	// Optional Helm workload definition values that can be provided to configure the
	// underlying loki chart.
	LokiHelmValuesDocument *string `json:",omitempty" validate:"optional"`

	// The version of the promtail helm chart to use from the helm repo, e.g. 1.2.3
	PromtailHelmChartVersion *string `json:",omitempty" validate:"optional"`

	// Optional Helm workload definition values that can be provided to configure the
	// underlying promtail chart.
	PromtailHelmValuesDocument *string `json:",omitempty" validate:"optional"`

	// The associated observability stack instances that are deployed from this definition.
	ObservabilityStackInstances []*ObservabilityStackInstance `json:",omitempty" validate:"optional,association"`
}

// ObservabilityStackInstance is a deployed instance of an observability stack.
type ObservabilityStackInstance struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Instance       `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The observability stack definition that belongs to this resource.
	ObservabilityStackDefinitionID *uint `json:",omitempty" validate:"required" gorm:"not null" relationship:"requires"`

	// The kubernetes runtime where the observability stack is installed.
	KubernetesRuntimeInstanceID *uint `json:",omitempty" validate:"required" gorm:"not null" relationship:"requires"`

	// If true, metrics will be enabled for the observability stack.
	MetricsEnabled *bool `json:",omitempty" validate:"optional" gorm:"default:true"`

	// If true, logging will be enabled for the observability stack.
	LoggingEnabled *bool `json:",omitempty" validate:"optional" gorm:"default:true"`

	// Dashboard
	// The observability dashboard instance that belongs to this resource.
	ObservabilityDashboardInstanceID *uint `json:",omitempty" validate:"optional" relationship:"owns"`

	// Optional Helm workload instance values that can be provided to configure the
	// underlying grafana chart.
	GrafanaHelmValuesDocument *string `json:",omitempty" validate:"optional"`

	// Metrics
	// The metrics instance that belongs to this resource.
	MetricsInstanceID *uint `json:",omitempty" validate:"optional" relationship:"owns"`

	// Optional Helm workload instance values that can be provided to configure the
	// underlying kube-prometheus-stack chart.
	KubePrometheusStackHelmValuesDocument *string `json:",omitempty" validate:"optional"`

	// Logging
	// The logging instance that belongs to this resource.
	LoggingInstanceID *uint `json:",omitempty" validate:"optional" relationship:"owns"`

	// Optional Helm workload instance values that can be provided to configure the
	// underlying loki chart.
	LokiHelmValuesDocument *string `json:",omitempty" validate:"optional"`

	// Optional Helm workload instance values that can be provided to configure the
	// underlying promtail chart.
	PromtailHelmValuesDocument *string `json:",omitempty" validate:"optional"`
}

// ObservabilityDashboardDefinition is the definition of an observability dashboard.
type ObservabilityDashboardDefinition struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Definition     `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The Grafana Helm workload definition that belongs to this resource.
	GrafanaHelmWorkloadDefinitionID *uint `json:",omitempty" validate:"optional" relationship:"owns;type:HelmWorkloadDefinition"`

	// The version of the grafana helm chart to use from the helm repo, e.g. 1.2.3
	GrafanaHelmChartVersion *string `json:",omitempty" validate:"optional" gorm:"default:'7.2.1'"`

	// Optional Helm workload definition values that can be provided to configure the
	// underlying grafana chart.
	GrafanaHelmValuesDocument *string `json:",omitempty" validate:"optional"`

	// The associated observability dashboard instances that are deployed from this definition.
	ObservabilityDashboardInstances []*ObservabilityDashboardInstance `json:",omitempty" validate:"optional,association"`
}

// ObservabilityDashboardInstances is a deployed instance of an observability dashboard.
type ObservabilityDashboardInstance struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Instance       `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The observability dashboard definition that belongs to this resource.
	ObservabilityDashboardDefinitionID *uint `json:",omitempty" validate:"required" gorm:"not null" relationship:"requires"`

	// The kubernetes runtime where the observability dashboard is installed.
	KubernetesRuntimeInstanceID *uint `json:",omitempty" validate:"required" gorm:"not null" relationship:"requires"`

	// The Grafana Helm workload instance that belongs to this resource.
	GrafanaHelmWorkloadInstanceID *uint `json:",omitempty" validate:"optional" relationship:"owns;type:HelmWorkloadInstance"`

	// Optional Helm workload definition values that can be provided to configure the
	// underlying grafana chart.
	GrafanaHelmValuesDocument *string `json:",omitempty" validate:"optional"`
}

// MetricsDefinition is the definition of a metrics aggregation layer for a workload.
type MetricsDefinition struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Definition     `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The kube-prometheus-stack Helm workload definition that belongs to this resource.
	KubePrometheusStackHelmWorkloadDefinitionID *uint `json:",omitempty" validate:"optional" relationship:"owns;type:HelmWorkloadDefinition"`

	// The version of the kube-prometheus-stack helm chart to use from the helm repo, e.g. 1.2.3
	KubePrometheusStackHelmChartVersion *string `json:",omitempty" validate:"optional" gorm:"default:'55.8.1'"`

	// Optional Helm workload definition values that can be provided to configure the
	// underlying kube-prometheus-stack chart.
	KubePrometheusStackHelmValuesDocument *string `json:",omitempty" validate:"optional"`

	// The associated metrics instances that are deployed from this definition.
	MetricsInstances []*MetricsInstance `json:",omitempty" validate:"optional,association"`
}

// MetricsInstances is a deployed instance of a metrics aggregation layer for a workload.
type MetricsInstance struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Instance       `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The metrics definition that belongs to this resource.
	MetricsDefinitionID *uint `json:",omitempty" validate:"required" gorm:"not null" relationship:"requires"`

	// The kubernetes runtime where the metrics is installed.
	KubernetesRuntimeInstanceID *uint `json:",omitempty" validate:"required" gorm:"not null" relationship:"requires"`

	// The kube-prometheus-stack helm workload instance that belongs to this resource.
	KubePrometheusStackHelmWorkloadInstanceID *uint `json:",omitempty" validate:"optional" relationship:"owns;type:HelmWorkloadInstance"`

	// Optional Helm workload instance values that can be provided to configure the
	// underlying kube-prometheus-stack chart.
	KubePrometheusStackHelmValuesDocument *string `json:",omitempty" validate:"optional"`
}

// LoggingDefinition is the definition of a logging implementation for a workload.
type LoggingDefinition struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Definition     `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The loki Helm workload definition that belongs to this resource.
	LokiHelmWorkloadDefinitionID *uint `json:",omitempty" validate:"optional" relationship:"owns;type:HelmWorkloadDefinition"`

	// The promtail Helm workload definition that belongs to this resource.
	PromtailHelmWorkloadDefinitionID *uint `json:",omitempty" validate:"optional" relationship:"owns;type:HelmWorkloadDefinition"`

	// The version of the loki helm chart to use from the helm repo, e.g. 1.2.3
	LokiHelmChartVersion *string `json:",omitempty" validate:"optional" gorm:"default:'5.41.6'"`

	// The version of the promtail helm chart to use from the helm repo, e.g. 1.2.3
	PromtailHelmChartVersion *string `json:",omitempty" validate:"optional" gorm:"default:'6.15.3'"`

	// Optional Helm workload definition values that can be provided to configure the
	// underlying loki chart.
	LokiHelmValuesDocument *string `json:",omitempty" validate:"optional"`

	// Optional Helm workload definition values that can be provided to configure the
	// underlying promtail chart.
	PromtailHelmValuesDocument *string `json:",omitempty" validate:"optional"`

	// The associated logging instances that are deployed from this definition.
	LoggingInstances []*LoggingInstance `json:",omitempty" validate:"optional,association"`
}

// LoggingInstances is a deployed instance of a logging implementation for a workload.
type LoggingInstance struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Instance       `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The logging definition that belongs to this resource.
	LoggingDefinitionID *uint `json:",omitempty" validate:"required" gorm:"not null" relationship:"requires"`

	// The kubernetes runtime where the logging is installed.
	KubernetesRuntimeInstanceID *uint `json:",omitempty" validate:"required" gorm:"not null" relationship:"requires"`

	// The loki helm workload instance that belongs to this resource.
	LokiHelmWorkloadInstanceID *uint `json:",omitempty" validate:"optional" relationship:"owns;type:HelmWorkloadInstance"`

	// The promtail helm workload instance that belongs to this resource.
	PromtailHelmWorkloadInstanceID *uint `json:",omitempty" validate:"optional" relationship:"owns;type:HelmWorkloadInstance"`

	// Optional Helm workload instance values that can be provided to configure the
	// underlying loki chart.
	LokiHelmValuesDocument *string `json:",omitempty" validate:"optional"`

	// Optional Helm workload instance values that can be provided to configure the
	// underlying promtail chart.
	PromtailHelmValuesDocument *string `json:",omitempty" validate:"optional"`
}
