package v0

import (
	"errors"
	"fmt"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"

	kube "github.com/threeport/threeport/pkg/kube/v0"
)

// The installs in this package are create-or-update, so re-running one is not
// destructive.  It is not free either: each is a burst of writes against a live
// cluster's API server, and the caller that reinstalls the compute space
// components also waits ten seconds for the CRDs to settle.  The checks below
// let a caller skip a step whose work is already done, and are deliberately
// per-step: a cluster missing only the support services operator should still
// get the operator.
//
// Each check answers "would installing this change anything", not merely "is
// something there".  Where a field legitimately changes between installs - the
// agent's image, the GCP project a binding names - the check compares it, so a
// stale resource is still reinstalled.

// resourceInstalled looks up one resource and reports whether it is present.
// A resource that is absent, or whose kind is not registered at all - which is
// how a missing CRD looks - is an answer rather than a failure.  Any other
// error is returned: reading a connection blip as absence would reinstall
// against a cluster that needs nothing.
func resourceInstalled(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
	apiGroup string,
	apiVersion string,
	kind string,
	namespace string,
	name string,
) (*unstructured.Unstructured, bool, error) {
	resource, err := kube.GetResource(
		apiGroup,
		apiVersion,
		kind,
		namespace,
		name,
		kubeClient,
		*mapper,
	)
	if err == nil {
		return resource, true, nil
	}
	if k8serrors.IsNotFound(err) {
		return nil, false, nil
	}
	var noKindMatch *meta.NoKindMatchError
	if errors.As(err, &noKindMatch) {
		return nil, false, nil
	}

	return nil, false, err
}

// ComputeSpaceControlPlaneComponentsCurrent reports whether the cluster already
// has what InstallComputeSpaceControlPlaneComponents would install: the
// threeport agent, running the image this installer would give it, and the
// CRDs that go in alongside it.
//
// The agent image is compared when the installer is set to update resources,
// because it is the one part of this set that legitimately changes - a runtime
// instance carrying a new ThreeportAgentImage needs the install to run even
// though an agent is already there. An installer that only creates cannot
// replace a running agent, so for one of those presence is the whole question.
func (cpi *ControlPlaneInstaller) ComputeSpaceControlPlaneComponentsCurrent(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
) (bool, error) {
	agent, present, err := resourceInstalled(
		kubeClient,
		mapper,
		"apps",
		"v1",
		"Deployment",
		cpi.Opts.Namespace,
		ThreeportAgentDeployName,
	)
	if err != nil {
		return false, fmt.Errorf("failed to check for the threeport agent: %w", err)
	}
	if !present {
		return false, nil
	}

	// With CreateOrUpdateKubeResources off the install only creates, so a
	// running agent is left alone whatever image it carries - reinstalling to
	// change it would be work that cannot succeed, and on any cluster installed
	// from a different registry it would be work done on every invocation. The
	// image is only a difference worth acting on when the install would act.
	if cpi.Opts.CreateOrUpdateKubeResources {
		wantImage := cpi.getImage(
			cpi.Opts.AgentInfo.Name,
			cpi.Opts.AgentInfo.ImageName,
			cpi.Opts.AgentInfo.ImageNamespace,
			cpi.Opts.AgentInfo.ImageTag,
		)
		running, err := containerImages(agent)
		if err != nil {
			return false, fmt.Errorf("failed to read the threeport agent's images: %w", err)
		}
		if !contains(running, wantImage) {
			return false, nil
		}
	}

	// the CRDs are installed with these components, and the support services
	// operator install that follows needs them registered
	_, present, err = resourceInstalled(
		kubeClient,
		mapper,
		"apiextensions.k8s.io",
		"v1",
		"CustomResourceDefinition",
		"",
		ThreeportCertManagerCRDName,
	)
	if err != nil {
		return false, fmt.Errorf("failed to check for the threeport CRDs: %w", err)
	}

	return present, nil
}

// SupportServicesOperatorInstalled reports whether the support services
// operator InstallThreeportSupportServicesOperator would install is already
// running on the cluster.
func SupportServicesOperatorInstalled(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
) (bool, error) {
	_, present, err := resourceInstalled(
		kubeClient,
		mapper,
		"apps",
		"v1",
		"Deployment",
		ControlPlaneNamespace,
		SupportServicesOperatorDeployName,
	)
	if err != nil {
		return false, fmt.Errorf("failed to check for the support services operator: %w", err)
	}

	return present, nil
}

// EksThreeportSystemServicesInstalled reports whether the EKS system services
// InstallEksThreeportSystemServices would install are already on the cluster.
func EksThreeportSystemServicesInstalled(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
) (bool, error) {
	_, present, err := resourceInstalled(
		kubeClient,
		mapper,
		"apps",
		"v1",
		"Deployment",
		ClusterAutoscalerNamespace,
		ClusterAutoscalerDeployName,
	)
	if err != nil {
		return false, fmt.Errorf("failed to check for the cluster autoscaler: %w", err)
	}

	return present, nil
}

// ComputeSpaceWorkloadControllerRBACCurrent reports whether every binding
// InstallComputeSpaceWorkloadControllerRBAC would create is present and names
// the workload identity principal it would name.  A binding left from another
// GCP project or another control plane namespace is not current: its subject no
// longer matches, and the controllers it was meant to authorize cannot reach
// the cluster.
func (cpi *ControlPlaneInstaller) ComputeSpaceWorkloadControllerRBACCurrent(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
	gcpProjectID string,
) (bool, error) {
	for _, controllerName := range computeSpaceWorkloadControllers() {
		binding, present, err := resourceInstalled(
			kubeClient,
			mapper,
			"rbac.authorization.k8s.io",
			"v1",
			"ClusterRoleBinding",
			"",
			fmt.Sprintf("%s-cluster-admin", controllerName),
		)
		if err != nil {
			return false, fmt.Errorf(
				"failed to check for the %s cluster-admin binding: %w", controllerName, err,
			)
		}
		if !present {
			return false, nil
		}

		wantSubject := fmt.Sprintf(
			"serviceAccount:%s.svc.id.goog[%s/%s]",
			gcpProjectID, cpi.Opts.Namespace, controllerName,
		)
		subjects, err := subjectNames(binding)
		if err != nil {
			return false, fmt.Errorf(
				"failed to read the %s cluster-admin binding's subjects: %w", controllerName, err,
			)
		}
		if !contains(subjects, wantSubject) {
			return false, nil
		}
	}

	return true, nil
}

// containerImages returns the images of every container in a deployment.
func containerImages(deployment *unstructured.Unstructured) ([]string, error) {
	containers, found, err := unstructured.NestedSlice(
		deployment.Object, "spec", "template", "spec", "containers",
	)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}

	var images []string
	for _, container := range containers {
		fields, ok := container.(map[string]interface{})
		if !ok {
			continue
		}
		if image, ok := fields["image"].(string); ok {
			images = append(images, image)
		}
	}

	return images, nil
}

// subjectNames returns the names of every subject in a cluster role binding.
func subjectNames(binding *unstructured.Unstructured) ([]string, error) {
	subjects, found, err := unstructured.NestedSlice(binding.Object, "subjects")
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}

	var names []string
	for _, subject := range subjects {
		fields, ok := subject.(map[string]interface{})
		if !ok {
			continue
		}
		if name, ok := fields["name"].(string); ok {
			names = append(names, name)
		}
	}

	return names, nil
}

// contains reports whether a value is in a slice.
func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}

	return false
}
