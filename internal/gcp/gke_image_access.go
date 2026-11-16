package gcp

import (
	"fmt"

	"github.com/go-logr/logr"

	"github.com/threeport/threeport/internal/provider"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	tpi "github.com/threeport/threeport/pkg/threeport-installer/v0"
)

// agentImageNamespace returns the image namespace of the agent a control plane
// records, or empty when it records none.
func agentImageNamespace(controlPlane *v0.ControlPlaneInstance) string {
	if controlPlane == nil {
		return ""
	}
	for _, component := range controlPlane.CustomComponentInfo {
		if component != nil && component.Name == tpi.ThreeportAgentName {
			return component.ImageNamespace
		}
	}

	return ""
}

// nodeImageNamespace returns the image namespace a cluster's nodes pull from
// when the provider can grant them read access to its repository. It returns
// empty when the control plane records none or the provider cannot manage the
// repository, so a provider created without that access still creates clusters.
func nodeImageNamespace(
	controlPlane *v0.ControlPlaneInstance,
	canManage func(imageNamespace string) (bool, error),
) (string, error) {
	// find the namespace the agent image comes from
	imageNamespace := agentImageNamespace(controlPlane)
	if imageNamespace == "" {
		return "", nil
	}

	// require that the provider can change access to its repository
	can, err := canManage(imageNamespace)
	if err != nil {
		return "", fmt.Errorf("failed to check access to the image repository: %w", err)
	}
	if !can {
		return "", nil
	}

	return imageNamespace, nil
}

// setNodeImageNamespace sets the image namespace on the infra so its stack
// grants the nodes read access to the control plane's image repository. A
// failure is logged and leaves the namespace empty, because the cluster is
// usable without the grant and a grant made by hand restores it.
func setNodeImageNamespace(
	r *controller.Reconciler,
	infra *provider.KubernetesRuntimeInfraGKE,
	log *logr.Logger,
) {
	// read the control plane this controller runs in, with its components
	selfInstance, err := client.GetSelfControlPlaneInstance(r.APIClient, r.APIServer)
	if err != nil {
		log.Error(err, "failed to get the control plane, so the nodes are not granted image repository access")
		return
	}
	controlPlane, err := client.GetControlPlaneInstanceByID(r.APIClient, r.APIServer, *selfInstance.ID)
	if err != nil {
		log.Error(err, "failed to get the control plane, so the nodes are not granted image repository access")
		return
	}

	// keep the namespace only when the provider can grant on its repository
	imageNamespace, err := nodeImageNamespace(
		controlPlane,
		func(imageNamespace string) (bool, error) {
			return provider.CanManageImageRepositoryAccess(imageNamespace, infra.ServiceAccountCredentials)
		},
	)
	if err != nil {
		log.Error(err, "failed to check access to the image repository, so the nodes are not granted it")
		return
	}
	if imageNamespace == "" {
		log.Info("the provider cannot manage the control plane's image repository, so the nodes are not granted access to it")
		return
	}
	infra.ImageNamespace = imageNamespace
}
