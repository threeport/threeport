package v0

import (
	"fmt"
	"path"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	// ModuleApiConfigSecretName is the secret the module API reads at startup.
	ModuleApiConfigSecretName = "module-api-config"

	moduleAPINameSuffix         = "-module-api"
	moduleImageNamePrefix       = "threeport-"
	moduleAPIContainerName      = "api-server"
	moduleMigratorContainerName = "database-migrator"
	moduleRestartAnnotation     = "kubectl.kubernetes.io/restartedAt"
	controllerConfigSecretName  = "controller-config"
)

// moduleKebab returns the image-name segment of a module API name.
// A name that does not end in "-module-api" returns an empty string.
func moduleKebab(moduleAPIName string) string {
	name := path.Base(moduleAPIName)
	if !strings.HasSuffix(name, moduleAPINameSuffix) {
		return ""
	}

	return strings.TrimSuffix(name, moduleAPINameSuffix)
}

// moduleImage returns namespace/name:tag.
// An empty namespace, name, or tag returns an empty string.
func moduleImage(namespace, name, tag string) string {
	if namespace == "" || name == "" || tag == "" {
		return ""
	}

	return fmt.Sprintf("%s/%s:%s", namespace, name, tag)
}

// moduleRestAPIImage returns the module API image reference.
func moduleRestAPIImage(namespace, kebab, tag string) string {
	return moduleImage(namespace, fmt.Sprintf("%s%s-rest-api", moduleImageNamePrefix, kebab), tag)
}

// moduleMigratorImage returns the module database migrator image reference.
func moduleMigratorImage(namespace, kebab, tag string) string {
	return moduleImage(namespace, fmt.Sprintf("%s%s-database-migrator", moduleImageNamePrefix, kebab), tag)
}

// moduleControllerImage returns the controller image reference.
// The image name is the last segment of the recorded deployment name.
func moduleControllerImage(namespace, deploymentName, tag string) string {
	return moduleImage(namespace, path.Base(deploymentName), tag)
}

// modulePluginImage returns the module plugin image reference.
func modulePluginImage(namespace, kebab, tag string) string {
	return moduleImage(namespace, fmt.Sprintf("%s%s-plugin", moduleImageNamePrefix, kebab), tag)
}

// ModuleInstallImages returns the images a module upgrade pulls: the API server
// and the database migrator, plus the plugin that runs the install job when the
// upgrade runs one. A module name with no image segment, or an empty namespace or
// tag, returns none.
func ModuleInstallImages(imageNamespace, moduleName, tag string, installJob bool) []string {
	kebab := moduleKebab(moduleName)
	if kebab == "" {
		return nil
	}

	candidates := []string{
		moduleRestAPIImage(imageNamespace, kebab, tag),
		moduleMigratorImage(imageNamespace, kebab, tag),
	}
	if installJob {
		candidates = append(candidates, modulePluginImage(imageNamespace, kebab, tag))
	}

	var images []string
	for _, image := range candidates {
		if image != "" {
			images = append(images, image)
		}
	}

	return images
}

// moduleAPIDeploymentName returns the module API deployment name.
func moduleAPIDeploymentName(kebab string) string {
	return fmt.Sprintf("%s%s-api-server", moduleImageNamePrefix, kebab)
}

// moduleControllerContainerName returns the container name on a controller deployment.
func moduleControllerContainerName(deploymentName string) string {
	return strings.TrimPrefix(path.Base(deploymentName), moduleImageNamePrefix)
}

// ModuleCopiedSecretNames returns the secrets the installer copies into a
// module namespace.  The database's root client certificate isn't among
// them: a module connects as the threeport user, to the control plane's
// database, which the control plane creates.
func ModuleCopiedSecretNames() []string {
	return []string{
		dbThreeportCertSecretName,
		EncryptionKeySecret,
		controllerConfigSecretName,
		ThreeportApiConfigSecret,
		NatsCACertSecretName,
	}
}

