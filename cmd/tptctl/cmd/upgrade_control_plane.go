/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	cli "github.com/threeport/threeport/pkg/cli/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
	installer "github.com/threeport/threeport/pkg/threeport-installer/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	kubemetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var updateImageTag string

// UpCmd represents the create threeport command
var UpgradeControlPlaneCmd = &cobra.Command{
	Use:     "control-plane",
	Example: "tptctl upgrade control-plane --version=v0.5.0",
	Short:   "Upgrades the version of the Threeport control plane",
	Long: `Update the image tag on existing control plane Deployments only.

Does not recreate manifests, RBAC, or configmaps. Use tptctl reinstall
for those.`,
	SilenceUsage: true,
	PreRun:       CommandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, config, apiEndpoint, requestedControlPlane := GetClientContext(cmd)

		encyptionKey, err := config.GetThreeportEncryptionKey(requestedControlPlane)
		if err != nil {
			cli.Error("failed to retrieve encryption key for control plane:", err)
			os.Exit(1)
		}

		// get the kubernetes runtime instance object
		kubernetesRuntimeInstance, err := client.GetThreeportControlPlaneKubernetesRuntimeInstance(
			apiClient,
			apiEndpoint,
		)
		if err != nil {
			cli.Error("failed to retrieve kubernetes runtime instance from threeport API:", err)
			os.Exit(1)
		}

		// get the kubernetes runtime instance object
		controlPlaneInstance, err := client.GetSelfControlPlaneInstance(
			apiClient,
			apiEndpoint,
		)
		if err != nil {
			cli.Error("failed to retrieve self control plane instance from threeport API:", err)
			os.Exit(1)
		}

		var dynamicKubeClient dynamic.Interface
		var mapper *meta.RESTMapper
		dynamicKubeClient, mapper, err = kube.GetClient(
			kubernetesRuntimeInstance,
			false,
			apiClient,
			apiEndpoint,
			encyptionKey,
		)
		if err != nil {
			cli.Error("failed to get kube client:", err)
			os.Exit(1)
		}

		err = updateDeploymentImageTag(updateImageTag, *controlPlaneInstance.Namespace, dynamicKubeClient, mapper)
		if err != nil {
			cli.Error("failed to update threeport deployment image tag", err)
			os.Exit(1)
		}

		// children created from here on default to the images this control plane
		// runs, so record the tag the deployments were just moved to
		if _, err := client.UpdateControlPlaneInstance(
			apiClient,
			apiEndpoint,
			&api_v0.ControlPlaneInstance{
				Common:   api_v0.Common{ID: controlPlaneInstance.ID},
				ImageTag: &updateImageTag,
			},
		); err != nil {
			cli.Error("deployments were updated but the new image tag could not be recorded for child control planes:", err)
			os.Exit(1)
		}

		cli.Complete(fmt.Sprintf("Succesfully updates all threeport deployments to image: %s", updateImageTag))
	},
}

