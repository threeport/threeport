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
	cli "github.com/threeport/threeport/pkg/cli/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
	installer "github.com/threeport/threeport/pkg/threeport-installer/v0"
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
	Example: "tptctl upgrade control-plane --version=v0.5",
	Short:   "Upgrades the version of the Threeport control plane",
	Long: `Upgrades the version of the Threeport control plane. The version should be a valid
	image tag.`,
	SilenceUsage: true,
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, config, apiEndpoint, requestedControlPlane := getClientContext(cmd)

		encyptionKey, err := config.GetEncryptionKey(requestedControlPlane)
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

	updateImageTagInDeployment(deployment, imageTag, installer.ThreeportRestApi.Name)
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

	updateImageTagInDeployment(deployment, imageTag, installer.ThreeportAgent.Name)
	deployment.SetName(installer.ThreeportAgentDeployName)

	_, err = dynamicKubeClient.
		Resource(mapping.Resource).
		Namespace(namespace).
		Update(context.TODO(), deployment, kubemetav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update deployment for %s: %w", installer.ThreeportAgent.Name, err)
	}

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
			return fmt.Errorf("failed to get kubernetes deployment for controller %s: %w", c.Name, err)
		}

		updateImageTagInDeployment(deployment, imageTag, c.Name)
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
	}

	return nil
}

func updateImageTagInDeployment(deployment *unstructured.Unstructured, imageTag string, name string) error {
	deploymentContent := deployment.UnstructuredContent()

	if _, ok := deploymentContent["spec"]; !ok {
		return fmt.Errorf("could not find spec in deployment for controller %s", name)
	}

	deploymentSpec, ok := deploymentContent["spec"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("could not type convert deployment spec for controller: %s", name)
	}

	if _, ok := deploymentSpec["template"]; !ok {
		return fmt.Errorf("could not find template in deployment spec for controller %s", name)
	}

	template, ok := deploymentSpec["template"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("could not type convert template for controller: %s", name)
	}

	if _, ok := template["spec"]; !ok {
		return fmt.Errorf("could not find spec in template for controller deployment for: %s", name)
	}

	templateSpec, ok := template["spec"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("could not type convert template spec for controller deployment for: %s", name)
	}

	if _, ok := templateSpec["containers"]; !ok {
		return fmt.Errorf("could not find containers in controller deployment for: %s", name)
	}

	initContainerSpec := []interface{}{}
	// If modifying the rest-api we need to ensure we update the database-migrator version as well
	if name == installer.ThreeportRestApi.Name {
		initContainersList, ok := templateSpec["initContainers"].([]interface{})
		if !ok {
			return fmt.Errorf("could not type convert init container list in deployment for: %s", name)
		}

		db_migrator, ok := initContainersList[1].(map[string]interface{})
		if !ok {
			return fmt.Errorf("could not type convert database init container for: %s", name)
		}

		currentImage := db_migrator["image"].(string)
		imageSlice := strings.Split(currentImage, ":")
		db_migrator["image"] = fmt.Sprintf("%s:%s", imageSlice[0], updateImageTag)
		initContainersList[1] = db_migrator
		initContainerSpec = initContainersList
	}

	containerSpec, ok := templateSpec["containers"].([]interface{})
	if !ok {
		return fmt.Errorf("could not type convert container list in deployment for: %s", name)
	}

	containerIndex := 0
	if name == installer.ThreeportAgent.Name {
		containerIndex = 1
	}

	container, ok := containerSpec[containerIndex].(map[string]interface{})
	if !ok {
		return fmt.Errorf("could not type convert container in deployment for: %s", name)
	}

	currentImage := container["image"].(string)
	imageSlice := strings.Split(currentImage, ":")
	container["image"] = fmt.Sprintf("%s:%s", imageSlice[0], updateImageTag)
	containerSpec[containerIndex] = container
	templateSpec["containers"] = containerSpec
	templateSpec["initContainers"] = initContainerSpec
	template["spec"] = templateSpec
	deploymentSpec["template"] = template
	deploymentContent["spec"] = deploymentSpec
	deployment.SetUnstructuredContent(deploymentContent)

	return nil
}

func init() {
	UpgradeCmd.AddCommand(UpgradeControlPlaneCmd)

	UpgradeControlPlaneCmd.Flags().StringVarP(
		&updateImageTag,
		"version", "t", "", "version to update Threeport Control plane.",
	)

	UpgradeControlPlaneCmd.MarkFlagRequired("version")
}
