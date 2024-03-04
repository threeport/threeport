package secret

import (
	"fmt"

	"github.com/threeport/threeport/internal/kubernetes-runtime/mapping"
)

// getExternalSecret returns a new ExternalSecret object
func (c *SecretInstanceConfig) getExternalSecret() map[string]interface{} {
	return map[string]interface{}{
		"apiVersion": "external-secrets.io/v1beta1",
		"kind":       "ExternalSecret",
		"metadata": map[string]interface{}{
			"name": c.secretInstance.Name,
		},
		"spec": map[string]interface{}{
			"refreshInterval": "1h",
			"secretStoreRef": map[string]interface{}{
				"name": c.secretInstance.Name,
				"kind": "SecretStore",
			},
			"target": map[string]interface{}{
				"name":           c.secretInstance.Name,
				"creationPolicy": "Owner",
			},
			"dataFrom": []interface{}{
				map[string]interface{}{
					"extract": map[string]interface{}{
						"key": c.secretDefinition.Name,
					},
				},
			},
		},
	}
}

// getSecretStore returns a new SecretStore object
func (c *SecretInstanceConfig) getSecretStore() (map[string]interface{}, error) {
	region, err := mapping.GetProviderRegionForLocation("aws", *c.kubernetesRuntimeInstance.Location)
	// region, err := mapping.GetProviderRegionForLocation(*c.kubernetesRuntimeDefinition.InfraProvider, *c.kubernetesRuntimeInstance.Location)
	if err != nil {
		return nil, fmt.Errorf("failed to get provider region for location: %w", err)
	}
	return map[string]interface{}{
		"apiVersion": "external-secrets.io/v1beta1",
		"kind":       "SecretStore",
		"metadata": map[string]interface{}{
			"name": c.secretInstance.Name,
		},
		"spec": map[string]interface{}{
			"provider": map[string]interface{}{
				"aws": map[string]interface{}{
					"service": "SecretsManager",
					"region":  region,
				},
			},
		},
	}, nil
}

// getExternalSecretsSupportServiceManifest returns a new ExternalSecrets object
func (c *SecretInstanceConfig) getExternalSecretsSupportServiceManifest(iamRoleArn string) map[string]interface{} {
	return map[string]interface{}{
		"apiVersion": "secrets.support-services.nukleros.io/v1alpha1",
		"kind":       "ExternalSecrets",
		"metadata": map[string]interface{}{
			"name": c.secretInstance.Name,
		},
		"spec": map[string]interface{}{
			//collection:
			//name: "supportservices"
			//namespace: ""
			"namespace":  "nukleros-secrets-system",
			"version":    "v0.9.11",
			"iamRoleArn": iamRoleArn,
			"certController": map[string]interface{}{
				"replicas": 1,
			},
			"image": "ghcr.io/external-secrets/external-secrets",
			"controller": map[string]interface{}{
				"replicas": 1,
			},
			"webhook": map[string]interface{}{
				"replicas": 1,
			},
		},
	}
}
