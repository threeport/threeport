package secret

// getExternalSecret returns a new ExternalSecret object
func getExternalSecret(name string) map[string]interface{} {
	return map[string]interface{}{
		"apiVersion": "external-secrets.io/v1beta1",
		"kind":       "ExternalSecret",
		"metadata": map[string]interface{}{
			"name": name,
		},
		"spec": map[string]interface{}{
			"refreshInterval": "1h",
			"secretStoreRef": map[string]interface{}{
				"name": "aws-secretsmanager",
				"kind": "SecretStore",
			},
			"target": map[string]interface{}{
				"name":           name,
				"creationPolicy": "Owner",
			},
			"dataFrom": []interface{}{
				map[string]interface{}{
					"extract": map[string]interface{}{
						"key": name,
					},
				},
			},
		},
	}
}

// getSecretStore returns a new SecretStore object
func getSecretStore() map[string]interface{} {
	return map[string]interface{}{
		"apiVersion": "external-secrets.io/v1beta1",
		"kind":       "SecretStore",
		"metadata": map[string]interface{}{
			"name": "aws-secretsmanager",
		},
		"spec": map[string]interface{}{
			"provider": map[string]interface{}{
				"aws": map[string]interface{}{
					"service": "SecretsManager",
					// define a specific role to limit access
					// to certain secrets.
					// role is a optional field that
					// can be omitted for test purposes
					// role: arn:aws:iam::123456789012:role/external-secrets
					"region": "us-east-1",
					"auth": map[string]interface{}{
						"secretRef": map[string]interface{}{
							"accessKeyIDSecretRef": map[string]interface{}{
								"name": "awssm",
								"key":  "access-key",
							},
							"secretAccessKeySecretRef": map[string]interface{}{
								"name": "awssm",
								"key":  "secret-access-key",
							},
						},
					},
				},
			},
		},
	}
}

// getExternalSecretsSupportServiceManifest returns a new ExternalSecrets object
func getExternalSecretsSupportServiceManifest() map[string]interface{} {
	return map[string]interface{}{
		"apiVersion": "secrets.support-services.nukleros.io/v1alpha1",
		"kind":       "ExternalSecrets",
		"metadata": map[string]interface{}{
			"name": "externalsecrets-sample",
		},
		"spec": map[string]interface{}{
			//collection:
			//name: "supportservices-sample"
			//namespace: ""
			"namespace": "nukleros-secrets-system",
			"version":   "v0.9.11",
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
