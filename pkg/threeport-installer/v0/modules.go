package v0

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

// DiscoverModuleNamespaces returns the unique namespaces parsed from non-core
// module controller deployment names, omitting the control plane namespace.
func (cpi *ControlPlaneInstaller) DiscoverModuleNamespaces(
	apiClient *http.Client,
	apiEndpoint string,
) ([]string, error) {
	// get registered module APIs
	moduleApis, err := client.GetModuleApis(apiClient, apiEndpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to get registered module APIs: %w", err)
	}

	var namespaces []string
	// exclude the control plane namespace
	seen := map[string]bool{cpi.Opts.Namespace: true}

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
			// record each new namespace from a namespace/name deployment name
			if controller.DeploymentName == nil {
				continue
			}
			namespace, _, qualified := strings.Cut(*controller.DeploymentName, "/")
			if !qualified || namespace == "" {
				continue
			}
			if seen[namespace] {
				continue
			}
			seen[namespace] = true
			namespaces = append(namespaces, namespace)
		}
	}

	return namespaces, nil
}

// ScaleDownModules scales each deployment with a non-zero replica count to
// zero and returns those counts once no ready replicas remain.
func (cpi *ControlPlaneInstaller) ScaleDownModules(
	kubeClient dynamic.Interface,
	namespaces []string,
) ([]ModuleDeploymentScale, error) {
	var scales []ModuleDeploymentScale

	for _, namespace := range namespaces {
		// list deployments in the module namespace
		deployList, err := kubeClient.Resource(deploymentGVR).Namespace(namespace).List(
			context.Background(), metav1.ListOptions{},
		)
		if err != nil {
			return nil, fmt.Errorf("failed to list deployments in module namespace %s: %w", namespace, err)
		}

		for _, deployment := range deployList.Items {
			// leave a deployment the installer does not own at its current replica count
			if deployment.GetLabels()[LabelManagedBy] != LabelManagedByValue {
				continue
			}
			// skip a deployment already at zero so it stays out of the record
			name := deployment.GetName()
			replicas, _, _ := util.NestedInt64OrFloat64(deployment.Object, "spec", "replicas")
			if replicas == 0 {
				continue
			}

			// scale the deployment to zero and record its prior count
			if err := cpi.setDeploymentReplicas(kubeClient, namespace, name, 0); err != nil {
				return nil, err
			}
			scales = append(scales, ModuleDeploymentScale{
				Namespace: namespace,
				Name:      name,
				Replicas:  replicas,
			})
		}
	}

	// return when nothing was scaled
	if len(scales) == 0 {
		return scales, nil
	}

	// report the scale-down
	fmt.Printf("Info: scaled %d module deployment(s) to 0 across %s\n", len(scales), strings.Join(namespaces, ", "))

	// wait until no ready replicas remain
	if err := util.Retry(60, 3, func() error {
		// count ready replicas still present
		pending := 0
		for _, namespace := range namespaces {
			current, err := kubeClient.Resource(deploymentGVR).Namespace(namespace).List(
				context.Background(), metav1.ListOptions{},
			)
			if err != nil {
				return fmt.Errorf("failed to list deployments while waiting for module scale-down: %w", err)
			}
			for _, deployment := range current.Items {
				ready, _, _ := util.NestedInt64OrFloat64(deployment.Object, "status", "readyReplicas")
				if ready > 0 {
					pending += int(ready)
				}
			}
		}
		if pending > 0 {
			return fmt.Errorf("%d module replica(s) still present", pending)
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("module deployments did not scale to zero: %w", err)
	}

	return scales, nil
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
	patch := []byte(fmt.Sprintf(`{"spec":{"replicas":%d}}`, replicas))
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
