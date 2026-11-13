package v0

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"

	api "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// RedeployInstalledModules points each non-core module at this control plane.
// The previous control plane and its namespace stay in place.
func (cpi *ControlPlaneInstaller) RedeployInstalledModules(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
	apiClient *http.Client,
	apiEndpoint string,
) error {
	if mapper == nil {
		return fmt.Errorf("failed to retarget installed modules: no REST mapper")
	}

	// list the module APIs restored with the database
	moduleApis, err := client.GetModuleApis(apiClient, apiEndpoint)
	if err != nil {
		return fmt.Errorf("failed to list module APIs: %w", err)
	}

	// list the controllers separately because the API list does not preload them
	moduleControllers, err := client.GetModuleControllers(apiClient, apiEndpoint)
	if err != nil {
		return fmt.Errorf("failed to list module controllers: %w", err)
	}
	controllersByAPI := map[uint][]string{}
	if moduleControllers != nil {
		for _, moduleController := range *moduleControllers {
			if moduleController.ModuleApiID == nil || moduleController.DeploymentName == nil {
				continue
			}
			controllersByAPI[*moduleController.ModuleApiID] = append(
				controllersByAPI[*moduleController.ModuleApiID],
				*moduleController.DeploymentName,
			)
		}
	}

	// retarget each non-core module
	if moduleApis == nil {
		return nil
	}
	for _, moduleAPI := range *moduleApis {
		if err := cpi.redeployInstalledModule(
			kubeClient,
			*mapper,
			moduleAPI,
			controllersByAPI,
			time.Now(),
		); err != nil {
			return err
		}
	}

	return nil
}

// redeployInstalledModule copies connection secrets, admits the module to the
// database, and restarts its workloads. A core module is left as installed.
func (cpi *ControlPlaneInstaller) redeployInstalledModule(
	kubeClient dynamic.Interface,
	mapper meta.RESTMapper,
	moduleAPI api.ModuleApi,
	controllersByAPI map[uint][]string,
	restartedAt time.Time,
) error {
	// leave a core module on the control plane installer
	if moduleAPI.Core != nil && *moduleAPI.Core {
		return nil
	}

	// read the module namespace from its registered endpoint
	apiServer, ok := moduleApiServerFromEndpoint(moduleAPI.Endpoint)
	if !ok {
		return fmt.Errorf(
			"module %s has no cluster endpoint to retarget",
			util.DerefString(moduleAPI.Name),
		)
	}
	kebab := ModuleKebab(util.DerefString(moduleAPI.Name))
	if kebab == "" {
		return fmt.Errorf(
			"module %s has no image name",
			util.DerefString(moduleAPI.Name),
		)
	}

	// copy the secrets the installer copies
	for _, secretName := range ModuleCopiedSecretNames() {
		if err := CopySecret(
			kubeClient,
			mapper,
			secretName,
			cpi.Opts.Namespace,
			apiServer.Namespace,
		); err != nil {
			return fmt.Errorf(
				"failed to copy secret %s into %s: %w",
				secretName,
				apiServer.Namespace,
				err,
			)
		}
	}

	// write the module API config for this namespace
	if _, err := kube.CreateOrUpdateResource(
		ModuleApiConfigSecret(apiServer.Namespace, apiServer.Service, cpi.Opts.AuthEnabled),
		kubeClient,
		mapper,
	); err != nil {
		return fmt.Errorf("failed to write module API config in %s: %w", apiServer.Namespace, err)
	}

	// admit the module pods to this control plane database
	service, err := kubeClient.Resource(serviceGVR).Namespace(apiServer.Namespace).Get(
		context.Background(),
		apiServer.Service,
		metav1.GetOptions{},
	)
	if err != nil {
		return fmt.Errorf(
			"failed to get module API service %s/%s: %w",
			apiServer.Namespace,
			apiServer.Service,
			err,
		)
	}
	selector, _, err := unstructured.NestedStringMap(service.Object, "spec", "selector")
	if err != nil {
		return fmt.Errorf(
			"failed to read the selector of service %s/%s: %w",
			apiServer.Namespace,
			apiServer.Service,
			err,
		)
	}
	if err := InstallModuleDatabasePolicy(
		kubeClient,
		mapper,
		cpi.Opts.Namespace,
		apiServer.Namespace,
		apiServer.Service,
		selector,
	); err != nil {
		return fmt.Errorf("failed to install the module database policy: %w", err)
	}

	// set images when both are recorded, then restart
	imageNamespace, imageTag := recordedModuleImages(&moduleAPI)
	if err := restartModuleDeployment(
		kubeClient,
		apiServer.Namespace,
		ModuleAPIDeploymentName(kebab),
		map[string]string{
			"containers":     ModuleRestAPIImage(imageNamespace, kebab, imageTag),
			"initContainers": ModuleMigratorImage(imageNamespace, kebab, imageTag),
		},
		map[string]string{
			"containers":     moduleAPIContainerName,
			"initContainers": moduleMigratorContainerName,
		},
		restartedAt,
	); err != nil {
		return err
	}

	// restart each controller
	if moduleAPI.ID == nil {
		return nil
	}
	for _, deploymentName := range controllersByAPI[*moduleAPI.ID] {
		namespace, name, splitErr := splitDeploymentName(deploymentName)
		if splitErr != nil {
			return fmt.Errorf("module %s: %w", util.DerefString(moduleAPI.Name), splitErr)
		}
		if err := restartModuleDeployment(
			kubeClient,
			namespace,
			name,
			map[string]string{
				"containers": ModuleControllerImage(imageNamespace, name, imageTag),
			},
			map[string]string{
				"containers": ModuleControllerContainerName(name),
			},
			restartedAt,
		); err != nil {
			return err
		}
	}

	return nil
}

