package secret

import (
	"fmt"

	runtime "github.com/threeport/threeport/internal/kubernetes-runtime"
	mapping "github.com/threeport/threeport/pkg/mapping/v0"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// getExternalSecret returns a new ExternalSecret object
func (c *SecretInstanceConfig) getExternalSecret() *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "external-secrets.io/v1beta1",
			"kind":       "ExternalSecret",
			"metadata": map[string]interface{}{
				"name": *c.secretInstance.Name,
			},
			"spec": map[string]interface{}{
				"refreshInterval": "1h",
				"secretStoreRef": map[string]interface{}{
					"name": *c.secretInstance.Name,
					"kind": "SecretStore",
				},
				"target": map[string]interface{}{
					"name":           *c.secretInstance.Name,
					"creationPolicy": "Owner",
				},
				"dataFrom": []interface{}{
					map[string]interface{}{
						"extract": map[string]interface{}{
							"key": *c.secretDefinition.Name,
						},
					},
				},
			},
		},
	}
}

// getSecretStore returns a new SecretStore object
func (c *SecretInstanceConfig) getSecretStore() (*unstructured.Unstructured, error) {
	provider, err := runtime.GetCloudProviderForInfraProvider(*c.kubernetesRuntimeDefinition.InfraProvider)
	if err != nil {
		return nil, fmt.Errorf("failed to get cloud provider for infra provider: %w", err)
	}
	region, err := mapping.GetProviderRegionForLocation(provider, *c.kubernetesRuntimeInstance.Location)
	if err != nil {
		return nil, fmt.Errorf("failed to get provider region for location: %w", err)
	}
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "external-secrets.io/v1beta1",
			"kind":       "SecretStore",
			"metadata": map[string]interface{}{
				"name": *c.secretInstance.Name,
			},
			"spec": map[string]interface{}{
				"provider": map[string]interface{}{
					"aws": map[string]interface{}{
						"service": "SecretsManager",
						"region":  region,
					},
				},
			},
		},
	}, nil
}
