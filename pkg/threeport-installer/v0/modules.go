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

// Module tables share the control plane database, so a drop of that
// database wipes module schema. The records that name those module
// deployments live in the same database and must be read first. The
// deployments are scaled to zero before the drop so they stop writing,
// then scaled back so the module APIs recreate schema and re-register.

// ModuleDeploymentScale is a recorded replica count for one module
// deployment, used to restore that count after a scale to zero.
type ModuleDeploymentScale struct {
	Namespace string
	Name      string
	Replicas  int64
}

// DiscoverModuleNamespaces returns the unique namespaces of registered
// non-core module controllers, omitting the control plane namespace.
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
		// skip the core threeport API
		if moduleApi.Core != nil && *moduleApi.Core {
			continue
		}
		if moduleApi.ID == nil {
			continue
		}

		// get controllers registered by this module API
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
			if controller.DeploymentName == nil {
				continue
			}
			// parse namespace from namespace/name
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

// ScaleDownModules scales every deployment in each module namespace to
// zero replicas, waits until none are ready, and returns the prior counts.
func (cpi *ControlPlaneInstaller) ScaleDownModules(
	kubeClient dynamic.Interface,
	namespaces []string,
) ([]ModuleDeploymentScale, error) {
	var scales []ModuleDeploymentScale

	// scale each running deployment to zero and record its replica count
	for _, namespace := range namespaces {
		deployList, err := kubeClient.Resource(deploymentGVR).Namespace(namespace).List(
			context.Background(), metav1.ListOptions{},
		)
		if err != nil {
			return nil, fmt.Errorf("failed to list deployments in module namespace %s: %w", namespace, err)
		}

		for _, deployment := range deployList.Items {
			name := deployment.GetName()
			replicas, _, _ := util.NestedInt64OrFloat64(deployment.Object, "spec", "replicas")
			// skip deployments already at zero so they stay out of the record
			if replicas == 0 {
				continue
			}

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

	if len(scales) == 0 {
		return scales, nil
	}

	fmt.Printf("Info: scaled %d module deployment(s) to 0 across %s\n", len(scales), strings.Join(namespaces, ", "))

	// wait until no ready replicas remain
	if err := util.Retry(60, 3, func() error {
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

// RestoreModuleScale restores each recorded deployment to its original
// replica count. It does not wait for the pods to become ready.
func (cpi *ControlPlaneInstaller) RestoreModuleScale(
	kubeClient dynamic.Interface,
	scales []ModuleDeploymentScale,
) error {
	if len(scales) == 0 {
		return nil
	}

	for _, scale := range scales {
		if err := cpi.setDeploymentReplicas(
			kubeClient, scale.Namespace, scale.Name, scale.Replicas,
		); err != nil {
			return err
		}
	}

	fmt.Printf("Info: restored %d module deployment(s) to their original replica count\n", len(scales))

	return nil
}

// setDeploymentReplicas patches a deployment's replica count and
// treats a missing deployment as nothing to do.
func (cpi *ControlPlaneInstaller) setDeploymentReplicas(
	kubeClient dynamic.Interface,
	namespace string,
	name string,
	replicas int64,
) error {
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