// recordedModuleImages returns the repository and tag stored on a module API.
// A core module, or a module missing either value, returns empty strings.
func recordedModuleImages(moduleAPI *api.ModuleApi) (string, string) {
	// leave the deployment image unless both coordinates are recorded
	if moduleAPI == nil || (moduleAPI.Core != nil && *moduleAPI.Core) {
		return "", ""
	}

	namespace := strings.TrimSpace(util.DerefString(moduleAPI.ImageNamespace))
	tag := strings.TrimSpace(util.DerefString(moduleAPI.ImageTag))
	if namespace == "" || tag == "" {
		return "", ""
	}

	return namespace, tag
}

// splitDeploymentName splits a recorded namespace/name deployment reference.
func splitDeploymentName(deploymentName string) (string, string, error) {
	namespace, name, ok := strings.Cut(deploymentName, "/")
	if !ok || namespace == "" || name == "" || strings.Contains(name, "/") {
		return "", "", fmt.Errorf("deployment %q is not namespace/name", deploymentName)
	}

	return namespace, name, nil
}

// restartModuleDeployment sets the named container images and restarts the deployment.
// An empty image leaves that container unchanged.
func restartModuleDeployment(
	kubeClient dynamic.Interface,
	namespace string,
	name string,
	images map[string]string,
	containerNames map[string]string,
	restartedAt time.Time,
) error {
	// read the deployment
	deployment, err := kubeClient.Resource(deploymentGVR).Namespace(namespace).Get(
		context.Background(),
		name,
		metav1.GetOptions{},
	)
	if err != nil {
		return fmt.Errorf("failed to get deployment %s/%s: %w", namespace, name, err)
	}

	// set each recorded image
	for listKey, image := range images {
		if err := SetNamedContainerImage(deployment, listKey, containerNames[listKey], image); err != nil {
			return fmt.Errorf("failed to set the image on deployment %s/%s: %w", namespace, name, err)
		}
	}

	// restart so the init containers run against this database
	if err := RestartDeployment(deployment, restartedAt); err != nil {
		return fmt.Errorf("failed to restart deployment %s/%s: %w", namespace, name, err)
	}
	if _, err := kubeClient.Resource(deploymentGVR).Namespace(namespace).Update(
		context.Background(),
		deployment,
		metav1.UpdateOptions{},
	); err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("failed to update deployment %s/%s: %w", namespace, name, err)
	}

	return nil
}
