package v0

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	client "github.com/threeport/threeport/pkg/client/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// Registered module deployment names live in the control plane database and
// are read before that database is dropped. The deployments are scaled to
// zero before the drop and restored after the control plane is installed again.

// ModuleDeploymentScale is one module deployment's replica count from before
// it was scaled to zero.
type ModuleDeploymentScale struct {
	// The namespace of the deployment
	Namespace string
	// The name of the deployment
	Name string
	// The replica count recorded before the deployment was scaled to zero
	Replicas int64
}

// DiscoverModuleDeployments returns the non-core module controller deployments
// registered in the control plane, omitting the control plane namespace.
func (cpi *ControlPlaneInstaller) DiscoverModuleDeployments(
	apiClient *http.Client,
	apiEndpoint string,
) ([]ModuleDeploymentScale, error) {
	// get registered module APIs
	moduleApis, err := client.GetModuleApis(apiClient, apiEndpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to get registered module APIs: %w", err)
	}

	var deployments []ModuleDeploymentScale
	// exclude the control plane namespace and repeated deployment names
	seenNamespace := map[string]bool{cpi.Opts.Namespace: true}
	seenDeployment := map[string]bool{}

	for _, moduleApi := range *moduleApis {
		// skip core modules and records without an id
		if moduleApi.Core != nil && *moduleApi.Core {
			continue
		}
		if moduleApi.ID == nil {
			continue
		}

		// get controllers registered by the module API
		controllers, err := client.GetModuleControllersByQueryString(
			apiClient,
			apiEndpoint,
			fmt.Sprintf("moduleapiid=%d", *moduleApi.ID),
		)
		if err != nil {
			return nil, fmt.Errorf(
				"failed to get controllers registered by module API %d: %w",
				*moduleApi.ID, err,
			)
		}

		for _, controller := range *controllers {
			// record each namespace/name deployment outside the control plane
			if controller.DeploymentName == nil {
				continue
			}
			namespace, name, qualified := strings.Cut(*controller.DeploymentName, "/")
			if !qualified || namespace == "" || name == "" {
				continue
			}
			if seenNamespace[namespace] && namespace == cpi.Opts.Namespace {
				continue
			}
			key := namespace + "/" + name
			if seenDeployment[key] {
				continue
			}
			seenDeployment[key] = true
			deployments = append(deployments, ModuleDeploymentScale{
				Namespace: namespace,
				Name:      name,
			})
		}
	}

	return deployments, nil
}

// DiscoverModuleNamespaces returns the namespace of every registered,
// non-core module, independent of whether it has any controllers.
//
// DiscoverModuleDeployments finds namespaces through ModuleController rows,
// and the SDK supports a module with no reconcilable objects at all - one
// that registers no ModuleController. Its namespace would never surface
// through that path, and its API server would be left unlabelled and cut off
// from its own database the moment the network policy applies.
//
// Every module registers exactly one ModuleApi, controllers or not, and the
// installer always writes its Endpoint as <service>.<namespace>.svc.cluster.local
// (see MODULE_API_ENDPOINT in the generated installer package) - so the
// namespace is read from there instead of inferred from a controller.
func (cpi *ControlPlaneInstaller) DiscoverModuleNamespaces(
	apiClient *http.Client,
	apiEndpoint string,
) ([]string, error) {
	moduleApis, err := client.GetModuleApis(apiClient, apiEndpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to get registered module APIs: %w", err)
	}

	seen := map[string]bool{}
	var namespaces []string
	for _, moduleApi := range *moduleApis {
		if moduleApi.Core != nil && *moduleApi.Core {
			continue
		}

		namespace, ok := moduleNamespaceFromEndpoint(moduleApi.Endpoint)
		if !ok || namespace == cpi.Opts.Namespace || seen[namespace] {
			continue
		}
		seen[namespace] = true
		namespaces = append(namespaces, namespace)
	}

	return namespaces, nil
}

// clusterLocalSuffix is what the installer appends after a module's
// namespace when it writes MODULE_API_ENDPOINT.
const clusterLocalSuffix = ".svc.cluster.local"

