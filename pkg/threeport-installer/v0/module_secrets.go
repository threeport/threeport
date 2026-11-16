package v0

import (
	"context"
	"fmt"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// CopySecret copies a secret of the control plane's namespace into a module's
// namespace.  A copy that already exists is refreshed, so a module installed
// again after the control plane's settings change, such as when the message
// broker began requiring TLS and a password, gets the current ones.
func CopySecret(
	dynamicClient dynamic.Interface,
	restMapper meta.RESTMapper,
	secretName string,
	sourceNamespace string,
	targetNamespace string,
) error {
	mapping, err := restMapper.RESTMapping(schema.GroupKind{Kind: "Secret"}, "v1")
	if err != nil {
		return fmt.Errorf("failed to get RESTMapping for Secret resource: %w", err)
	}
	secrets := dynamicClient.Resource(mapping.Resource)

	source, err := secrets.Namespace(sourceNamespace).Get(context.TODO(), secretName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf(
			"failed to get Secret '%s' from namespace '%s': %w",
			secretName,
			sourceNamespace,
			err,
		)
	}

	target := secrets.Namespace(targetNamespace)
	existing, err := target.Get(context.TODO(), secretName, metav1.GetOptions{})
	switch {
	case err == nil:
		existing.SetLabels(source.GetLabels())
		existing.SetAnnotations(source.GetAnnotations())
		existing.Object["data"] = source.Object["data"]
		delete(existing.Object, "stringData")
		if _, err := target.Update(context.TODO(), existing, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf(
				"failed to update Secret '%s' in namespace '%s': %w",
				secretName,
				targetNamespace,
				err,
			)
		}
	case k8serrors.IsNotFound(err):
		copied := &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name":      secretName,
				"namespace": targetNamespace,
			},
		}}
		copied.SetLabels(source.GetLabels())
		copied.SetAnnotations(source.GetAnnotations())
		for _, field := range []string{"type", "data"} {
			if value, found := source.Object[field]; found {
				copied.Object[field] = value
			}
		}
		if _, err := target.Create(context.TODO(), copied, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf(
				"failed to create Secret '%s' in namespace '%s': %w",
				secretName,
				targetNamespace,
				err,
			)
		}
	default:
		return fmt.Errorf(
			"failed to check if Secret '%s' exists in namespace '%s': %w",
			secretName,
			targetNamespace,
			err,
		)
	}

	return nil
}
