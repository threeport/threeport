package observability

import (
	"fmt"

	"gopkg.in/yaml.v2"
	"helm.sh/helm/v3/pkg/cli/values"
)

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

// MergeHelmValues merges two helm values documents.
func MergeHelmValues(base, override string) (string, error) {

	// var settings = cli.New()
	// p := getter.All(settings)
	values := values.Options{
		JSONValues: []string{
			base,
			override,
		},
	}
	grafanaGoValues, err := values.MergeValues(nil)
	if err != nil {
		return "", fmt.Errorf("failed to merge grafana helm values: %w", err)
	}
	grafanaByteValues, err := yaml.Marshal(grafanaGoValues)
	if err != nil {
		return "", fmt.Errorf("failed to marshal grafana helm values: %w", err)
	}

	return string(grafanaByteValues), nil
}
