package v0

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
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

// DiscoverModuleApiServers returns the API server of every registered,
// non-core module, independent of whether it has any controllers.
//
// DiscoverModuleDeployments finds namespaces through ModuleController rows,
// and the SDK supports a module with no reconcilable objects at all - one
// that registers no ModuleController. Its namespace would never surface
// through that path, and its API server would be left without a database
// network policy and cut off from its own database.
//
// Every module registers exactly one ModuleApi, controllers or not, and the
// installer always writes its Endpoint as <service>.<namespace>.svc.cluster.local
// (see MODULE_API_ENDPOINT in the generated installer package) - so the
// service and its namespace are read from there instead of inferred from a
// controller.
func (cpi *ControlPlaneInstaller) DiscoverModuleApiServers(
	apiClient *http.Client,
	apiEndpoint string,
) ([]ModuleApiServer, error) {
	moduleApis, err := client.GetModuleApis(apiClient, apiEndpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to get registered module APIs: %w", err)
	}

	seen := map[ModuleApiServer]bool{}
	var apiServers []ModuleApiServer
	for _, moduleApi := range *moduleApis {
		if moduleApi.Core != nil && *moduleApi.Core {
			continue
		}

		apiServer, ok := moduleApiServerFromEndpoint(moduleApi.Endpoint)
		if !ok || apiServer.Namespace == cpi.Opts.Namespace || seen[apiServer] {
			continue
		}
		seen[apiServer] = true
		apiServers = append(apiServers, apiServer)
	}

	return apiServers, nil
}

// clusterLocalSuffix is what the installer appends after a module's
// namespace when it writes MODULE_API_ENDPOINT.
const clusterLocalSuffix = ".svc.cluster.local"

// moduleApiServerFromEndpoint reads the service and namespace out of a
// module's registered endpoint, which the installer always writes as
// <service>.<namespace>.svc.cluster.local.
//
// A nil endpoint, or one not shaped this way - an external endpoint, say, or
// the core API's own - reports false rather than guessing: admitting the
// wrong pods to the database is worse than leaving one module out.
func moduleApiServerFromEndpoint(endpoint *string) (ModuleApiServer, bool) {
	if endpoint == nil {
		return ModuleApiServer{}, false
	}
	host, hasSuffix := strings.CutSuffix(*endpoint, clusterLocalSuffix)
	if !hasSuffix {
		return ModuleApiServer{}, false
	}

	parts := strings.Split(host, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return ModuleApiServer{}, false
	}

	return ModuleApiServer{Namespace: parts[1], Service: parts[0]}, true
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

		// a retry keeps the count stored before the first scale to zero
		replicas, _, _ := util.NestedInt64OrFloat64(deployment.Object, "spec", "replicas")
		if replicas == 0 {
			recorded, ok := recordedModuleReplicas(deployment)
			if !ok {
				continue
			}
			scales = append(scales, ModuleDeploymentScale{
				Namespace: target.Namespace,
				Name:      target.Name,
				Replicas:  recorded,
			})
			continue
		}

		// skip a live deployment that is not a module controller
		if !moduleControllerDeployment(deployment) {
			fmt.Printf("Info: left %s/%s running because it is not a module controller\n", target.Namespace, target.Name)
			continue
		}

		// scale the deployment to zero and record its prior count
		if err := cpi.recordModuleReplicas(kubeClient, deployment, replicas); err != nil {
			return scales, err
		}
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

// ScaleDownModuleWriters scales module controllers and API servers to zero and returns the counts it stopped.
func (cpi *ControlPlaneInstaller) ScaleDownModuleWriters(
	kubeClient dynamic.Interface,
	controllers []ModuleDeploymentScale,
	apiServers []ModuleApiServer,
) ([]ModuleDeploymentScale, error) {
	// scale controller deployments and keep the counts
	scales, err := cpi.ScaleDownModules(kubeClient, controllers)
	if err != nil {
		return scales, err
	}

	// record each module API server, then scale it to zero
	apiNames := map[string]map[string]int64{}
	for _, server := range apiServers {
		deployment, err := kubeClient.Resource(deploymentGVR).Namespace(server.Namespace).Get(
			context.Background(), server.Service, metav1.GetOptions{},
		)
		if k8serrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return scales, fmt.Errorf("failed to get module API deployment %s/%s: %w", server.Namespace, server.Service, err)
		}
		replicas, _, _ := util.NestedInt64OrFloat64(deployment.Object, "spec", "replicas")
		if replicas == 0 {
			recorded, ok := recordedModuleReplicas(deployment)
			if !ok {
				continue
			}
			replicas = recorded
		} else if err := cpi.recordModuleReplicas(kubeClient, deployment, replicas); err != nil {
			return scales, err
		}
		if err := cpi.setDeploymentReplicas(kubeClient, server.Namespace, server.Service, 0); err != nil {
			return scales, err
		}
		scales = append(scales, ModuleDeploymentScale{
			Namespace: server.Namespace,
			Name:      server.Service,
			Replicas:  replicas,
		})
		if apiNames[server.Namespace] == nil {
			apiNames[server.Namespace] = map[string]int64{}
		}
		apiNames[server.Namespace][server.Service] = 0
	}

	// wait until the scaled API pods are gone
	for namespace, names := range apiNames {
		if err := waitForPodsGone(kubeClient, namespace, names); err != nil {
			return scales, err
		}
	}

	return scales, nil
}