// moduleNamespaceFromEndpoint reads the namespace out of a module's
// registered endpoint, which the installer always writes as
// <service>.<namespace>.svc.cluster.local.
//
// A nil endpoint, or one not shaped this way - an external endpoint, say, or
// the core API's own - reports false rather than guessing: admitting the
// wrong namespace to the database is worse than leaving one unlabelled.
func moduleNamespaceFromEndpoint(endpoint *string) (string, bool) {
	if endpoint == nil {
		return "", false
	}
	host, hasSuffix := strings.CutSuffix(*endpoint, clusterLocalSuffix)
	if !hasSuffix {
		return "", false
	}

	parts := strings.Split(host, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", false
	}

	return parts[1], true
}

// ScaleDownModules scales each named module controller deployment with a
// non-zero replica count to zero. An error still returns counts already scaled.
func (cpi *ControlPlaneInstaller) ScaleDownModules(
	kubeClient dynamic.Interface,
	targets []ModuleDeploymentScale,
) ([]ModuleDeploymentScale, error) {
	var scales []ModuleDeploymentScale

	for _, target := range targets {
		// read the registered deployment
		deployment, err := kubeClient.Resource(deploymentGVR).Namespace(target.Namespace).Get(
			context.Background(), target.Name, metav1.GetOptions{},
		)
		if k8serrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return scales, fmt.Errorf(
				"failed to get module deployment %s/%s: %w",
				target.Namespace, target.Name, err,
			)
		}

		// skip a deployment already at zero so it stays out of the record
		replicas, _, _ := util.NestedInt64OrFloat64(deployment.Object, "spec", "replicas")
		if replicas == 0 {
			continue
		}

		// skip a live deployment that is not a module controller
		if !moduleControllerDeployment(deployment) {
			fmt.Printf("Info: left %s/%s running because it is not a module controller\n", target.Namespace, target.Name)
			continue
		}

		// scale the deployment to zero and record its prior count
		if err := cpi.setDeploymentReplicas(kubeClient, target.Namespace, target.Name, 0); err != nil {
			return scales, err
		}
		scales = append(scales, ModuleDeploymentScale{
			Namespace: target.Namespace,
			Name:      target.Name,
			Replicas:  replicas,
		})
	}

	// return when nothing was scaled
	if len(scales) == 0 {
		return scales, nil
	}

	// report the scale-down
	fmt.Printf("Info: scaled %d module deployment(s) to 0\n", len(scales))

	// wait until no ready replicas remain on the scaled deployments
	if err := util.Retry(60, 3, func() error {
		// count ready replicas still present
		pending := 0
		for _, target := range scales {
			current, err := kubeClient.Resource(deploymentGVR).Namespace(target.Namespace).Get(
				context.Background(), target.Name, metav1.GetOptions{},
			)
			if k8serrors.IsNotFound(err) {
				continue
			}
			if err != nil {
				return fmt.Errorf(
					"failed to get module deployment %s/%s while waiting for scale-down: %w",
					target.Namespace, target.Name, err,
				)
			}
			ready, _, _ := util.NestedInt64OrFloat64(current.Object, "status", "readyReplicas")
			if ready > 0 {
				pending += int(ready)
			}
		}
		if pending > 0 {
			return fmt.Errorf("%d module replica(s) still present", pending)
		}
		return nil
	}); err != nil {
		return scales, fmt.Errorf("module deployments did not scale to zero: %w", err)
	}

	return scales, nil
}

// moduleControllerDeployment reports whether the deployment is a module
// controller. Module installers do not stamp the managed-by label.
func moduleControllerDeployment(deployment *unstructured.Unstructured) bool {
	// require the -controller name suffix
	name := deployment.GetName()
	if !strings.HasSuffix(name, "-controller") {
		return false
	}

	// require the pod label to match that name
	labels, _, _ := unstructured.NestedStringMap(deployment.Object, "spec", "template", "metadata", "labels")
	if labels["app.kubernetes.io/name"] != name {
		return false
	}

	// require a -controller container command
	containers, _, _ := unstructured.NestedSlice(deployment.Object, "spec", "template", "spec", "containers")
	for _, raw := range containers {
		container, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		command, _, _ := unstructured.NestedStringSlice(container, "command")
		for _, entry := range command {
			if strings.HasSuffix(entry, "-controller") {
				return true
			}
		}
	}
	return false
}

