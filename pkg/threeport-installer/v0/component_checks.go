package v0

import (
	"errors"
	"fmt"
	"reflect"

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
// InstallComputeSpaceWorkloadControllerRBAC would create is already in place.
//
// How much of a binding is compared follows what the install could do about a
// difference.  An installer set to update can replace a binding left from
// another GCP project or pointing at the wrong role, so those are compared.  An
// installer that only creates cannot touch a binding that exists, so for one of
// those the question is only whether the binding is there - calling a stale one
// not current would ask, on every invocation, for a repair that cannot land.
//
// A binding that exists but grants nothing useful is therefore reported current
// to a create-only installer.  That is not this check going quiet on a real
// problem: nothing in this path can fix it either way, and reporting it would
// only add back the redundant installs this check exists to remove.
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

		// A binding that is present but wrong can only be put right by an
		// install that updates. With a create-only installer, creating over an
		// existing binding does nothing, so reporting it stale would ask for a
		// repair on every invocation that can never land - the exact waste
		// these checks exist to remove. Presence is the whole question there.
		if !cpi.Opts.CreateOrUpdateKubeResources {
			continue
		}

		matches, err := bindingGrants(
			binding,
			cpi.computeSpaceWorkloadControllerBinding(controllerName, gcpProjectID),
		)
		if err != nil {
			return false, fmt.Errorf(
				"failed to read the %s cluster-admin binding: %w", controllerName, err,
			)
		}
		if !matches {
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

// bindingGrants reports whether an existing cluster role binding grants what a
// wanted one would. The subject's name is not enough on its own: a binding can
// carry the right name as the wrong kind of subject, or point at a different
// role, and in either case the controller it was meant to authorize has no
// access. The role reference and every wanted subject are compared whole.
func bindingGrants(existing, wanted *unstructured.Unstructured) (bool, error) {
	existingRole, _, err := unstructured.NestedMap(existing.Object, "roleRef")
	if err != nil {
		return false, err
	}
	wantedRole, _, err := unstructured.NestedMap(wanted.Object, "roleRef")
	if err != nil {
		return false, err
	}
	if !reflect.DeepEqual(existingRole, wantedRole) {
		return false, nil
	}

	existingSubjects, _, err := unstructured.NestedSlice(existing.Object, "subjects")
	if err != nil {
		return false, err
	}
	wantedSubjects, _, err := unstructured.NestedSlice(wanted.Object, "subjects")
	if err != nil {
		return false, err
	}

	for _, wantedSubject := range wantedSubjects {
		found := false
		for _, existingSubject := range existingSubjects {
			if reflect.DeepEqual(existingSubject, wantedSubject) {
				found = true
				break
			}
		}
		if !found {
			return false, nil
		}
	}

	return true, nil
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
