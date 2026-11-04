package v0

import (
	"context"
	"errors"
	"fmt"
	"strings"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// clusterRoleBindingGVR is the resource the control plane's cluster-scoped
// bindings are stored as.
var clusterRoleBindingGVR = schema.GroupVersionResource{
	Group:    "rbac.authorization.k8s.io",
	Version:  "v1",
	Resource: "clusterrolebindings",
}

// workloadBindingSuffix and clusterAdminBindingSuffix end the names of the
// cluster role bindings the installer creates for each controller.
const (
	workloadBindingSuffix     = "threeportworkloads"
	clusterAdminBindingSuffix = "cluster-admin"
)

// clusterAdminControllers are the controllers that are bound to cluster-admin.
func clusterAdminControllers() []string {
	return []string{
		ThreeportHelmWorkloadControllerName,
		ThreeportKubernetesWorkloadControllerName,
		ThreeportControlPlaneControllerName,
	}
}

// clusterBindingName returns the name of a controller's cluster role binding in
// a control plane namespace.
func clusterBindingName(namespace, serviceAccount, suffix string) string {
	return fmt.Sprintf("%s-%s-%s", namespace, serviceAccount, suffix)
}

// workloadIdentityUser returns the user a pod's token authenticates as on GKE
// with Workload Identity.
func workloadIdentityUser(gcpProjectID, namespace, serviceAccount string) string {
	return fmt.Sprintf("serviceAccount:%s.svc.id.goog[%s/%s]", gcpProjectID, namespace, serviceAccount)
}

// controllerClusterRoleBindings builds the cluster-scoped bindings for one
// controller: a binding of the shared threeportworkloads cluster role for every
// controller, and a cluster-admin binding for the controllers that deploy
// arbitrary resources.
//
// Each binding is labeled with the control plane namespace.  The bindings
// outlive that namespace, so the label is what lets the teardown find them.
func (cpi *ControlPlaneInstaller) controllerClusterRoleBindings(
	controller *v0.ControlPlaneComponent,
) []*unstructured.Unstructured {
	namespace := cpi.Opts.Namespace

	// On GKE with Workload Identity enabled, the kube-apiserver authenticates
	// pod tokens as "serviceAccount:PROJECT.svc.id.goog[NAMESPACE/KSA]" rather
	// than "system:serviceaccount:NAMESPACE:NAME". The kind:ServiceAccount
	// subject only matches the latter, so we add a kind:User subject for the
	// WI principal to ensure RBAC applies to the actual pod identity.  On GKE
	// each controller authenticates to the target cluster as its own WI
	// principal (via per-request ADC tokens), so both subjects are bound.
	subjects := func() []interface{} {
		subjects := []interface{}{
			map[string]interface{}{
				"kind":      "ServiceAccount",
				"name":      controller.ServiceAccountName,
				"namespace": namespace,
			},
		}
		if cpi.Opts.InfraProvider == v0.KubernetesRuntimeInfraProviderGKE && cpi.Opts.GcpProjectId != "" {
			subjects = append(subjects, map[string]interface{}{
				"kind":     "User",
				"name":     workloadIdentityUser(cpi.Opts.GcpProjectId, namespace, controller.ServiceAccountName),
				"apiGroup": "rbac.authorization.k8s.io",
			})
		}
		return subjects
	}

	binding := func(suffix, clusterRole string) *unstructured.Unstructured {
		return &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "rbac.authorization.k8s.io/v1",
				"kind":       "ClusterRoleBinding",
				"metadata": map[string]interface{}{
					"name": clusterBindingName(namespace, controller.ServiceAccountName, suffix),
					"labels": map[string]interface{}{
						LabelControlPlaneNamespace: namespace,
					},
				},
				"roleRef": map[string]interface{}{
					"apiGroup": "rbac.authorization.k8s.io",
					"kind":     "ClusterRole",
					"name":     clusterRole,
				},
				"subjects": subjects(),
			},
		}
	}

	// Bind the shared ClusterRole to this controller's service account so it
	// can manage ThreeportWorkload resources.
	bindings := []*unstructured.Unstructured{
		binding(workloadBindingSuffix, "threeport-controllers-threeportworkloads"),
	}

	// The helm-workload-controller and kubernetes-workload-controller deploy
	// arbitrary resources — Helm charts or raw manifests — that can define any
	// Kubernetes resource type in any namespace (including creating
	// namespaces). The control-plane-controller similarly creates namespaces,
	// secrets, service accounts, and StatefulSets/Deployments directly when
	// bootstrapping a new child control plane. cluster-admin is required so
	// they can manage the full lifecycle of those resources.
	for _, name := range clusterAdminControllers() {
		if controller.Name == name {
			bindings = append(bindings, binding(clusterAdminBindingSuffix, "cluster-admin"))
			break
		}
	}

	return bindings
}