// RestoreModuleScale sets each recorded deployment back to its prior replica
// count and does not wait for pods to become ready.
func (cpi *ControlPlaneInstaller) RestoreModuleScale(
	kubeClient dynamic.Interface,
	scales []ModuleDeploymentScale,
) error {
	// return when there is nothing to restore
	if len(scales) == 0 {
		return nil
	}

	// restore each recorded replica count
	for _, scale := range scales {
		if err := cpi.setDeploymentReplicas(
			kubeClient, scale.Namespace, scale.Name, scale.Replicas,
		); err != nil {
			return err
		}
	}

	// report the restore
	fmt.Printf("Info: restored %d module deployment(s) to their original replica count\n", len(scales))

	return nil
}

// setDeploymentReplicas patches a deployment to the given replica count and
// treats a missing deployment as success.
func (cpi *ControlPlaneInstaller) setDeploymentReplicas(
	kubeClient dynamic.Interface,
	namespace string,
	name string,
	replicas int64,
) error {
	// patch the deployment replica count
	patch := fmt.Appendf(nil, `{"spec":{"replicas":%d}}`, replicas)
	_, err := kubeClient.Resource(deploymentGVR).Namespace(namespace).Patch(
		context.Background(),
		name,
		"application/strategic-merge-patch+json",
		patch,
		metav1.PatchOptions{},
	)
	if err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("failed to scale deployment %s/%s to %d: %w", namespace, name, replicas, err)
	}

	return nil
}

// namespaceGVR is the v1 namespaces resource, used to label the namespaces
// modules are installed into.
var namespaceGVR = schema.GroupVersionResource{
	Group:    "",
	Version:  "v1",
	Resource: "namespaces",
}

// LabelModuleNamespaces marks the namespaces of registered modules so the
// database network policy admits them.
//
// New modules carry the label from the moment they are created: the SDK
// generates it onto the namespace. Modules installed before that do not, and
// nothing regenerates them on their own - so without this, installing the
// policy would cut every module already running away from its own database,
// and the first anyone would hear of it is a module failing long after the
// control plane reported success.
//
// It takes the namespaces the caller has already discovered - with
// DiscoverModuleNamespaces, not DiscoverModuleDeployments, which only sees a
// module through its controllers and misses one that registers none - rather
// than reading the registry itself. Reinstall drops the control plane
// database between discovering them and reaching here, so by this point
// there is no registry left to ask.
//
// One namespace that cannot be labelled does not stop the others. A
// namespace that has already been removed is not a failure, and one this has
// no permission to label is better reported at the end with the rest than as
// the one error that hid them.
func (cpi *ControlPlaneInstaller) LabelModuleNamespaces(
	kubeClient dynamic.Interface,
	namespaces []string,
) error {
	labelled := map[string]bool{}
	var failures []error
	for _, namespace := range namespaces {
		if labelled[namespace] {
			continue
		}
		labelled[namespace] = true

		if err := cpi.labelNamespace(kubeClient, namespace); err != nil {
			failures = append(failures, err)
		}
	}

	return errors.Join(failures...)
}

// labelNamespace marks one namespace as a module's. A namespace that is no
// longer there is not an error: the registry outlives what it describes.
func (cpi *ControlPlaneInstaller) labelNamespace(
	kubeClient dynamic.Interface,
	namespace string,
) error {
	patch := fmt.Appendf(nil, `{"metadata":{"labels":{%q:%q}}}`, LabelModule, LabelModuleValue)
	_, err := kubeClient.Resource(namespaceGVR).Patch(
		context.Background(),
		namespace,
		"application/strategic-merge-patch+json",
		patch,
		metav1.PatchOptions{},
	)
	if err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("failed to label module namespace %s: %w", namespace, err)
	}

	return nil
}
