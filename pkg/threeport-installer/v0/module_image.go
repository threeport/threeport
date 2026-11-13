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

// ModuleKebab returns the image-name segment of a module API name.
// A name that does not end in "-module-api" returns an empty string.
func ModuleKebab(moduleAPIName string) string {
	name := path.Base(moduleAPIName)
	if !strings.HasSuffix(name, moduleAPINameSuffix) {
		return ""
	}

	return strings.TrimSuffix(name, moduleAPINameSuffix)
}

// ModuleImage returns namespace/name:tag.
// An empty namespace, name, or tag returns an empty string.
func ModuleImage(namespace, name, tag string) string {
	if namespace == "" || name == "" || tag == "" {
		return ""
	}

	return fmt.Sprintf("%s/%s:%s", namespace, name, tag)
}

// ModuleRestAPIImage returns the module API image reference.
func ModuleRestAPIImage(namespace, kebab, tag string) string {
	return ModuleImage(namespace, fmt.Sprintf("%s%s-rest-api", moduleImageNamePrefix, kebab), tag)
}

// ModuleMigratorImage returns the module database migrator image reference.
func ModuleMigratorImage(namespace, kebab, tag string) string {
	return ModuleImage(namespace, fmt.Sprintf("%s%s-database-migrator", moduleImageNamePrefix, kebab), tag)
}

// ModuleControllerImage returns the controller image reference.
// The image name is the last segment of the recorded deployment name.
func ModuleControllerImage(namespace, deploymentName, tag string) string {
	return ModuleImage(namespace, path.Base(deploymentName), tag)
}

// ModuleTptctlImage returns the module tptctl plugin image reference.
func ModuleTptctlImage(namespace, kebab, tag string) string {
	return ModuleImage(namespace, fmt.Sprintf("%s%s-tptctl", moduleImageNamePrefix, kebab), tag)
}

// ModuleAPIDeploymentName returns the module API deployment name.
func ModuleAPIDeploymentName(kebab string) string {
	return fmt.Sprintf("%s%s-api-server", moduleImageNamePrefix, kebab)
}

// ModuleControllerContainerName returns the container name on a controller deployment.
func ModuleControllerContainerName(deploymentName string) string {
	return strings.TrimPrefix(path.Base(deploymentName), moduleImageNamePrefix)
}

// ModuleCopiedSecretNames returns the secrets the installer copies into a module namespace.
func ModuleCopiedSecretNames() []string {
	return []string{
		dbRootCertSecretName,
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

// SetNamedContainerImage sets one container image on a deployment.
// An empty image leaves the container unchanged.
func SetNamedContainerImage(deployment *unstructured.Unstructured, listKey, containerName, image string) error {
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

// RestartDeployment sets the pod-template restart annotation.
func RestartDeployment(deployment *unstructured.Unstructured, at time.Time) error {
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
