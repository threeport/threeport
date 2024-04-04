package v0

import (
	"fmt"
	"net/http"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// ObservabilityStackConfig contains the config for an observability stack which
// is an abstraction of an observability stack definition and instance.
type ObservabilityStackConfig struct {
	ObservabilityStack ObservabilityStackValues `yaml:"ObservabilityStack"`
}

// ObservabilityStackValues provides the configuration for an observability
// stack definition and instance.
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
	ObservabilityConfigPath               string                           `yaml:"ObservabilityConfigPath"`
}

// ObservabilityStackDefinitionConfig contains the config for an observability
// stack definition.
type ObservabilityStackDefinitionConfig struct {
	ObservabilityStackDefinition ObservabilityStackDefinitionValues `yaml:"ObservabilityStackDefinition"`
}

// ObservabilityStackDefinitionValues contains the attributes needed to manage
// an observability stack definition.
type ObservabilityStackDefinitionValues struct {
	Name                                  string `yaml:"Name"`
	GrafanaHelmValues                     string `yaml:"GrafanaHelmValues"`
	GrafanaHelmValuesDocument             string `yaml:"GrafanaHelmValuesDocument"`
	LokiHelmValues                        string `yaml:"LokiHelmValues"`
	LokiHelmValuesDocument                string `yaml:"LokiHelmValuesDocument"`
	PromtailHelmValues                    string `yaml:"PromtailHelmValues"`
	PromtailHelmValuesDocument            string `yaml:"PromtailHelmValuesDocument"`
	KubePrometheusStackHelmValues         string `yaml:"KubePrometheusStackHelmValues"`
	KubePrometheusStackHelmValuesDocument string `yaml:"KubePrometheusStackHelmValuesDocument"`
	ObservabilityConfigPath               string `yaml:"ObservabilityConfigPath"`
}

// ObservabilityStackInstanceConfig contains the config for an observability
// stack definition.
type ObservabilityStackInstanceConfig struct {
	ObservabilityStackInstance ObservabilityStackInstanceValues `yaml:"ObservabilityStackInstance"`
}

// ObservabilityStackInstanceValues contains the attributes needed to manage
// an observability stack definition.
type ObservabilityStackInstanceValues struct {
	Name                                  string                             `yaml:"Name"`
	KubernetesRuntimeInstance             *KubernetesRuntimeInstanceValues   `yaml:"KubernetesRuntimeInstance"`
	MetricsEnabled                        bool                               `yaml:"MetricsEnabled"`
	LoggingEnabled                        bool                               `yaml:"LoggingEnabled"`
	GrafanaHelmValues                     string                             `yaml:"GrafanaHelmValues"`
	GrafanaHelmValuesDocument             string                             `yaml:"GrafanaHelmValuesDocument"`
	LokiHelmValues                        string                             `yaml:"LokiHelmValues"`
	LokiHelmValuesDocument                string                             `yaml:"LokiHelmValuesDocument"`
	PromtailHelmValues                    string                             `yaml:"PromtailHelmValues"`
	PromtailHelmValuesDocument            string                             `yaml:"PromtailHelmValuesDocument"`
	KubePrometheusStackHelmValues         string                             `yaml:"KubePrometheusStackHelmValues"`
	KubePrometheusStackHelmValuesDocument string                             `yaml:"KubePrometheusStackHelmValuesDocument"`
	ObservabilityConfigPath               string                             `yaml:"ObservabilityConfigPath"`
	ObservabilityStackDefinition          ObservabilityStackDefinitionValues `yaml:"ObservabilityStackDefinition"`
}

