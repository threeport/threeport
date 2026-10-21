package v0

import (
	"context"
	"errors"
	"fmt"
	"time"

	kubeerr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	kubemetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// irsaRoleAnnotation is the annotation EKS reads to assume a role.
const irsaRoleAnnotation = "eks.amazonaws.com/role-arn"

// irsaAccounts are the accounts the cert-manager and external-secrets operators create.
var irsaAccounts = map[string]struct{}{
	"nukleros-certs-system/cert-manager":       {},
	"nukleros-secrets-system/external-secrets": {},
}

// createEksAccount creates an operator account.
// An account that already exists gets its role annotation.
func createEksAccount(
	kubeObject *unstructured.Unstructured,
	role string,
	kubeClient dynamic.Interface,
	mapper meta.RESTMapper,
) (*unstructured.Unstructured, error) {
	// get the REST mapping for the account
	mapping, err := getResourceMapping(kubeObject, mapper)
	if err != nil {
		return nil, fmt.Errorf("failed to get REST mapping for kubernetes resource: %w", err)
	}

	// create the account, then set the role when it already exists
	var result *unstructured.Unstructured
	delay := createRetryBaseDelay
	for attempt := 0; attempt < createRetryAttempts; attempt++ {
		result, err = kubeClient.
			Resource(mapping.Resource).
			Namespace(kubeObject.GetNamespace()).
			Create(context.Background(), kubeObject, kubemetav1.CreateOptions{})
		if err == nil {
			return result, nil
		}
		if kubeerr.IsAlreadyExists(err) {
			return updateIrsaRole(kubeObject, role, kubeClient, mapping)
		}
		// create the operator namespace, then retry the account
		if namespaceIsMissing(err) {
			if err := ensureNamespace(kubeClient, kubeObject.GetNamespace()); err != nil {
				return nil, err
			}
			result, err = kubeClient.
				Resource(mapping.Resource).
				Namespace(kubeObject.GetNamespace()).
				Create(context.Background(), kubeObject, kubemetav1.CreateOptions{})
			if err == nil {
				return result, nil
			}
			if kubeerr.IsAlreadyExists(err) {
				return updateIrsaRole(kubeObject, role, kubeClient, mapping)
			}
			return nil, fmt.Errorf("failed to create kubernetes resource:%w", err)
		}
		if !isTransientKubeError(err) {
			return nil, fmt.Errorf("failed to create kubernetes resource:%w", err)
		}
		if attempt < createRetryAttempts-1 {
			time.Sleep(delay)
			delay *= 2
		}
	}

	return nil, fmt.Errorf("failed to create kubernetes resource after %d attempts:%w", createRetryAttempts, err)
}

// irsaRole returns the role annotation for an account the operator creates.
func irsaRole(object *unstructured.Unstructured) (string, bool) {
	// accept only the two operator accounts that carry a role
	if object.GetKind() != "ServiceAccount" {
		return "", false
	}
	if _, ok := irsaAccounts[object.GetNamespace()+"/"+object.GetName()]; !ok {
		return "", false
	}

	role, found, err := unstructured.NestedString(object.Object, "metadata", "annotations", irsaRoleAnnotation)
	if err != nil || !found || role == "" {
		return "", false
	}

	return role, true
}

// namespaceIsMissing reports whether a create failed because the namespace is absent.
func namespaceIsMissing(err error) bool {
	// match a missing namespace
	var status *kubeerr.StatusError
	if !errors.As(err, &status) || status.ErrStatus.Details == nil {
		return false
	}
	return status.ErrStatus.Details.Kind == "namespaces"
}

// ensureNamespace creates the namespace when it is not already present.
func ensureNamespace(kubeClient dynamic.Interface, name string) error {
	namespace := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Namespace",
		"metadata": map[string]interface{}{
			"name": name,
		},
	}}
	gvr := schema.GroupVersionResource{Version: "v1", Resource: "namespaces"}

	// create the namespace
	_, err := kubeClient.Resource(gvr).Create(context.Background(), namespace, kubemetav1.CreateOptions{})
	if err != nil && !kubeerr.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create kubernetes namespace: %w", err)
	}
	return nil
}

// updateIrsaRole sets the role annotation on an account that already exists.
func updateIrsaRole(
	desired *unstructured.Unstructured,
	role string,
	kubeClient dynamic.Interface,
	mapping *meta.RESTMapping,
) (*unstructured.Unstructured, error) {
	existing, err := kubeClient.
		Resource(mapping.Resource).
		Namespace(desired.GetNamespace()).
		Get(context.Background(), desired.GetName(), kubemetav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get existing service account: %w", err)
	}

	// set the role annotation on the existing account
	annotations := existing.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[irsaRoleAnnotation] = role
	existing.SetAnnotations(annotations)

	updated, err := kubeClient.
		Resource(mapping.Resource).
		Namespace(existing.GetNamespace()).
		Update(context.Background(), existing, kubemetav1.UpdateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to update service account role annotation: %w", err)
	}

	return updated, nil
}
