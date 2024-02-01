package v0

import (
	"fmt"
	"net/http"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
)

type ObservabilityStack struct {
	ObservabilityStack *ObservabilityStackValues `yaml:"ObservabilityStack"`
}

// ObservabilityStackValues provides the configuration for an observability stack
type ObservabilityStackValues struct {
	Name                                  string                           `yaml:"Name"`
	KubernetesRuntimeInstance             *KubernetesRuntimeInstanceValues `yaml:"KubernetesRuntimeInstance"`
	MetricsEnabled                        bool                             `yaml:"MetricsEnabled"`
	LoggingEnabled                        bool                             `yaml:"LoggingEnabled"`
	GrafanaHelmValues                     string                           `yaml:"GrafanaHelmValues"`
	GrafanaHelmValuesDocument             string                           `yaml:"GrafanaHelmValuesDocument"`
	LokiHelmValues                        string                           `yaml:"LokiHelmValues"`
	LokiHelmValuesDocument                string                           `yaml:"LokiHelmValuesDocument"`
	PromtailHelmValues                    string                           `yaml:"PromtailHelmValues"`
	PromtailHelmValuesDocument            string                           `yaml:"PromtailHelmValuesDocument"`
	KubePrometheusStackHelmValues         string                           `yaml:"KubePrometheusStackHelmValues"`
	KubePrometheusStackHelmValuesDocument string                           `yaml:"KubePrometheusStackHelmValuesDocument"`
	ObservabilityStackConfigPath          string                           `yaml:"ObservabilityStackConfigPath"`
}

// Create creates an observability stack definition and instance
func (o *ObservabilityStackValues) Create(apiClient *http.Client, apiEndpoint string) error {
	// validate observability stack create values
	if err := o.ValidateCreate(); err != nil {
		return fmt.Errorf("failed to validate observability stack values: %w", err)
	}

	// get kubernetes runtime instance
	kri, err := client.GetKubernetesRuntimeInstanceByName(
		apiClient,
		apiEndpoint,
		o.KubernetesRuntimeInstance.Name,
	)
	if err != nil {
		return fmt.Errorf("failed to get kubernetes runtime instance: %w", err)
	}

	// create observability stack definition
	osd := &v0.ObservabilityStackDefinition{
		Definition: v0.Definition{
			Name: &o.Name,
		},
	}

	// set grafana helm values if present
	grafanaHelmValuesDocument, err := GetValuesFromDocumentOrInline(o.GrafanaHelmValues, o.GrafanaHelmValuesDocument, o.ObservabilityStackConfigPath)
	if err != nil {
		return fmt.Errorf("failed to get grafana values document from path: %w", err)
	}
	osd.GrafanaHelmValuesDocument = grafanaHelmValuesDocument

	// set loki helm values if present
	lokiHelmValuesDocument, err := GetValuesFromDocumentOrInline(o.LokiHelmValues, o.LokiHelmValuesDocument, o.ObservabilityStackConfigPath)
	if err != nil {
		return fmt.Errorf("failed to get loki values document from path: %w", err)
	}
	osd.LokiHelmValuesDocument = lokiHelmValuesDocument

	// set promtail helm values if present
	promtailHelmValuesDocument, err := GetValuesFromDocumentOrInline(o.PromtailHelmValues, o.PromtailHelmValuesDocument, o.ObservabilityStackConfigPath)
	if err != nil {
		return fmt.Errorf("failed to get promtail values document from path: %w", err)
	}
	osd.PromtailHelmValuesDocument = promtailHelmValuesDocument

	// set kube-prometheus-stack helm values if present
	kubePrometheusStackHelmValuesDocument, err := GetValuesFromDocumentOrInline(o.KubePrometheusStackHelmValues, o.KubePrometheusStackHelmValuesDocument, o.ObservabilityStackConfigPath)
	if err != nil {
		return fmt.Errorf("failed to get kube-prometheus-stack values document from path: %w", err)
	}
	osd.KubePrometheusStackHelmValuesDocument = kubePrometheusStackHelmValuesDocument

	// create observability stack definition
	createdOsd, err := client.CreateObservabilityStackDefinition(
		apiClient,
		apiEndpoint,
		osd,
	)
	if err != nil {
		return fmt.Errorf("failed to create observability stack definition: %w", err)
	}

	// create observability stack instance
	_, err = client.CreateObservabilityStackInstance(
		apiClient,
		apiEndpoint,
		&v0.ObservabilityStackInstance{
			Instance: v0.Instance{
				Name: &o.Name,
			},
			ObservabilityStackDefinitionID: createdOsd.ID,
			KubernetesRuntimeInstanceID:    kri.ID,
			MetricsEnabled:                 &o.MetricsEnabled,
			LoggingEnabled:                 &o.LoggingEnabled,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to create observability stack instance: %w", err)
	}

	return nil
}

// Delete deletes an observability stack definition and instance
func (o *ObservabilityStackValues) Delete(apiClient *http.Client, apiEndpoint string) error {
	// validate observability stack delete values
	if err := o.ValidateDelete(); err != nil {
		return fmt.Errorf("failed to validate observability stack values: %w", err)
	}

	// get observability stack definition
	osi, err := client.GetObservabilityStackInstanceByName(
		apiClient,
		apiEndpoint,
		o.Name,
	)
	if err != nil {
		return fmt.Errorf("failed to get observability stack definition: %w", err)
	}

	// delete observability stack instance
	_, err = client.DeleteObservabilityStackInstance(
		apiClient,
		apiEndpoint,
		*osi.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to delete observability stack instance: %w", err)
	}

	// delete observability stack definition
	_, err = client.DeleteObservabilityStackDefinition(
		apiClient,
		apiEndpoint,
		*osi.ObservabilityStackDefinitionID,
	)
	if err != nil {
		return fmt.Errorf("failed to delete observability stack definition: %w", err)
	}

	return nil
}

// ValidateCreate validates the observability stack values for creation
func (o *ObservabilityStackValues) ValidateCreate() error {
	if o.Name == "" {
		return fmt.Errorf("name is required")
	}
	if o.KubernetesRuntimeInstance == nil {
		return fmt.Errorf("KubernetesRuntimeInstance is required")
	}
	if o.KubernetesRuntimeInstance.Name == "" {
		return fmt.Errorf("KubernetesRuntimeInstance.Name is required")
	}
	return nil
}

// ValidateDelete validates the observability stack values for deletion
func (o *ObservabilityStackValues) ValidateDelete() error {
	if o.Name == "" {
		return fmt.Errorf("name is required")
	}
	return nil
}