func (o *ObservabilityStackValues) GetOperations(
	apiClient *http.Client,
	apiEndpoint string,
) (*util.Operations, *v0.ObservabilityStackDefinition, *v0.ObservabilityStackInstance) {
	var createdObservabilityStackDefinition v0.ObservabilityStackDefinition
	var createdObservabilityStackInstance v0.ObservabilityStackInstance

	operations := util.Operations{}

	observabilityStackDefinitionValues := ObservabilityStackDefinitionValues{
		Name: o.Name,
		GrafanaHelmValues: o.GrafanaHelmValues,
		GrafanaHelmValuesDocument: o.GrafanaHelmValuesDocument,
		LokiHelmValues: o.LokiHelmValues,
		LokiHelmValuesDocument: o.LokiHelmValuesDocument,
		PromtailHelmValues: o.PromtailHelmValues,
		PromtailHelmValuesDocument: o.PromtailHelmValuesDocument,
		KubePrometheusStackHelmValues: o.KubePrometheusStackHelmValues,
		KubePrometheusStackHelmValuesDocument: o.KubePrometheusStackHelmValuesDocument,
		ObservabilityConfigPath: o.ObservabilityConfigPath,
	}
	operations.AppendOperation(util.Operation{
		Name: "observability stack definition",
		Create: func() error {
			createdOsd, err := observabilityStackDefinitionValues.Create(apiClient, apiEndpoint)
			if err != nil {
				return fmt.Errorf("failed to create observability stack definition with name %s: %w", o.Name, err)
			}
			createdObservabilityStackDefinition = *createdOsd
			return nil
		},
		Delete: func() error {
			_, err := observabilityStackDefinitionValues.Delete(apiClient, apiEndpoint)
			if err != nil {
				return fmt.Errorf("failed to delete observability stack definition with name %s: %w", o.Name, err)
			}
			return nil
		},
	})

	observabilityStackInstanceValues := ObservabilityStackInstanceValues{
		Name: o.Name,
		KubernetesRuntimeInstance: o.KubernetesRuntimeInstance,
		MetricsEnabled: o.MetricsEnabled,
		LoggingEnabled: o.LoggingEnabled,
		GrafanaHelmValues: o.GrafanaHelmValues,
		GrafanaHelmValuesDocument: o.GrafanaHelmValuesDocument,
		LokiHelmValues: o.LokiHelmValues,
		LokiHelmValuesDocument: o.LokiHelmValuesDocument,
		PromtailHelmValues: o.PromtailHelmValues,
		PromtailHelmValuesDocument: o.PromtailHelmValuesDocument,
		KubePrometheusStackHelmValues: o.KubePrometheusStackHelmValues,
		KubePrometheusStackHelmValuesDocument: o.KubePrometheusStackHelmValuesDocument,
		ObservabilityConfigPath: o.ObservabilityConfigPath,
		ObservabilityStackDefinition: observabilityStackDefinitionValues,
	}
	operations.AppendOperation(util.Operation{
		Name: "observability stack instance",
		Create: func() error {
			createdOsi, err := observabilityStackInstanceValues.Create(apiClient, apiEndpoint)
			if err != nil {
				return fmt.Errorf("failed to create observability stack instance with name %s: %w", o.Name, err)
			}
			createdObservabilityStackInstance = *createdOsi
			return nil
		},
		Delete: func() error {
			deletedOsi, err := observabilityStackInstanceValues.Delete(apiClient, apiEndpoint)
			if err != nil {
				return fmt.Errorf("failed to delete observability stack instance with name %s: %w", o.Name, err)
			}
			// wait for observability stack instance to be deleted
			util.Retry(60, 1, func() error {
				if _, err := client.GetObservabilityStackInstanceByName(
					apiClient,
					apiEndpoint,
					*deletedOsi.Name,
				); err == nil {
					return fmt.Errorf(
						"observability stack instance %s still exists",
						*deletedOsi.Name,
					)
				}
				return nil
			})

			return nil
		},
	})

	return &operations, &createdObservabilityStackDefinition, &createdObservabilityStackInstance
}

// Create creates an observability stack definition and instance
func (o *ObservabilityStackValues) Create(
	apiClient *http.Client,
	apiEndpoint string,
) (*v0.ObservabilityStackDefinition, *v0.ObservabilityStackInstance, error) {
	// validate observability stack create values
	if err := o.ValidateCreate(); err != nil {
		return nil, nil, fmt.Errorf("failed to validate observability stack values: %w", err)
	}

	// get observability stack operations
	operations, observabilityStackDefinition, observabilityStackInstance := o.GetOperations(apiClient, apiEndpoint)

	// execute observability stack create operations
	if err := operations.Create(); err != nil {
		return nil, nil, fmt.Errorf(
			"failed to execute create operations for observability stack defined instance with name %s: %w",
			o.Name,
			err,
		)
	}

	return observabilityStackDefinition, observabilityStackInstance, nil
}

// Delete deletes an observability stack definition and instance
func (o *ObservabilityStackValues) Delete(
	apiClient *http.Client,
	apiEndpoint string,
) (*v0.ObservabilityStackDefinition, *v0.ObservabilityStackInstance, error) {
	// validate observability stack delete values
	if err := o.ValidateDelete(); err != nil {
		return nil, nil, fmt.Errorf("failed to validate observability stack values: %w", err)
	}

	// get observability stack operations
	operations, _, _ := o.GetOperations(apiClient, apiEndpoint)

	// execute observability stack delete operations
	if err := operations.Delete(); err != nil {
		return nil, nil, fmt.Errorf(
			"failed to execute delete operations for observability stack defined instance with name %s: %w",
			o.Name,
			err,
		)
	}

	return nil, nil, nil
}