const moduleWriterScaleConfigMap = "module-writer-scales"

var moduleConfigMapGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}

// recordModuleReplicas stores the count that a later retry has to restore.
func (cpi *ControlPlaneInstaller) recordModuleReplicas(
	kubeClient dynamic.Interface,
	deployment *unstructured.Unstructured,
	replicas int64,
) error {
	// replace a count left by an earlier backup with the live count
	patch := fmt.Appendf(
		nil,
		`{"metadata":{"annotations":{%q:%q}}}`,
		replicasBeforeRestoreAnnotation,
		strconv.FormatInt(replicas, 10),
	)
	_, err := kubeClient.Resource(deploymentGVR).Namespace(deployment.GetNamespace()).Patch(
		context.Background(),
		deployment.GetName(),
		types.MergePatchType,
		patch,
		metav1.PatchOptions{},
	)
	if err != nil {
		return fmt.Errorf(
			"failed to record replicas for %s/%s: %w",
			deployment.GetNamespace(), deployment.GetName(), err,
		)
	}
	return nil
}

// recordedModuleReplicas reads the count stored before a module writer was stopped.
func recordedModuleReplicas(deployment *unstructured.Unstructured) (int64, bool) {
	// ignore a deployment that was never stopped by this restore
	raw := deployment.GetAnnotations()[replicasBeforeRestoreAnnotation]
	count, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || count <= 0 {
		return 0, false
	}
	return count, true
}

// rememberModuleScales stores the module writer counts in the control plane namespace.
func (cpi *ControlPlaneInstaller) rememberModuleScales(
	kubeClient dynamic.Interface,
	scales []ModuleDeploymentScale,
) error {
	// keep an empty list so a retry can tell discovery was skipped on purpose
	if scales == nil {
		scales = []ModuleDeploymentScale{}
	}
	encoded, err := json.Marshal(scales)
	if err != nil {
		return fmt.Errorf("failed to encode module writer scales: %w", err)
	}
	object := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]interface{}{
			"name":      moduleWriterScaleConfigMap,
			"namespace": cpi.Opts.Namespace,
		},
		"data": map[string]interface{}{"scales": string(encoded)},
	}}
	_, err = kubeClient.Resource(moduleConfigMapGVR).Namespace(cpi.Opts.Namespace).Create(
		context.Background(), object, metav1.CreateOptions{},
	)
	if k8serrors.IsAlreadyExists(err) {
		_, err = kubeClient.Resource(moduleConfigMapGVR).Namespace(cpi.Opts.Namespace).Update(
			context.Background(), object, metav1.UpdateOptions{},
		)
	}
	if err != nil {
		return fmt.Errorf("failed to store module writer scales: %w", err)
	}
	return nil
}

// rememberedModuleScales reads the module writer counts stored before the API was stopped.
func (cpi *ControlPlaneInstaller) rememberedModuleScales(
	kubeClient dynamic.Interface,
) ([]ModuleDeploymentScale, error) {
	// a missing record means this attempt never stopped the module writers
	object, err := kubeClient.Resource(moduleConfigMapGVR).Namespace(cpi.Opts.Namespace).Get(
		context.Background(), moduleWriterScaleConfigMap, metav1.GetOptions{},
	)
	if k8serrors.IsNotFound(err) {
		return []ModuleDeploymentScale{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read module writer scales: %w", err)
	}
	raw, _, _ := unstructured.NestedString(object.Object, "data", "scales")
	var scales []ModuleDeploymentScale
	if err := json.Unmarshal([]byte(raw), &scales); err != nil {
		return nil, fmt.Errorf("failed to decode module writer scales: %w", err)
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