func updateDeploymentImageTag(imageTag string, namespace string, dynamicKubeClient dynamic.Interface, mapper *meta.RESTMapper) error {
	// Update Rest-Api
	deployment, err := kube.GetResource(
		"apps",
		"v1",
		"Deployment",
		namespace,
		installer.ThreeportRestApi.ServiceResourceName,
		dynamicKubeClient,
		*mapper,
	)
	if err != nil {
		return fmt.Errorf("failed to get kubernetes deployment for rest-api: %w", err)
	}

	// return the rest-api image update error
	apiReplaced, err := updateImageTagInDeployment(deployment, imageTag, installer.ThreeportRestApi.Name)
	if err != nil {
		return err
	}
	deployment.SetName(installer.ThreeportRestApi.ServiceResourceName)

	gk := schema.GroupKind{
		Group: "apps",
		Kind:  "Deployment",
	}
	mapping, err := (*mapper).RESTMapping(gk)
	if err != nil {
		return fmt.Errorf("failed to map deployment group kind to resource: %w", err)
	}

	_, err = dynamicKubeClient.
		Resource(mapping.Resource).
		Namespace(namespace).
		Update(context.TODO(), deployment, kubemetav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update deployment for %s: %w", installer.ThreeportRestApi.Name, err)
	}
	rolledOut := []retagged{{name: deployment.GetName(), previous: apiReplaced}}

	// Update Agent
	deployment, err = kube.GetResource(
		"apps",
		"v1",
		"Deployment",
		namespace,
		installer.ThreeportAgentDeployName,
		dynamicKubeClient,
		*mapper,
	)
	if err != nil {
		return fmt.Errorf("failed to get kubernetes deployment for %s: %w", installer.ThreeportAgent.Name, err)
	}

	agentReplaced, err := updateImageTagInDeployment(deployment, imageTag, installer.ThreeportAgent.Name)
	if err != nil {
		return err
	}
	deployment.SetName(installer.ThreeportAgentDeployName)

	_, err = dynamicKubeClient.
		Resource(mapping.Resource).
		Namespace(namespace).
		Update(context.TODO(), deployment, kubemetav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update deployment for %s: %w", installer.ThreeportAgent.Name, err)
	}
	rolledOut = append(rolledOut, retagged{name: installer.ThreeportAgentDeployName, previous: agentReplaced})

	// Update controller deployments
	controllerList := installer.ThreeportControllerList

	for _, c := range controllerList {
		deploymentResourceName := fmt.Sprintf("threeport-%s", c.Name)
		deployment, err := kube.GetResource(
			"apps",
			"v1",
			"Deployment",
			namespace,
			deploymentResourceName,
			dynamicKubeClient,
			*mapper,
		)
		if err != nil {
			// skip a controller deployment that is not found
			if k8serrors.IsNotFound(err) {
				continue
			}
			return fmt.Errorf("failed to get kubernetes deployment for controller %s: %w", c.Name, err)
		}

		// return the controller image update error
		controllerReplaced, err := updateImageTagInDeployment(deployment, imageTag, c.Name)
		if err != nil {
			return err
		}
		deployment.SetName(deploymentResourceName)

		gk := schema.GroupKind{
			Group: "apps",
			Kind:  "Deployment",
		}
		mapping, err := (*mapper).RESTMapping(gk)
		if err != nil {
			return fmt.Errorf("failed to map deployment group kind to resource: %w", err)
		}

		// update the resource
		_, err = dynamicKubeClient.
			Resource(mapping.Resource).
			Namespace(namespace).
			Update(context.TODO(), deployment, kubemetav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update deployment for controller %s: %w", c.Name, err)
		}
		rolledOut = append(rolledOut, retagged{name: deploymentResourceName, previous: controllerReplaced})
	}

	// An Update that does not error means the API server accepted the new
	// spec, not that anything is running it. Reporting success there reports
	// it while the new pods may still be pulling, crash looping, or failing
	// to start at all - and an upgrade that says it worked is the one nobody
	// goes back to check.
	fmt.Printf("Info: waiting for %d workload(s) to roll out\n", len(rolledOut))
	for _, workload := range rolledOut {
		if err := waitForRollout(
			dynamicKubeClient, mapping.Resource, namespace, workload.name, workload.previous,
		); err != nil {
			return err
		}
	}

	return nil
}

// retagged is a workload this upgrade changed, and the images it was running
// before. The previous images are carried along so a failed rollout can say
// what to go back to - by then the cluster holds only the new ones.
type retagged struct {
	name     string
	previous []string
}

// rolloutAttempts and rolloutPollSeconds bound how long a single workload is
// given to come up on its new image. They are variables so a test can wait
// seconds instead of minutes for the failing case.
var (
	rolloutAttempts    = 60
	rolloutPollSeconds = 5
)

// waitForRollout polls until a workload is running its new spec, or gives up.
//
// Three things have to agree. The controller must have observed the
// generation this upgrade produced, or the status being read is about the
// spec from before it and would report the previous rollout as this one.
// Every replica must be on the new image. And every replica must be ready,
// which is what separates a pod that started from one that is crash looping
// on the image it was just given.
func waitForRollout(
	kubeClient dynamic.Interface,
	resource schema.GroupVersionResource,
	namespace string,
	name string,
	previous []string,
) error {
	if err := util.Retry(rolloutAttempts, rolloutPollSeconds, func() error {
		current, err := kubeClient.Resource(resource).Namespace(namespace).Get(
			context.TODO(), name, kubemetav1.GetOptions{},
		)
		if err != nil {
			return fmt.Errorf("failed to read %s while waiting for its rollout: %w", name, err)
		}

		desired, _, _ := util.NestedInt64OrFloat64(current.Object, "spec", "replicas")
		generation, _, _ := util.NestedInt64OrFloat64(current.Object, "metadata", "generation")
		observed, _, _ := util.NestedInt64OrFloat64(current.Object, "status", "observedGeneration")
		updated, _, _ := util.NestedInt64OrFloat64(current.Object, "status", "updatedReplicas")
		total, _, _ := util.NestedInt64OrFloat64(current.Object, "status", "replicas")
		available, _, _ := util.NestedInt64OrFloat64(current.Object, "status", "availableReplicas")

		switch {
		case observed < generation:
			return fmt.Errorf("%s has not been observed at generation %d yet", name, generation)
		case updated < desired:
			return fmt.Errorf("%s has %d of %d replica(s) on the new image", name, updated, desired)
		case total > updated:
			return fmt.Errorf("%s still has %d replica(s) on the old image", name, total-updated)
		case available < updated:
			return fmt.Errorf("%s has %d of %d new replica(s) available", name, available, updated)
		}

		return nil
	}); err != nil {
		return fmt.Errorf(
			"%s did not roll out within %d seconds; the control plane is part upgraded and it was running %s: %w",
			name, rolloutAttempts*rolloutPollSeconds, strings.Join(previous, ", "), err,
		)
	}

	return nil
}

