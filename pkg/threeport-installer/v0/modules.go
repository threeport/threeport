package v0

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/iancoleman/strcase"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"

	"github.com/threeport/threeport/pkg/api-server/v0/database"
	api "github.com/threeport/threeport/pkg/api/v0"
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

// scaleDownModuleWriters scales module controllers and API servers to zero and returns the counts it stopped.
func (cpi *ControlPlaneInstaller) scaleDownModuleWriters(
	kubeClient dynamic.Interface,
	controllers []ModuleDeploymentScale,
	apiServers []ModuleApiServer,
) ([]ModuleDeploymentScale, error) {
	// include controllers that live in a module API namespace but are not registered by name
	found, err := moduleNamespaceControllers(kubeClient, apiServers, controllers)
	if err != nil {
		return nil, err
	}
	controllers = append(controllers, found...)

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

const (
	moduleDatabaseConfigMap = "module-databases"
	moduleDatabaseConfigKey = "databases"
)

// moduleDatabaseName returns the database created for a module.
func moduleDatabaseName(moduleName string) string {
	return fmt.Sprintf("threeport_%s_api", strcase.ToSnake(moduleName))
}

// moduleDatabaseNames returns the database of each module that keeps its own.
// A core module shares the control plane database and is left out.
func moduleDatabaseNames(moduleApis []api.ModuleApi) ([]string, error) {
	// skip the control plane database and repeated names
	seen := map[string]bool{}
	var names []string
	for _, moduleApi := range moduleApis {
		if moduleApi.Core != nil && *moduleApi.Core {
			continue
		}
		moduleName := util.DerefString(moduleApi.Name)
		if moduleName == "" {
			continue
		}
		// the registered name is namespace/short-module-api, and the database uses the short name
		shortName := moduleKebab(moduleName)
		if shortName == "" {
			shortName = moduleName
		}
		name := moduleDatabaseName(shortName)
		if name == DatabaseName || seen[name] {
			continue
		}
		if !moduleDatabaseNamePattern.MatchString(name) {
			return nil, fmt.Errorf("module database name %q is not a sql identifier", name)
		}
		seen[name] = true
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// discoverModuleDatabases returns the module databases registered with the API.
func (cpi *ControlPlaneInstaller) discoverModuleDatabases(
	apiClient *http.Client,
	apiEndpoint string,
) ([]string, error) {
	// read the registered module APIs
	moduleApis, err := client.GetModuleApis(apiClient, apiEndpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to get registered module APIs: %w", err)
	}
	if moduleApis == nil {
		return []string{}, nil
	}
	return moduleDatabaseNames(*moduleApis)
}

// rememberModuleDatabases stores the module databases in the control plane namespace.
// An empty list is stored so a retry can tell that none were registered.
func rememberModuleDatabases(kubeClient dynamic.Interface, namespace string, names []string) error {
	// keep an empty list so a retry can tell discovery found none
	if names == nil {
		names = []string{}
	}
	encoded, err := json.Marshal(names)
	if err != nil {
		return fmt.Errorf("failed to encode module databases: %w", err)
	}
	object := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]interface{}{
			"name":      moduleDatabaseConfigMap,
			"namespace": namespace,
		},
		"data": map[string]interface{}{moduleDatabaseConfigKey: string(encoded)},
	}}
	_, err = kubeClient.Resource(moduleConfigMapGVR).Namespace(namespace).Create(
		context.Background(), object, metav1.CreateOptions{},
	)
	if k8serrors.IsAlreadyExists(err) {
		_, err = kubeClient.Resource(moduleConfigMapGVR).Namespace(namespace).Update(
			context.Background(), object, metav1.UpdateOptions{},
		)
	}
	if err != nil {
		return fmt.Errorf("failed to store module databases: %w", err)
	}
	return nil
}

// readModuleDatabases reads the module databases stored before the API was stopped.
// The second result is false when that record was never written.
func readModuleDatabases(kubeClient dynamic.Interface, namespace string) ([]string, bool, error) {
	// a missing record means this attempt never listed the module databases
	object, err := kubeClient.Resource(moduleConfigMapGVR).Namespace(namespace).Get(
		context.Background(), moduleDatabaseConfigMap, metav1.GetOptions{},
	)
	if k8serrors.IsNotFound(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("failed to read module databases: %w", err)
	}
	raw, _, _ := unstructured.NestedString(object.Object, "data", moduleDatabaseConfigKey)
	var names []string
	if err := json.Unmarshal([]byte(raw), &names); err != nil {
		return nil, false, fmt.Errorf("failed to decode module databases: %w", err)
	}
	return names, true, nil
}

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

