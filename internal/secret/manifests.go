package secret

import (
	"fmt"

	runtime "github.com/threeport/threeport/internal/kubernetes-runtime"
	mapping "github.com/threeport/threeport/pkg/mapping/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
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

// getExternalSecretsSupportServiceYaml returns the external-secrets component.
func (c *SecretInstanceConfig) getExternalSecretsSupportServiceYaml() (string, error) {
	// build the secrets component the v0.9 operator accepts
	rendered, err := util.UnstructuredToYaml(
		&unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "platform.addons.nukleros.io/v1alpha1",
				"kind":       "SecretsComponent",
				"metadata": map[string]interface{}{
					"name":      "external-secrets",
					"namespace": "nukleros-secrets-system",
				},
				"spec": map[string]interface{}{
					"namespace": "nukleros-secrets-system",
					"externalSecrets": map[string]interface{}{
						"image":    "ghcr.io/external-secrets/external-secrets",
						"version":  "v0.9.11",
						"replicas": 1,
					},
				},
			},
		},
	)
	if err != nil {
		return "", err
	}
	return rendered, nil
}
