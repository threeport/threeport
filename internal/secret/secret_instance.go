package secret

import (
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
)

// secretInstanceCreated reconciles state for a new secret
// instance.
func secretInstanceCreated(
	r *controller.Reconciler,
	secretInstance *v0.SecretInstance,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}

// secretInstanceCreated reconciles state for a secret instance
// instance when it is changed.
func secretInstanceUpdated(
	r *controller.Reconciler,
	secretInstance *v0.SecretInstance,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}

// secretInstanceCreated reconciles state for a secret instance
// instance when it is removed.
func secretInstanceDeleted(
	r *controller.Reconciler,
	secretInstance *v0.SecretInstance,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}

// CreateExternalSecret returns a new ExternalSecret object
func CreateExternalSecret(name string) *unstructured.Unstructured {
	var externalSecret = &unstructured.Unstructured{
		Object: map[string]interface{}{
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
		},
	}
	return externalSecret
}
