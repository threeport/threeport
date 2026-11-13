package v0

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// UpdateDeploymentImageTag sets imageTag on the control plane deployments that already exist.
// A deployment that already runs that tag is left in place. When wait is set, the call blocks until each changed deployment is ready.
func UpdateDeploymentImageTag(
	imageTag string,
	namespace string,
	backupBucket string,
	dynamicKubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
	controllers []*v0.ControlPlaneComponent,
	wait bool,
) error {
	// The new database migrator backs the database up before migrating, from
	// a location and with a certificate an API server Deployment installed
	// before it knew to does not mount.  Moved to the new image as it is, it
	// would find no location and migrate without a backup on a control plane
	// that has them.  So the location goes in first, and the Deployment gets
	// both mounts in the same update that moves it to the new image.
	backupInstaller := &ControlPlaneInstaller{
		Opts: Options{
			Namespace:       namespace,
			GcpBackupBucket: backupBucket,
		},
	}
	if err := backupInstaller.InstallDatabaseBackupConfig(dynamicKubeClient, mapper); err != nil {
		return err
	}

	// Update Rest-Api
	deployment, err := kube.GetResource(
		"apps",
		"v1",
		"Deployment",
		namespace,
		ThreeportRestApi.ServiceResourceName,
		dynamicKubeClient,
		*mapper,
	)
	if err != nil {
		return fmt.Errorf("failed to get kubernetes deployment for rest-api: %w", err)
	}
	before, err := deployment.MarshalJSON()
	if err != nil {
		return fmt.Errorf("failed to read deployment for rest-api: %w", err)
	}

	// return the rest-api image update error
	apiReplaced, err := UpdateImageTagInDeployment(deployment, imageTag, ThreeportRestApi.Name)
	if err != nil {
		return err
	}
	if err := PrepareDatabaseMigratorForBackups(deployment); err != nil {
		return fmt.Errorf("failed to give the database migrator what it backs up with: %w", err)
	}
	deployment.SetName(ThreeportRestApi.ServiceResourceName)

	gk := schema.GroupKind{
		Group: "apps",
		Kind:  "Deployment",
	}
	mapping, err := (*mapper).RESTMapping(gk)
	if err != nil {
		return fmt.Errorf("failed to map deployment group kind to resource: %w", err)
	}

	// skip a deployment whose spec did not change
	var rolledOut []retagged
	changed, err := writeDeploymentIfChanged(before, deployment, dynamicKubeClient, mapping.Resource, namespace, ThreeportRestApi.Name)
	if err != nil {
		return err
	}
	if changed {
		rolledOut = append(rolledOut, retagged{name: deployment.GetName(), previous: apiReplaced})
	}

	// Update Agent
	deployment, err = kube.GetResource(
		"apps",
		"v1",
		"Deployment",
		namespace,
		ThreeportAgentDeployName,
		dynamicKubeClient,
		*mapper,
	)
	if err != nil {
		return fmt.Errorf("failed to get kubernetes deployment for %s: %w", ThreeportAgent.Name, err)
	}
	before, err = deployment.MarshalJSON()
	if err != nil {
		return fmt.Errorf("failed to read deployment for %s: %w", ThreeportAgent.Name, err)
	}

	agentReplaced, err := UpdateImageTagInDeployment(deployment, imageTag, ThreeportAgent.Name)
	if err != nil {
		return err
	}
	deployment.SetName(ThreeportAgentDeployName)

	// skip a deployment whose spec did not change
	changed, err = writeDeploymentIfChanged(before, deployment, dynamicKubeClient, mapping.Resource, namespace, ThreeportAgent.Name)
	if err != nil {
		return err
	}
	if changed {
		rolledOut = append(rolledOut, retagged{name: ThreeportAgentDeployName, previous: agentReplaced})
	}

	// update the controllers named by the recorded apis list
	// a nil list is the full default set
	controllerList := controllers
	if controllerList == nil {
		controllerList = ThreeportControllerList
	}

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
		before, err := deployment.MarshalJSON()
		if err != nil {
			return fmt.Errorf("failed to read deployment for controller %s: %w", c.Name, err)
		}

		// return the controller image update error
		controllerReplaced, err := UpdateImageTagInDeployment(deployment, imageTag, c.Name)
		if err != nil {
			return err
		}
		deployment.SetName(deploymentResourceName)

		gk := schema.GroupKind{
			Group: "apps",
			Kind:  "Deployment",
		}
		mapping, err = (*mapper).RESTMapping(gk)
		if err != nil {
			return fmt.Errorf("failed to map deployment group kind to resource: %w", err)
		}

		// skip a deployment whose spec did not change
		changed, err := writeDeploymentIfChanged(before, deployment, dynamicKubeClient, mapping.Resource, namespace, c.Name)
		if err != nil {
			return err
		}
		if changed {
			rolledOut = append(rolledOut, retagged{name: deploymentResourceName, previous: controllerReplaced})
		}
	}

	// wait only for the deployments this call changed
	if !wait || len(rolledOut) == 0 {
		return nil
	}

	// An Update that does not error means the API server accepted the new
	// spec, not that anything is running it. Reporting success there reports
	// it while the new pods may still be pulling, crash looping, or failing
	// to start at all - and an upgrade that says it worked is the one nobody
	// goes back to check.
	fmt.Printf("Info: waiting for %d workload(s) to roll out\n", len(rolledOut))
	for _, workload := range rolledOut {
		if err := WaitForRollout(
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

// RolloutAttempts and RolloutPollSeconds bound how long a single workload is
// given to come up on its new image. They are variables so a test can wait
// seconds instead of minutes for the failing case.
var (
	RolloutAttempts    = 60
	RolloutPollSeconds = 5
)

// WaitForRollout polls until a workload is running its new spec, or gives up.
//
// Three things have to agree. The controller must have observed the
// generation this upgrade produced, or the status being read is about the
// spec from before it and would report the previous rollout as this one.
// Every replica must be on the new image. And every replica must be ready,
// which is what separates a pod that started from one that is crash looping
// on the image it was just given.
func WaitForRollout(
	kubeClient dynamic.Interface,
	resource schema.GroupVersionResource,
	namespace string,
	name string,
	previous []string,
) error {
	if err := util.Retry(RolloutAttempts, RolloutPollSeconds, func() error {
		current, err := kubeClient.Resource(resource).Namespace(namespace).Get(
			context.TODO(), name, metav1.GetOptions{},
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
			name, RolloutAttempts*RolloutPollSeconds, strings.Join(previous, ", "), err,
		)
	}

	return nil
}

// UpdateImageTagInDeployment retags the containers this command owns and
// returns the images they were on, so a failed rollout can say what to go
// back to.
func UpdateImageTagInDeployment(deployment *unstructured.Unstructured, imageTag string, name string) ([]string, error) {
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

	if name == ThreeportRestApi.Name {
		initContainersList, ok := templateSpec["initContainers"].([]interface{})
		if !ok {
			return nil, fmt.Errorf("could not type convert init container list in deployment for: %s", name)
		}

		migrator, err := ContainerNamed(initContainersList, ThreeportDatabaseMigratorContainerName)
		if err != nil {
			return nil, fmt.Errorf("could not find the init container to retag for %s: %w", name, err)
		}
		previous, err := RetagContainer(migrator, imageTag)
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

	container, err := ContainerNamed(containerSpec, ImageContainerName(name))
	if err != nil {
		return nil, fmt.Errorf("could not find the container to retag for %s: %w", name, err)
	}
	previous, err := RetagContainer(container, imageTag)
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

// writeDeploymentIfChanged updates the deployment when its JSON changed.
func writeDeploymentIfChanged(
	before []byte,
	deployment *unstructured.Unstructured,
	dynamicKubeClient dynamic.Interface,
	resource schema.GroupVersionResource,
	namespace string,
	what string,
) (bool, error) {
	// leave a deployment that already has this spec
	after, err := deployment.MarshalJSON()
	if err != nil {
		return false, fmt.Errorf("failed to read deployment for %s: %w", what, err)
	}
	if bytes.Equal(before, after) {
		return false, nil
	}

	// write the changed deployment
	_, err = dynamicKubeClient.Resource(resource).Namespace(namespace).Update(
		context.TODO(),
		deployment,
		metav1.UpdateOptions{},
	)
	if err != nil {
		return false, fmt.Errorf("failed to update deployment for %s: %w", what, err)
	}

	return true, nil
}

// imageContainerName is the container that carries a component's own image.
//
// A controller's container is named after the controller. The API server and
// the agent are not: their pods were written by hand rather than from the
// controller template.
func ImageContainerName(componentName string) string {
	switch componentName {
	case ThreeportRestApi.Name:
		return ThreeportApiServerContainerName
	case ThreeportAgent.Name:
		return ThreeportAgentContainerName
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
func ContainerNamed(containers []interface{}, name string) (map[string]interface{}, error) {
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
func RetagContainer(container map[string]interface{}, imageTag string) (string, error) {
	currentImage, ok := container["image"].(string)
	if !ok {
		return "", fmt.Errorf("container %v has no image to retag", container["name"])
	}
	container["image"] = fmt.Sprintf("%s:%s", util.ImageWithoutTag(currentImage), imageTag)

	return currentImage, nil
}
