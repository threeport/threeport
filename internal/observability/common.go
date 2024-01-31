package observability

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v2"
	"helm.sh/helm/v3/pkg/cli/values"
)

const GrafanaHelmRepo = "https://grafana.github.io/helm-charts"
const PrometheusCommunityHelmRepo = "https://prometheus-community.github.io/helm-charts"

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

	temporaryFiles := map[string]string{
		"/tmp/values.yaml":          base,
		"/tmp/override-values.yaml": override,
	}

	var valueFiles []string
	// create temporary files in /tmp and populate valueFiles
	for path, file := range temporaryFiles {
		err := os.WriteFile(path, []byte(file), 0644)
		if err != nil {
			return "", fmt.Errorf("failed to write base helm values: %w", err)
		}
		valueFiles = append(valueFiles, path)
	}

	values := values.Options{
		ValueFiles: valueFiles,
	}
	grafanaGoValues, err := values.MergeValues(nil)
	if err != nil {
		return "", fmt.Errorf("failed to merge grafana helm values: %w", err)
	}
	grafanaByteValues, err := yaml.Marshal(grafanaGoValues)
	if err != nil {
		return "", fmt.Errorf("failed to marshal grafana helm values: %w", err)
	}

	// clean up temporary files
	for filePath := range temporaryFiles {
		err := os.Remove(filePath)
		if err != nil {
			return "", fmt.Errorf("failed to remove temporary file: %w", err)
		}
	}

	return string(grafanaByteValues), nil
}
