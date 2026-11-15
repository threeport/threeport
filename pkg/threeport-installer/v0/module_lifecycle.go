package v0

import (
	"fmt"
	"net/http"
	"time"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	api "github.com/threeport/threeport/pkg/api/v0"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

const moduleWorkloadPrefix = "threeport-"

// HTTPClientForInstance builds an API client from the certificates stored on a control plane.
// An empty client certificate means auth is off.
func HTTPClientForInstance(inst *api.ControlPlaneInstance) (*http.Client, error) {
	// auth is off when the row has no client certificate
	if util.DerefString(inst.ClientCert) == "" {
		return client_lib.GetHTTPClient(false, "", "", "", "")
	}

	// decode the certificates the row stores as base64
	caCert, err := util.Base64Decode(util.DerefString(inst.CACert))
	if err != nil {
		return nil, fmt.Errorf("failed to decode ca certificate: %w", err)
	}
	clientCert, err := util.Base64Decode(*inst.ClientCert)
	if err != nil {
		return nil, fmt.Errorf("failed to decode client certificate: %w", err)
	}
	clientKey, err := util.Base64Decode(util.DerefString(inst.ClientKey))
	if err != nil {
		return nil, fmt.Errorf("failed to decode client key: %w", err)
	}

	return client_lib.GetHTTPClient(true, caCert, clientCert, clientKey, "")
}

// ReconcileInstalledModuleImages applies recorded module images to the deployments that exist.
// A missing deployment is left alone.
func ReconcileInstalledModuleImages(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
	apiClient *http.Client,
	apiEndpoint string,
	inst *api.ControlPlaneInstance,
) error {
	// list the modules and the controllers that belong to them
	modules, err := client.GetModuleApis(apiClient, apiEndpoint)
	if err != nil {
		return fmt.Errorf("failed to list module APIs: %w", err)
	}
	controllers, err := client.GetModuleControllers(apiClient, apiEndpoint)
	if err != nil {
		return fmt.Errorf("failed to list module controllers: %w", err)
	}
	byAPI := map[uint][]api.ModuleController{}
	if controllers != nil {
		for _, controller := range *controllers {
			if controller.ModuleApiID == nil {
				continue
			}
			byAPI[*controller.ModuleApiID] = append(byAPI[*controller.ModuleApiID], controller)
		}
	}

	// retag each module that has image coordinates
	if modules == nil {
		return nil
	}
	for _, module := range *modules {
		// a core module has no image of its own
		if module.Core != nil && *module.Core {
			continue
		}

		// bring a stopped module back even when it has no recorded image
		kebab := moduleKebab(util.DerefString(module.Name))
		workloadNamespace := moduleWorkloadNamespace(&module)
		imageNamespace, imageTag := recordedModuleImages(&module)
		controlPlaneNamespace := util.DerefString(inst.Namespace)
		if _, err := refreshModuleTrust(
			kubeClient, workloadNamespace, controlPlaneNamespace, inst,
		); err != nil {
			return err
		}
		if err := retargetModuleDeployment(
			kubeClient, workloadNamespace, moduleAPIDeploymentName(kebab), controlPlaneNamespace,
		); err != nil {
			return err
		}
		if imageNamespace == "" || imageTag == "" {
			if err := resumeModuleByName(
				kubeClient, workloadNamespace, moduleAPIDeploymentName(kebab),
			); err != nil {
				return err
			}
		} else if err := retagModuleAPI(
			kubeClient, mapper, &module, imageNamespace, imageTag, controlPlaneNamespace,
		); err != nil {
			return err
		}
		if module.ID == nil {
			continue
		}
		for _, controller := range byAPI[*module.ID] {
			namespace, recorded, splitErr := splitDeploymentName(util.DerefString(controller.DeploymentName))
			if splitErr == nil {
				if name, nameErr := installedModuleDeployment(kubeClient, namespace, recorded, kebab); nameErr == nil {
					if err := retargetModuleDeployment(
						kubeClient, namespace, name, controlPlaneNamespace,
					); err != nil {
						return err
					}
				}
			}
			if imageNamespace == "" || imageTag == "" {
				if err := resumeRecordedController(kubeClient, &controller, kebab); err != nil {
					return err
				}
				continue
			}
			if err := retagModuleController(
				kubeClient, mapper, &controller, kebab, imageNamespace, imageTag, controlPlaneNamespace,
			); err != nil {
				return err
			}
		}
	}

	return nil
}

// retagModuleAPI restarts the API deployment when its image differs.
// A missing deployment is an error.
func retagModuleAPI(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
	module *api.ModuleApi,
	imageNamespace, imageTag, controlPlaneNamespace string,
) error {
	// read the API deployment
	kebab := moduleKebab(util.DerefString(module.Name))
	namespace := moduleWorkloadNamespace(module)
	name := moduleAPIDeploymentName(kebab)
	deployment, err := kube.GetResource(
		"apps", "v1", "Deployment", namespace, name, kubeClient, *mapper,
	)
	if err != nil {
		return fmt.Errorf("failed to get module API deployment %s/%s: %w", namespace, name, err)
	}
	if err := resumeStoppedDeployment(kubeClient, namespace, deployment); err != nil {
		return err
	}

	// leave a deployment whose containers are missing or already current
	apiImage := moduleRestAPIImage(imageNamespace, kebab, imageTag)
	migratorImage := moduleMigratorImage(imageNamespace, kebab, imageTag)
	currentAPI, apiOK := namedContainerImage(deployment, "containers", moduleAPIContainerName)
	currentMigrator, migratorOK := namedContainerImage(
		deployment, "initContainers", moduleMigratorContainerName,
	)
	if !apiOK || !migratorOK || (currentAPI == apiImage && currentMigrator == migratorImage) {
		return nil
	}

	// restart onto the recorded images
	return restartModuleDeployment(
		kubeClient,
		namespace,
		name,
		controlPlaneNamespace,
		map[string]string{
			"containers":     apiImage,
			"initContainers": migratorImage,
		},
		map[string]string{
			"containers":     moduleAPIContainerName,
			"initContainers": moduleMigratorContainerName,
		},
		time.Now().UTC(),
	)
}

// retagModuleController restarts one controller deployment when its image differs.
// A missing controller deployment is left alone.
func retagModuleController(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
	controller *api.ModuleController,
	kebab, imageNamespace, imageTag, controlPlaneNamespace string,
) error {
	// read the controller deployment the install created
	namespace, recorded, err := splitDeploymentName(util.DerefString(controller.DeploymentName))
	if err != nil {
		return nil
	}
	name, err := installedModuleDeployment(kubeClient, namespace, recorded, kebab)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	deployment, err := kube.GetResource(
		"apps", "v1", "Deployment", namespace, name, kubeClient, *mapper,
	)
	if k8serrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to get module controller %s/%s: %w", namespace, name, err)
	}
	if err := resumeStoppedDeployment(kubeClient, namespace, deployment); err != nil {
		return err
	}

	// leave a deployment whose container is missing or already current
	container := moduleControllerContainerName(name)
	image := moduleControllerImage(imageNamespace, name, imageTag)
	current, ok := namedContainerImage(deployment, "containers", container)
	if !ok || current == image {
		return nil
	}

	// restart onto the recorded image
	return restartModuleDeployment(
		kubeClient,
		namespace,
		name,
		controlPlaneNamespace,
		map[string]string{"containers": image},
		map[string]string{"containers": container},
		time.Now().UTC(),
	)
}