// updateImageTagInDeployment retags the containers this command owns and
// returns the images they were on, so a failed rollout can say what to go
// back to.
func updateImageTagInDeployment(deployment *unstructured.Unstructured, imageTag string, name string) ([]string, error) {
	var replaced []string
	deploymentContent := deployment.UnstructuredContent()

	if _, ok := deploymentContent["spec"]; !ok {
		return nil, fmt.Errorf("could not find spec in deployment for controller %s", name)
	}

	deploymentSpec, ok := deploymentContent["spec"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("could not type convert deployment spec for controller: %s", name)
	}

	if _, ok := deploymentSpec["template"]; !ok {
		return nil, fmt.Errorf("could not find template in deployment spec for controller %s", name)
	}

	template, ok := deploymentSpec["template"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("could not type convert template for controller: %s", name)
	}

	if _, ok := template["spec"]; !ok {
		return nil, fmt.Errorf("could not find spec in template for controller deployment for: %s", name)
	}

	templateSpec, ok := template["spec"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("could not type convert template spec for controller deployment for: %s", name)
	}

	if _, ok := templateSpec["containers"]; !ok {
		return nil, fmt.Errorf("could not find containers in controller deployment for: %s", name)
	}

	if name == installer.ThreeportRestApi.Name {
		initContainersList, ok := templateSpec["initContainers"].([]interface{})
		if !ok {
			return nil, fmt.Errorf("could not type convert init container list in deployment for: %s", name)
		}

		migrator, err := containerNamed(initContainersList, installer.ThreeportDatabaseMigratorContainerName)
		if err != nil {
			return nil, fmt.Errorf("could not find the init container to retag for %s: %w", name, err)
		}
		previous, err := retagContainer(migrator, imageTag)
		if err != nil {
			return nil, fmt.Errorf("could not retag the init container for %s: %w", name, err)
		}
		replaced = append(replaced, previous)

		templateSpec["initContainers"] = initContainersList
	}

	containerSpec, ok := templateSpec["containers"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("could not type convert container list in deployment for: %s", name)
	}

	container, err := containerNamed(containerSpec, imageContainerName(name))
	if err != nil {
		return nil, fmt.Errorf("could not find the container to retag for %s: %w", name, err)
	}
	previous, err := retagContainer(container, imageTag)
	if err != nil {
		return nil, fmt.Errorf("could not retag the container for %s: %w", name, err)
	}
	replaced = append(replaced, previous)

	templateSpec["containers"] = containerSpec
	template["spec"] = templateSpec
	deploymentSpec["template"] = template
	deploymentContent["spec"] = deploymentSpec
	deployment.SetUnstructuredContent(deploymentContent)

	return replaced, nil
}

func init() {
	UpgradeCmd.AddCommand(UpgradeControlPlaneCmd)

	UpgradeControlPlaneCmd.Flags().StringVarP(
		&updateImageTag,
		"version", "t", "", "version to update Threeport Control plane.",
	)

	UpgradeControlPlaneCmd.MarkFlagRequired("version")
}

// imageContainerName is the container that carries a component's own image.
//
// A controller's container is named after the controller. The API server and
// the agent are not: their pods were written by hand rather than from the
// controller template.
func imageContainerName(componentName string) string {
	switch componentName {
	case installer.ThreeportRestApi.Name:
		return installer.ThreeportApiServerContainerName
	case installer.ThreeportAgent.Name:
		return installer.ThreeportAgentContainerName
	default:
		return componentName
	}
}

// containerNamed returns the container with the given name.
//
// These were looked up by position: index 1 for the database migrator, index
// 1 for the agent's own container and index 0 for everything else. A position
// is not a property of a container, it is a property of whatever else is in
// the list beside it - so adding a sidecar, or one of the additional init
// containers the installer already supports, moved the thing being retagged
// and the upgrade retagged something else, or ran off the end of the list and
// panicked. A name is the container's own.
//
// The map is returned rather than a copy, so the caller writes through it
// into the list it came from.
func containerNamed(containers []interface{}, name string) (map[string]interface{}, error) {
	var found []string
	for _, raw := range containers {
		container, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		containerName, ok := container["name"].(string)
		if !ok {
			continue
		}
		if containerName == name {
			return container, nil
		}
		found = append(found, containerName)
	}

	return nil, fmt.Errorf("no container named %s; the pod has %v", name, found)
}

// retagContainer points a container at the same image with a new tag, and
// returns the image it replaced.
//
// The previous image is returned rather than discarded because the only
// advice worth giving when a rollout fails is what to go back to, and by then
// it has been overwritten in the cluster.
func retagContainer(container map[string]interface{}, imageTag string) (string, error) {
	currentImage, ok := container["image"].(string)
	if !ok {
		return "", fmt.Errorf("container %v has no image to retag", container["name"])
	}
	container["image"] = fmt.Sprintf("%s:%s", util.ImageWithoutTag(currentImage), imageTag)

	return currentImage, nil
}