// ValidateCreate validates the observability stack values for creation
func (o *ObservabilityStackValues) ValidateCreate() error {
	multiError := util.MultiError{}

	// ensure name is set
	if o.Name == "" {
		multiError.AppendError(fmt.Errorf("name is required"))
	}

	// ensure kubernetes runtime instance is set
	if o.KubernetesRuntimeInstance == nil {
		multiError.AppendError(fmt.Errorf("KubernetesRuntimeInstance is required"))
	}

	// ensure kubernetes runtime instance name is set
	if o.KubernetesRuntimeInstance.Name == "" {
		multiError.AppendError(fmt.Errorf("KubernetesRuntimeInstance.Name is required"))
	}

	// ensure grafana helm values and document are not both set
	if o.GrafanaHelmValues != "" && o.GrafanaHelmValuesDocument != "" {
		multiError.AppendError(fmt.Errorf("GrafanaHelmValues and GrafanaHelmValuesDocument cannot both be set"))
	}

	// ensure loki helm values and document are not both set
	if o.LokiHelmValues != "" && o.LokiHelmValuesDocument != "" {
		multiError.AppendError(fmt.Errorf("LokiHelmValues and LokiHelmValuesDocument cannot both be set"))
	}

	// ensure promtail helm values and document are not both set
	if o.PromtailHelmValues != "" && o.PromtailHelmValuesDocument != "" {
		multiError.AppendError(fmt.Errorf("PromtailHelmValues and PromtailHelmValuesDocument cannot both be set"))
	}

	// ensure kube-prometheus-stack helm values and document are not both set
	if o.KubePrometheusStackHelmValues != "" && o.KubePrometheusStackHelmValuesDocument != "" {
		multiError.AppendError(fmt.Errorf("KubePrometheusStackHelmValues and KubePrometheusStackHelmValuesDocument cannot both be set"))
	}

	return multiError.Error()
}

// ValidateDelete validates the observability stack values for deletion
func (o *ObservabilityStackValues) ValidateDelete() error {
	if o.Name == "" {
		return fmt.Errorf("name is required")
	}
	return nil
}

// Create creates an observability stack definition.
func (o *ObservabilityStackDefinitionValues) Create(
	apiClient *http.Client,
	apiEndpoint string,
) (*v0.ObservabilityStackDefinition, error) {
	// validate observability stack create values
	if err := o.ValidateCreate(); err != nil {
		return nil, fmt.Errorf("failed to validate observability stack values: %w", err)
	}

	// construct observability stack definition object
	osd := &v0.ObservabilityStackDefinition{
		Definition: v0.Definition{
			Name: &o.Name,
		},
	}

	// set grafana helm values if present
	grafanaHelmValuesDocument, err := GetValuesFromDocumentOrInline(
		o.GrafanaHelmValues,
		o.GrafanaHelmValuesDocument,
		o.ObservabilityConfigPath,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get grafana values document from path: %w", err)
	}
	osd.GrafanaHelmValuesDocument = grafanaHelmValuesDocument

	// set loki helm values if present
	lokiHelmValuesDocument, err := GetValuesFromDocumentOrInline(
		o.LokiHelmValues,
		o.LokiHelmValuesDocument,
		o.ObservabilityConfigPath,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get loki values document from path: %w", err)
	}
	osd.LokiHelmValuesDocument = lokiHelmValuesDocument

	// set promtail helm values if present
	promtailHelmValuesDocument, err := GetValuesFromDocumentOrInline(
		o.PromtailHelmValues,
		o.PromtailHelmValuesDocument,
		o.ObservabilityConfigPath,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get promtail values document from path: %w", err)
	}
	osd.PromtailHelmValuesDocument = promtailHelmValuesDocument

	// set kube-prometheus-stack helm values if present
	kubePrometheusStackHelmValuesDocument, err := GetValuesFromDocumentOrInline(
		o.KubePrometheusStackHelmValues,
		o.KubePrometheusStackHelmValuesDocument,
		o.ObservabilityConfigPath,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get kube-prometheus-stack values document from path: %w", err)
	}
	osd.KubePrometheusStackHelmValuesDocument = kubePrometheusStackHelmValuesDocument

	// create observability stack definition
	createdOsd, err := client.CreateObservabilityStackDefinition(
		apiClient,
		apiEndpoint,
		osd,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create observability stack definition: %w", err)
	}

	return createdOsd, nil
}