// WorkloadRESTMapper returns a mapper for the workloads a control plane upgrade touches.
func WorkloadRESTMapper() meta.RESTMapper {
	// register the kinds the upgrade reads and writes
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{
		{Version: "v1"},
		{Group: "apps", Version: "v1"},
		{Group: "batch", Version: "v1"},
		{Group: "rbac.authorization.k8s.io", Version: "v1"},
	})
	mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "Secret"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "ServiceAccount"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "Namespace"}, meta.RESTScopeRoot)
	mapper.Add(
		schema.GroupVersionKind{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "ClusterRoleBinding"},
		meta.RESTScopeRoot,
	)
	mapper.Add(
		schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
		meta.RESTScopeNamespace,
	)
	mapper.Add(
		schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "Job"},
		meta.RESTScopeNamespace,
	)

	return mapper
}

// moduleWorkloadNamespace returns the namespace the module workloads run in.
func moduleWorkloadNamespace(module *api.ModuleApi) string {
	// prefer the namespace the module endpoint records
	if server, ok := moduleApiServerFromEndpoint(module.Endpoint); ok && server.Namespace != "" {
		return server.Namespace
	}

	return moduleWorkloadPrefix + moduleKebab(util.DerefString(module.Name))
}

// namedContainerImage returns the image of one container.
func namedContainerImage(obj *unstructured.Unstructured, listKey, containerName string) (string, bool) {
	// walk the named container list
	list, found, err := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", listKey)
	if err != nil || !found {
		return "", false
	}
	for _, item := range list {
		container, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		name, _, _ := unstructured.NestedString(container, "name")
		if name != containerName {
			continue
		}
		image, _, _ := unstructured.NestedString(container, "image")
		return image, image != ""
	}

	return "", false
}
