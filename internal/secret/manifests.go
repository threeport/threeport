package secret

import (
	"fmt"

	runtime "github.com/threeport/threeport/internal/kubernetes-runtime"
	"github.com/threeport/threeport/internal/kubernetes-runtime/mapping"
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

// getExternalSecretsSupportService returns a new ExternalSecrets object
func (c *SecretInstanceConfig) getExternalSecretsSupportServiceYaml(iamRoleArn string) (string, error) {
	return util.UnstructuredToYaml(
		&unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "secrets.support-services.nukleros.io/v1alpha1",
				"kind":       "ExternalSecrets",
				"metadata": map[string]interface{}{
					"name": "externalsecrets",
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
			},
		},
	)
}