// Delete deletes an observability stack definition.
func (o *ObservabilityStackDefinitionValues) Delete(
	apiClient *http.Client,
	apiEndpoint string,
) (*v0.ObservabilityStackDefinition, error) {
	// validate observability stack definition delete values
	if err := o.ValidateDelete(); err != nil {
		return nil, fmt.Errorf("failed to validate observability stack values: %w", err)
	}

	// get observability stack definition
	osd, err := client.GetObservabilityStackDefinitionByName(
		apiClient,
		apiEndpoint,
		o.Name,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get observability stack definition: %w", err)
	}

	// delete observability stack definition
	deletedOsd, err := client.DeleteObservabilityStackDefinition(
		apiClient,
		apiEndpoint,
		*osd.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to delete observability stack instance: %w", err)
	}

	return deletedOsd, nil
}

// ValidateCreate validates the observability stack definition values for creation
func (o *ObservabilityStackDefinitionValues) ValidateCreate() error {
	multiError := util.MultiError{}

	// ensure name is set
	if o.Name == "" {
		multiError.AppendError(fmt.Errorf("name is required"))
	}

	// ensure grafana helm values and document are not both set
	if o.GrafanaHelmValues != "" && o.GrafanaHelmValuesDocument != "" {
		multiError.AppendError(fmt.Errorf("GrafanaHelmValues and GrafanaHelmValuesDocument cannot both be set"))
	}

	// ensure loki helm values and document are not both set
	if o.LokiHelmValues != "" && o.LokiHelmValuesDocument != "" {
		multiError.AppendError(fmt.Errorf("LokiHelmValues and LokiHelmValuesDocument cannot both be set"))
	}

	// ensure promtail helm values and document are not both set
	if o.PromtailHelmValues != "" && o.PromtailHelmValuesDocument != "" {
		multiError.AppendError(fmt.Errorf("PromtailHelmValues and PromtailHelmValuesDocument cannot both be set"))
	}

	// ensure kube-prometheus-stack helm values and document are not both set
	if o.KubePrometheusStackHelmValues != "" && o.KubePrometheusStackHelmValuesDocument != "" {
		multiError.AppendError(fmt.Errorf("KubePrometheusStackHelmValues and KubePrometheusStackHelmValuesDocument cannot both be set"))
	}

	return multiError.Error()
}

// ValidateDelete validates the observability stack definition values for deletion
func (o *ObservabilityStackDefinitionValues) ValidateDelete() error {
	if o.Name == "" {
		return fmt.Errorf("name is required")
	}
	return nil
}

// Create creates an observability stack instance.
func (o *ObservabilityStackInstanceValues) Create(
	apiClient *http.Client,
	apiEndpoint string,
) (*v0.ObservabilityStackInstance, error) {
	// validate observability stack create values
	if err := o.ValidateCreate(); err != nil {
		return nil, fmt.Errorf("failed to validate observability stack values: %w", err)
	}

	// get kubernetes runtime instance
	kri, err := client.GetKubernetesRuntimeInstanceByName(
		apiClient,
		apiEndpoint,
		o.KubernetesRuntimeInstance.Name,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get kubernetes runtime instance: %w", err)
	}

	osd, err := client.GetObservabilityStackDefinitionByName(
		apiClient,
		apiEndpoint,
		o.ObservabilityStackDefinition.Name,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to get observability stack definition by name %s: %w",
			o.ObservabilityStackDefinition.Name,
			err,
		)
	}

	// construct observability stack instance object
	osi := &v0.ObservabilityStackInstance{
		Instance: v0.Instance{
			Name: &o.Name,
		},
		ObservabilityStackDefinitionID: osd.ID,
		KubernetesRuntimeInstanceID:    kri.ID,
		MetricsEnabled:                 &o.MetricsEnabled,
		LoggingEnabled:                 &o.LoggingEnabled,
	}

	// set grafana helm values if present
	grafanaHelmValuesDocument, err := GetValuesFromDocumentOrInline(
		o.GrafanaHelmValues,
		o.GrafanaHelmValuesDocument,
		o.ObservabilityConfigPath,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get grafana values document from path: %w", err)
	}
	osi.GrafanaHelmValuesDocument = grafanaHelmValuesDocument

	// set loki helm values if present
	lokiHelmValuesDocument, err := GetValuesFromDocumentOrInline(
		o.LokiHelmValues,
		o.LokiHelmValuesDocument,
		o.ObservabilityConfigPath,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get loki values document from path: %w", err)
	}
	osi.LokiHelmValuesDocument = lokiHelmValuesDocument

	// set promtail helm values if present
	promtailHelmValuesDocument, err := GetValuesFromDocumentOrInline(
		o.PromtailHelmValues,
		o.PromtailHelmValuesDocument,
		o.ObservabilityConfigPath,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get promtail values document from path: %w", err)
	}
	osi.PromtailHelmValuesDocument = promtailHelmValuesDocument

	// set kube-prometheus-stack helm values if present
	kubePrometheusStackHelmValuesDocument, err := GetValuesFromDocumentOrInline(
		o.KubePrometheusStackHelmValues,
		o.KubePrometheusStackHelmValuesDocument,
		o.ObservabilityConfigPath,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get kube-prometheus-stack values document from path: %w", err)
	}
	osi.KubePrometheusStackHelmValuesDocument = kubePrometheusStackHelmValuesDocument

	// create observability stack instance
	createdOsi, err := client.CreateObservabilityStackInstance(
		apiClient,
		apiEndpoint,
		osi,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create observability stack instance: %w", err)
	}

	return createdOsi, nil
}