// ModuleApiConfigSecret returns the module API config secret.
// The rest API reads the env key at /etc/threeport/mod/env.
func ModuleApiConfigSecret(namespace, apiServerName string, authEnabled bool) *unstructured.Unstructured {
	// write the endpoint the module API serves inside its namespace
	config := fmt.Sprintf(
		"MODULE_API_ENDPOINT=%s.%s.svc.cluster.local\nMODULE_NAMESPACE=%s\nTHREEPORT_AUTH_ENABLED=%t\n",
		apiServerName,
		namespace,
		namespace,
		authEnabled,
	)

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name":      ModuleApiConfigSecretName,
				"namespace": namespace,
			},
			"type": "Opaque",
			"stringData": map[string]interface{}{
				"env": config,
			},
		},
	}
}

// setNamedContainerImage sets one container image on a deployment.
// An empty image leaves the container unchanged.
func setNamedContainerImage(deployment *unstructured.Unstructured, listKey, containerName, image string) error {
	if image == "" || containerName == "" {
		return nil
	}

	// find the named container
	containers, found, err := unstructured.NestedSlice(deployment.Object, "spec", "template", "spec", listKey)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", listKey, err)
	}
	if !found {
		return fmt.Errorf("deployment has no %s", listKey)
	}

	updated := false
	for i, item := range containers {
		container, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		name, _, nameErr := unstructured.NestedString(container, "name")
		if nameErr != nil {
			return fmt.Errorf("failed to read container name: %w", nameErr)
		}
		if name != containerName {
			continue
		}
		container["image"] = image
		namespace, tag := splitModuleImage(image)
		if namespace != "" && tag != "" {
			setContainerEnvValue(container, "MODULE_IMAGE_NAMESPACE", namespace)
			setContainerEnvValue(container, "MODULE_IMAGE_TAG", tag)
		}
		containers[i] = container
		updated = true
	}
	if !updated {
		return fmt.Errorf("deployment has no %s container %s", listKey, containerName)
	}

	// write the container list back
	if err := unstructured.SetNestedSlice(deployment.Object, containers, "spec", "template", "spec", listKey); err != nil {
		return fmt.Errorf("failed to set %s: %w", listKey, err)
	}

	return nil
}

// splitModuleImage returns the repository and tag of a container image.
// The repository is everything before the last slash, so a registry port stays in it.
func splitModuleImage(image string) (string, string) {
	slash := strings.LastIndex(image, "/")
	if slash <= 0 {
		return "", ""
	}
	name := image[slash+1:]
	colon := strings.LastIndex(name, ":")
	if colon <= 0 || colon == len(name)-1 {
		return "", ""
	}
	return image[:slash], name[colon+1:]
}

// setContainerEnvValue sets one environment value on a container.
func setContainerEnvValue(container map[string]interface{}, name, value string) {
	// replace an existing entry, or append one
	env, _, _ := unstructured.NestedSlice(container, "env")
	for i, item := range env {
		entry, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		current, _, _ := unstructured.NestedString(entry, "name")
		if current != name {
			continue
		}
		entry["value"] = value
		env[i] = entry
		_ = unstructured.SetNestedSlice(container, env, "env")
		return
	}
	env = append(env, map[string]interface{}{"name": name, "value": value})
	_ = unstructured.SetNestedSlice(container, env, "env")
}

// restartDeployment sets the pod-template restart annotation.
func restartDeployment(deployment *unstructured.Unstructured, at time.Time) error {
	// set the restart annotation
	annotations, _, err := unstructured.NestedStringMap(
		deployment.Object,
		"spec",
		"template",
		"metadata",
		"annotations",
	)
	if err != nil {
		return fmt.Errorf("failed to read pod template annotations: %w", err)
	}
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[moduleRestartAnnotation] = at.UTC().Format(time.RFC3339)

	if err := unstructured.SetNestedStringMap(
		deployment.Object,
		annotations,
		"spec",
		"template",
		"metadata",
		"annotations",
	); err != nil {
		return fmt.Errorf("failed to set pod template annotations: %w", err)
	}

	return nil
}
