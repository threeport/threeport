package v0

import (
	"context"
	"errors"
	"fmt"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	kube "github.com/threeport/threeport/pkg/kube/v0"
)

// namespaceNameLabel is the label the Kubernetes API server sets on every
// namespace, always equal to its name.  Nothing can make it say anything
// else, unlike a label of threeport's own, which anything allowed to patch a
// namespace can add to any namespace it likes.
const namespaceNameLabel = "kubernetes.io/metadata.name"

// namespaceGVR is the v1 namespaces resource.
var namespaceGVR = schema.GroupVersionResource{Version: "v1", Resource: "namespaces"}

// serviceGVR is the v1 services resource.
var serviceGVR = schema.GroupVersionResource{Version: "v1", Resource: "services"}

// ModuleDatabasePolicyName returns the name of the network policy that admits
// a module's API server to the control plane's database.  It is named after
// the API server, which is unique to the module even when two modules share a
// namespace.
func ModuleDatabasePolicyName(apiServerName string) string {
	return "threeport-database-access-" + apiServerName
}

// ModuleDatabasePolicy returns the network policy that admits a module's API
// server to the control plane's database, on the SQL port only.
//
// A module keeps its own database inside the control plane's CockroachDB, and
// its API server's pod connects to it directly: its db-init and
// database-migrator init containers, and the API server itself.  The policy
// admits the pods carrying apiServerLabels in the module's namespace and
// nothing else: the namespace is matched by the label the Kubernetes API
// server keeps equal to its name, and the pods by their labels in that same
// peer, so both have to hold.
//
// The policy lives in the control plane's namespace, beside the database it
// opens, and is owned by the module's namespace, so deleting that namespace
// deletes the policy.  A namespace created again under the same name is not
// admitted until the module is installed into it again.
func ModuleDatabasePolicy(
	controlPlaneNamespace string,
	moduleNamespace *unstructured.Unstructured,
	apiServerName string,
	apiServerLabels map[string]string,
) *unstructured.Unstructured {
	podLabels := map[string]interface{}{}
	for key, value := range apiServerLabels {
		podLabels[key] = value
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "networking.k8s.io/v1",
			"kind":       "NetworkPolicy",
			"metadata": map[string]interface{}{
				"name":      ModuleDatabasePolicyName(apiServerName),
				"namespace": controlPlaneNamespace,
				"ownerReferences": []interface{}{
					map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "Namespace",
						"name":       moduleNamespace.GetName(),
						"uid":        string(moduleNamespace.GetUID()),
					},
				},
			},
			"spec": map[string]interface{}{
				"podSelector": podsNamed("cockroachdb"),
				"policyTypes": []interface{}{"Ingress"},
				"ingress": []interface{}{
					map[string]interface{}{
						"from": []interface{}{
							map[string]interface{}{
								"namespaceSelector": labelled(namespaceNameLabel, moduleNamespace.GetName()),
								"podSelector": map[string]interface{}{
									"matchLabels": podLabels,
								},
							},
						},
						"ports": []interface{}{
							tcpPort(cockroachSqlPort),
						},
					},
				},
			},
		},
	}
}

// InstallModuleDatabasePolicy creates or updates the network policy that
// admits a module's API server to the control plane's database.  The
// module's namespace has to exist, since the policy is owned by it.
func InstallModuleDatabasePolicy(
	kubeClient dynamic.Interface,
	mapper meta.RESTMapper,
	controlPlaneNamespace string,
	moduleNamespace string,
	apiServerName string,
	apiServerLabels map[string]string,
) error {
	if len(apiServerLabels) == 0 {
		return fmt.Errorf(
			"refusing to admit every pod of namespace %s to the database: API server %s has no pod labels",
			moduleNamespace, apiServerName,
		)
	}

	namespace, err := kubeClient.Resource(namespaceGVR).Get(
		context.Background(), moduleNamespace, metav1.GetOptions{},
	)
	if err != nil {
		return fmt.Errorf("failed to get module namespace %s: %w", moduleNamespace, err)
	}

	policy := ModuleDatabasePolicy(controlPlaneNamespace, namespace, apiServerName, apiServerLabels)
	if _, err := kube.CreateOrUpdateResource(policy, kubeClient, mapper); err != nil {
		return fmt.Errorf("failed to create network policy %s: %w", policy.GetName(), err)
	}

	return nil
}

// ModuleApiServer is a registered module's API server: the service its
// endpoint names, and the namespace that service is in.
type ModuleApiServer struct {
	Namespace string
	Service   string
}

// InstallModuleDatabasePolicies creates the database network policy of each
// registered module, for modules installed before their installer created
// one.  The pods admitted are the ones the module's API server service
// selects.
//
// One module whose policy cannot be installed does not stop the others.  A
// module whose namespace or service is gone is skipped: the registry
// outlives what it describes.
func (cpi *ControlPlaneInstaller) InstallModuleDatabasePolicies(
	kubeClient dynamic.Interface,
	mapper meta.RESTMapper,
	apiServers []ModuleApiServer,
) error {
	var failures []error
	for _, apiServer := range apiServers {
		service, err := kubeClient.Resource(serviceGVR).Namespace(apiServer.Namespace).Get(
			context.Background(), apiServer.Service, metav1.GetOptions{},
		)
		if k8serrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			failures = append(failures, fmt.Errorf(
				"failed to get module API server service %s/%s: %w",
				apiServer.Namespace, apiServer.Service, err,
			))
			continue
		}

		selector, _, err := unstructured.NestedStringMap(service.Object, "spec", "selector")
		if err != nil {
			failures = append(failures, fmt.Errorf(
				"failed to read the selector of service %s/%s: %w",
				apiServer.Namespace, apiServer.Service, err,
			))
			continue
		}

		err = InstallModuleDatabasePolicy(
			kubeClient, mapper, cpi.Opts.Namespace, apiServer.Namespace, apiServer.Service, selector,
		)
		if err != nil && !k8serrors.IsNotFound(err) {
			failures = append(failures, err)
		}
	}

	return errors.Join(failures...)
}