// moduleApiServersFromPolicies returns module API servers named by database policies in the control plane namespace.
func (cpi *ControlPlaneInstaller) moduleApiServersFromPolicies(
	kubeClient dynamic.Interface,
) ([]ModuleApiServer, error) {
	// list the policies that admit module API servers to the database
	list, err := kubeClient.Resource(networkPolicyGVR).Namespace(cpi.Opts.Namespace).List(
		context.Background(),
		metav1.ListOptions{},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list database policies in %s: %w", cpi.Opts.Namespace, err)
	}

	// skip this namespace, a repeated server, and a name that does not parse
	seen := map[ModuleApiServer]bool{}
	var servers []ModuleApiServer
	for i := range list.Items {
		server, ok := moduleApiServerFromPolicyName(list.Items[i].GetName())
		if !ok ||
			server.Namespace == cpi.Opts.Namespace ||
			seen[server] {
			continue
		}
		seen[server] = true
		servers = append(servers, server)
	}

	return servers, nil
}

// moduleApiServersFromDatabaseHost returns module API servers whose command names this control plane's database.
// A module installed before it had a database policy still carries that command.
func (cpi *ControlPlaneInstaller) moduleApiServersFromDatabaseHost(
	kubeClient dynamic.Interface,
) ([]ModuleApiServer, error) {
	// list every deployment
	host := controlPlaneDatabaseHost(cpi.Opts.Namespace)
	list, err := kubeClient.Resource(deploymentGVR).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list deployments while finding module writers: %w", err)
	}

	// skip this namespace and a deployment that does not name this database
	seen := map[ModuleApiServer]bool{}
	var servers []ModuleApiServer
	for i := range list.Items {
		deployment := &list.Items[i]
		namespace := deployment.GetNamespace()
		if namespace == "" ||
			namespace == cpi.Opts.Namespace ||
			!deploymentCommandContains(deployment, host) {
			continue
		}
		server := ModuleApiServer{Namespace: namespace, Service: deployment.GetName()}
		if seen[server] {
			continue
		}
		seen[server] = true
		servers = append(servers, server)
	}

	return servers, nil
}

// controlPlaneDatabaseHost returns the database service a module command records.
func controlPlaneDatabaseHost(namespace string) string {
	return database.ThreeportDatabaseHost + "." + namespace + clusterLocalSuffix
}

// deploymentCommandContains reports whether a container command or argument contains text.
func deploymentCommandContains(deployment *unstructured.Unstructured, text string) bool {
	// check each container command and its arguments
	for _, listKey := range []string{"containers", "initContainers"} {
		containers, _, _ := unstructured.NestedSlice(deployment.Object, "spec", "template", "spec", listKey)
		for _, raw := range containers {
			container, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			for _, field := range []string{"command", "args"} {
				values, _, _ := unstructured.NestedStringSlice(container, field)
				for _, value := range values {
					if strings.Contains(value, text) {
						return true
					}
				}
			}
		}
	}
	return false
}

// appendModuleApiServers adds module API servers that are not already listed.
func appendModuleApiServers(base, extra []ModuleApiServer) []ModuleApiServer {
	// skip a repeated server
	seen := map[ModuleApiServer]bool{}
	for _, server := range base {
		seen[server] = true
	}
	for _, server := range extra {
		if server.Namespace == "" || seen[server] {
			continue
		}
		seen[server] = true
		base = append(base, server)
	}
	return base
}

