package observability

import (
	"fmt"
)

const GrafanaHelmRepo = "https://grafana.github.io/helm-charts"
const PrometheusCommunityHelmRepo = "https://prometheus-community.github.io/helm-charts"

// ObservabilityDashboardName returns the name of an observability dashboard object
func ObservabilityDashboardName(name string) string {
	return fmt.Sprintf("%s-observability-dashboard", name)
}

// MetricsName returns the name of a metrics object
func MetricsName(name string) string {
	return fmt.Sprintf("%s-metrics", name)
}

// LoggingName returns the name of a logging chart
func LoggingName(name string) string {
	return fmt.Sprintf("%s-logging", name)
}

// GrafanaChartName returns the name of the grafana chart
func GrafanaChartName(name string) string {
	return fmt.Sprintf("%s-grafana", name)
}

// KubePrometheusStackChartName returns the name of the kube-prometheus-stack
// chart
func KubePrometheusStackChartName(name string) string {
	return fmt.Sprintf("%s-kube-prometheus-stack", name)
}

// LokiHelmChartName returns the name of the loki chart
func LokiHelmChartName(name string) string {
	return fmt.Sprintf("%s-loki", name)
}

// PromtailHelmChartName returns the name of the promtail chart
func PromtailHelmChartName(name string) string {
	return fmt.Sprintf("%s-promtail", name)
}