// Delete deletes an observability stack instance.
func (o *ObservabilityStackInstanceValues) Delete(
	apiClient *http.Client,
	apiEndpoint string,
) (*v0.ObservabilityStackInstance, error) {
	// validate observability stack instance delete values
	if err := o.ValidateDelete(); err != nil {
		return nil, fmt.Errorf("failed to validate observability stack values: %w", err)
	}

	// get observability stack instance
	osi, err := client.GetObservabilityStackInstanceByName(
		apiClient,
		apiEndpoint,
		o.Name,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get observability stack instance: %w", err)
	}

	// delete observability stack instance
	deletedOsi, err := client.DeleteObservabilityStackInstance(
		apiClient,
		apiEndpoint,
		*osi.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to delete observability stack instance: %w", err)
	}

	return deletedOsi, nil
}

// ValidateCreate validates the observability stack instance values for creation
func (o *ObservabilityStackInstanceValues) ValidateCreate() error {
	multiError := util.MultiError{}

	// ensure name is set
	if o.Name == "" {
		multiError.AppendError(fmt.Errorf("name is required"))
	}

	// ensure kubernetes runtime instance is set
	if o.KubernetesRuntimeInstance == nil {
		multiError.AppendError(fmt.Errorf("KubernetesRuntimeInstance is required"))
	}

	// ensure kubernetes runtime instance name is set
	if o.KubernetesRuntimeInstance != nil && o.KubernetesRuntimeInstance.Name == "" {
		multiError.AppendError(fmt.Errorf("KubernetesRuntimeInstance.Name is required"))
	}

	// ensure grafana helm values and document are not both set
	if o.GrafanaHelmValues != "" && o.GrafanaHelmValuesDocument != "" {
		multiError.AppendError(fmt.Errorf("GrafanaHelmValues and GrafanaHelmValuesDocument cannot both be set"))
	}

	// ensure loki helm values and document are not both set
	if o.LokiHelmValues != "" && o.LokiHelmValuesDocument != "" {
		multiError.AppendError(fmt.Errorf("LokiHelmValues and LokiHelmValuesDocument cannot both be set"))
	}

	// ensure promtail helm values and document are not both set
	if o.PromtailHelmValues != "" && o.PromtailHelmValuesDocument != "" {
		multiError.AppendError(fmt.Errorf("PromtailHelmValues and PromtailHelmValuesDocument cannot both be set"))
	}

	// ensure kube-prometheus-stack helm values and document are not both set
	if o.KubePrometheusStackHelmValues != "" && o.KubePrometheusStackHelmValuesDocument != "" {
		multiError.AppendError(fmt.Errorf("KubePrometheusStackHelmValues and KubePrometheusStackHelmValuesDocument cannot both be set"))
	}

	return multiError.Error()
}

// ValidateDelete validates the observability stack instance values for deletion
func (o *ObservabilityStackInstanceValues) ValidateDelete() error {
	if o.Name == "" {
		return fmt.Errorf("name is required")
	}
	return nil
}