// moduleNamespaceControllers returns controller deployments in each module API namespace.
func moduleNamespaceControllers(
	kubeClient dynamic.Interface,
	apiServers []ModuleApiServer,
	known []ModuleDeploymentScale,
) ([]ModuleDeploymentScale, error) {
	// skip names the registry already returned
	seen := map[string]bool{}
	for _, item := range known {
		seen[item.Namespace+"/"+item.Name] = true
	}
	seenNamespace := map[string]bool{}
	var found []ModuleDeploymentScale
	for _, server := range apiServers {
		if server.Namespace == "" || seenNamespace[server.Namespace] {
			continue
		}
		seenNamespace[server.Namespace] = true

		// list the deployments in the module namespace
		list, err := kubeClient.Resource(deploymentGVR).Namespace(server.Namespace).List(
			context.Background(),
			metav1.ListOptions{},
		)
		if k8serrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("failed to list deployments in %s: %w", server.Namespace, err)
		}
		for i := range list.Items {
			deployment := &list.Items[i]
			if !moduleControllerDeployment(deployment) {
				continue
			}
			key := deployment.GetNamespace() + "/" + deployment.GetName()
			if seen[key] {
				continue
			}
			seen[key] = true
			found = append(found, ModuleDeploymentScale{
				Namespace: deployment.GetNamespace(),
				Name:      deployment.GetName(),
			})
		}
	}
	return found, nil
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

	// a present name label must match. An installer that stamps none is still a controller
	labels, _, _ := unstructured.NestedStringMap(deployment.Object, "spec", "template", "metadata", "labels")
	if label := labels["app.kubernetes.io/name"]; label != "" && label != name {
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
// count, removes the count recorded for the backup, and does not wait for pods
// to become ready.
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
		if err := cpi.resumeDeploymentReplicas(
			kubeClient, scale.Namespace, scale.Name, scale.Replicas,
		); err != nil {
			return err
		}
	}

	// report the restore
	fmt.Printf("Info: restored %d module deployment(s) to their original replica count\n", len(scales))

	return nil
}

// SplitModuleScales divides the recorded module deployments by an install
// request. The first result holds the deployments in the namespace the request's
// module installs into, which the install creates again. The second holds the
// rest, which keep running. An empty or unreadable request leaves every
// deployment in the second result.
func SplitModuleScales(
	request string,
	scales []ModuleDeploymentScale,
) ([]ModuleDeploymentScale, []ModuleDeploymentScale) {
	// replace nothing when no module is requested
	module, moduleNamespace, err := moduleFromInstallRequest(request)
	if err != nil {
		return nil, scales
	}

	// split by the namespace the request's module installs into
	namespace := moduleInstallTargetNamespace(module, moduleNamespace)
	var replaced, others []ModuleDeploymentScale
	for _, scale := range scales {
		if scale.Namespace == namespace {
			replaced = append(replaced, scale)
			continue
		}
		others = append(others, scale)
	}

	return replaced, others
}

// DeleteModuleNamespaces deletes each namespace that holds a recorded module
// deployment, so a module install creates the module's workloads again from the
// module's own installer.
func DeleteModuleNamespaces(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
	scales []ModuleDeploymentScale,
) error {
	// collect each module namespace once
	var namespaces []string
	seen := map[string]bool{}
	for _, scale := range scales {
		if scale.Namespace == "" || seen[scale.Namespace] {
			continue
		}
		seen[scale.Namespace] = true
		namespaces = append(namespaces, scale.Namespace)
	}
	if len(namespaces) == 0 {
		return nil
	}

	// delete them together
	if err := DeleteNamespaces(kubeClient, mapper, namespaces); err != nil {
		return fmt.Errorf("failed to delete module namespaces %s: %w", strings.Join(namespaces, ", "), err)
	}
	fmt.Printf("Info: deleted module namespace(s) %s\n", strings.Join(namespaces, ", "))

	return nil
}

// resumeDeploymentReplicas patches a stopped deployment back to its prior
// replica count and removes the count recorded before it stopped, in one patch.
// A later backup then reads zero replicas with no record as a deployment it did
// not stop. A missing deployment is success.
func (cpi *ControlPlaneInstaller) resumeDeploymentReplicas(
	kubeClient dynamic.Interface,
	namespace string,
	name string,
	replicas int64,
) error {
	// set the count and null the annotation in one merge patch
	patch := fmt.Appendf(
		nil,
		`{"metadata":{"annotations":{%q:null}},"spec":{"replicas":%d}}`,
		replicasBeforeRestoreAnnotation,
		replicas,
	)
	_, err := kubeClient.Resource(deploymentGVR).Namespace(namespace).Patch(
		context.Background(),
		name,
		"application/strategic-merge-patch+json",
		patch,
		metav1.PatchOptions{},
	)
	if err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("failed to resume deployment %s/%s at %d: %w", namespace, name, replicas, err)
	}

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