// DeleteControlPlaneClusterRoleBindings deletes the cluster-scoped bindings the
// installer created for a control plane namespace.  Deleting the namespace
// leaves them behind, and the cluster-admin ones would then grant that access
// to any service account of the same name that later appears in a namespace of
// the same name.
//
// Bindings labeled with the namespace are deleted.  Bindings created before the
// label existed are found by the names the installer gave them, and are deleted
// only when every subject is in this namespace, so a name that happens to match
// another control plane's binding is left alone.  The shared cluster roles are
// never touched: other control planes on the cluster use them.
//
// A binding that is already gone counts as deleted.  Any other error is
// returned, after every deletion has been attempted.
func DeleteControlPlaneClusterRoleBindings(
	kubeClient dynamic.Interface,
	namespace string,
) error {
	if namespace == "" {
		return errors.New("a control plane namespace is required to delete its cluster role bindings")
	}
	bindings := kubeClient.Resource(clusterRoleBindingGVR)
	var errs []error

	// delete the bindings labeled with this namespace
	selector := fmt.Sprintf(
		"%s=%s,%s=%s",
		LabelManagedBy, LabelManagedByValue,
		LabelControlPlaneNamespace, namespace,
	)
	labeled, err := bindings.List(context.Background(), metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		errs = append(errs, fmt.Errorf("failed to list cluster role bindings matching %q: %w", selector, err))
	} else {
		for _, item := range labeled.Items {
			if err := deleteClusterRoleBinding(bindings, item.GetName()); err != nil {
				errs = append(errs, err)
			}
		}
	}

	// delete the bindings from before the label by the names they were given
	for _, name := range legacyClusterBindingNames(namespace) {
		existing, err := bindings.Get(context.Background(), name, metav1.GetOptions{})
		if k8serrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to get cluster role binding %s: %w", name, err))
			continue
		}
		if !bindingSubjectsInNamespace(existing, namespace) {
			continue
		}
		if err := deleteClusterRoleBinding(bindings, name); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// legacyClusterBindingNames lists the names the installer gave a control
// plane's bindings for its own controllers before they were labeled.
func legacyClusterBindingNames(namespace string) []string {
	names := make([]string, 0, len(ThreeportControllerList)+len(clusterAdminControllers()))
	for _, controller := range ThreeportControllerList {
		names = append(names, clusterBindingName(namespace, controller.ServiceAccountName, workloadBindingSuffix))
	}
	for _, serviceAccount := range clusterAdminControllers() {
		names = append(names, clusterBindingName(namespace, serviceAccount, clusterAdminBindingSuffix))
	}
	return names
}

// bindingSubjectsInNamespace reports whether the binding has subjects and every
// one is in the namespace: a service account there, or the Workload Identity
// user for one.
func bindingSubjectsInNamespace(binding *unstructured.Unstructured, namespace string) bool {
	subjects, found, err := unstructured.NestedSlice(binding.Object, "subjects")
	if err != nil || !found || len(subjects) == 0 {
		return false
	}
	for _, s := range subjects {
		subject, ok := s.(map[string]interface{})
		if !ok {
			return false
		}
		kind, _ := subject["kind"].(string)
		switch kind {
		case "ServiceAccount":
			if subjectNamespace, _ := subject["namespace"].(string); subjectNamespace != namespace {
				return false
			}
		case "User":
			name, _ := subject["name"].(string)
			if !strings.HasPrefix(name, "serviceAccount:") || !strings.Contains(name, "["+namespace+"/") {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// deleteClusterRoleBinding deletes one binding, treating a missing one as done.
func deleteClusterRoleBinding(bindings dynamic.NamespaceableResourceInterface, name string) error {
	if err := bindings.Delete(context.Background(), name, metav1.DeleteOptions{}); err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete cluster role binding %s: %w", name, err)
	}
	return nil
}
