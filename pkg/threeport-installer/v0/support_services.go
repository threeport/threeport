package v0

import (
	"bytes"
	"embed"
	"fmt"
	"reflect"

	kubeerr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/yaml"

	kube "github.com/threeport/threeport/pkg/kube/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

const (
	SupportServicesOperatorImage = "ghcr.io/nukleros/support-services-operator:v0.9.0"

	// links the service account declared in the IngressComponent resource to
	// the IRSA role internal/provider creates for it on EKS.
	DNSManagerServiceAccountName     = "external-dns"
	DNSManagerServiceAccountNamepace = "nukleros-ingress-system"

	DNS01ChallengeServiceAccountName     = "cert-manager"
	DNS01ChallengeServiceAccountNamepace = "nukleros-certs-system"

	SecretsManagerServiceAccountName      = "external-secrets"
	SecretsManagerServiceAccountNamespace = "nukleros-secrets-system"

	// links the service account used by the EBS CSI driver to the IRSA role
	// internal/provider creates for it on EKS.
	StorageManagerServiceAccountName      = "ebs-csi-controller-sa"
	StorageManagerServiceAccountNamespace = "kube-system"

	// links the service account used by the cluster autoscaler installation to
	// the IRSA role internal/provider creates for it on EKS.
	ClusterAutoscalerServiceAccountName = "cluster-autoscaler"
	ClusterAutoscalerNamespace          = "kube-system"

	// the deployments and CRD named here are what the installs in this package
	// create, and what the checks that guard those installs look up
	ClusterAutoscalerDeployName       = "cluster-autoscaler"
	SupportServicesOperatorDeployName = "support-services-operator-controller-manager"
	ThreeportCertManagerCRDName       = "certificatescomponents.platform.addons.nukleros.io"

	supportServicesOperatorNamespace = "support-services-operator-system"
	supportServicesOperatorImage     = "controller:latest"
)

// ThreeportCRDNames are the custom resource definitions InstallThreeportCRDs
// creates. The install and the check that guards it read the same list, so a CRD
// added to one is not missed by the other - and a check that missed one would
// report a cluster ready while the support services operator install that
// depends on it is still absent.
func ThreeportCRDNames() []string {
	// return the definitions the embedded install manifest registers
	return []string{
		"certificatescomponents.platform.addons.nukleros.io",
		"databasecomponents.application.addons.nukleros.io",
		"ingresscomponents.platform.addons.nukleros.io",
		"secretscomponents.platform.addons.nukleros.io",
		"supportservices.setup.addons.nukleros.io",
	}
}

//go:embed assets/support-services-operator.yaml
var supportServicesOperatorYAML embed.FS

// InstallThreeportCRDs installs the support-services custom resource definitions.
func InstallThreeportCRDs(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
) error {
	objects, err := supportServicesOperatorObjects()
	if err != nil {
		return err
	}

	// custom resource definitions have to exist before the operator deployment
	for _, object := range objects {
		if object.GetKind() != "CustomResourceDefinition" {
			continue
		}
		if _, err := kube.CreateResource(object, kubeClient, *mapper); err != nil {
			return fmt.Errorf("failed to install support services CRD %s: %w", object.GetName(), err)
		}
	}

	return nil
}

// InstallEksThreeportSystemServices installs the support services CRDs and operator.
func InstallEksThreeportSystemServices(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
	clusterName string,
	accountID string,
) error {
	// install the definitions, then the operator that reconciles them
	if err := InstallThreeportCRDs(kubeClient, mapper); err != nil {
		return err
	}
	if err := InstallThreeportSupportServicesOperator(kubeClient, mapper, "aws"); err != nil {
		return err
	}

	// the autoscaler is not part of the operator manifest
	return installEksClusterAutoscaler(kubeClient, mapper, clusterName, accountID)
}

// SupportServicesCloud maps an infra provider to a SupportServices cloud value.
// The custom resource accepts only local, google, and aws.
func SupportServicesCloud(infraProvider string) string {
	// map the provider onto a value the custom resource accepts
	switch infraProvider {
	case "eks":
		return "aws"
	case "gke":
		return "google"
	default:
		return "local"
	}
}

// InstallThreeportSupportServicesOperator installs the support services operator.
func InstallThreeportSupportServicesOperator(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
	cloud string,
) error {
	// install the operator, then the support services object
	objects, err := supportServicesOperatorObjects()
	if err != nil {
		return err
	}

	// the namespace threeport-control-plane already exists
	for _, object := range objects {
		if object.GetKind() == "CustomResourceDefinition" || object.GetKind() == "Namespace" {
			continue
		}
		if object.GetKind() == "Deployment" {
			if err := installDeployment(object, kubeClient, mapper); err != nil {
				return fmt.Errorf("failed to install support services operator deployment: %w", err)
			}
			continue
		}
		// an existing cluster role is updated so its rules match this manifest
		install := kube.CreateResource
		if object.GetKind() == "ClusterRole" || object.GetKind() == "ClusterRoleBinding" {
			install = kube.CreateOrUpdateResource
		}
		if _, err := install(object, kubeClient, *mapper); err != nil {
			return fmt.Errorf("failed to install support services operator object %s: %w", object.GetName(), err)
		}
	}

	supportServices := supportServicesObject(cloud)
	if _, err := kube.CreateResource(supportServices, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to create SupportServices object: %w", err)
	}

	return nil
}

// addCainjectorPatchVerb adds patch to webhook rules that only allow update.
// It returns whether the role changed.
func addCainjectorPatchVerb(role map[string]interface{}) (bool, error) {
	rules, found, err := unstructured.NestedSlice(role, "rules")
	if err != nil || !found {
		return false, fmt.Errorf("cainjector role has no rules")
	}
	changed := false
	for _, rule := range rules {
		entry, ok := rule.(map[string]interface{})
		if !ok {
			continue
		}
		if !webhookRule(entry) {
			continue
		}
		verbs := stringList(entry["verbs"])
		if hasVerb(verbs, "patch") {
			continue
		}
		// store verbs as []interface{} so the role update can deep-copy them
		next := make([]interface{}, 0, len(verbs)+1)
		for _, verb := range verbs {
			next = append(next, verb)
		}
		entry["verbs"] = append(next, "patch")
		changed = true
	}
	// NestedSlice copies the rules, so write the edited slice back
	if changed {
		role["rules"] = rules
	}
	return changed, nil
}

// webhookRule reports whether a role rule covers admission webhook configuration.
func webhookRule(rule map[string]interface{}) bool {
	// match the admission group and either webhook resource
	groups := stringList(rule["apiGroups"])
	resources := stringList(rule["resources"])
	if !hasVerb(groups, "admissionregistration.k8s.io") {
		return false
	}
	return hasVerb(resources, "validatingwebhookconfigurations") || // validating webhook
		hasVerb(resources, "mutatingwebhookconfigurations") // mutating webhook
}

// stringList reads a string slice stored as []string or []interface{}.
func stringList(value interface{}) []string {
	// accept both encodings unstructured objects use
	switch typed := value.(type) {
	case []string:
		return typed
	case []interface{}:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if ok {
				values = append(values, text)
			}
		}
		return values
	default:
		return nil
	}
}

// hasVerb reports whether the list contains the value.
func hasVerb(values []string, want string) bool {
	// return when the list contains the value
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// supportServicesObject returns the SupportServices custom resource for a cloud.
func supportServicesObject(cloud string) *unstructured.Unstructured {
	// set the cloud the operator uses for the load balancer
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "setup.addons.nukleros.io/v1alpha1",
			"kind":       "SupportServices",
			"metadata": map[string]interface{}{
				"name": "threeport-support-services",
			},
			"spec": map[string]interface{}{
				"tier":                     "development",
				"defaultIngressController": "nginx",
				"cloud":                    cloud,
			},
		},
	}
}

// deploymentGoneAttempts bounds how many times to read a deleted deployment.
var deploymentGoneAttempts = 120

// deploymentGoneWaitSeconds sets the pause between those reads.
var deploymentGoneWaitSeconds = 1

// installDeployment applies a deployment, replacing it when the selector changed.
// A deployment selector is immutable, so the v0.4 operator cannot be updated in place.
func installDeployment(object *unstructured.Unstructured, kubeClient dynamic.Interface, mapper *meta.RESTMapper) error {
	// read the deployment that is already installed
	existing, err := kube.GetResource(
		object.GroupVersionKind().Group,
		object.GroupVersionKind().Version,
		object.GetKind(),
		object.GetNamespace(),
		object.GetName(),
		kubeClient,
		*mapper,
	)
	if err != nil && !kubeerr.IsNotFound(err) {
		return fmt.Errorf("failed to get deployment %s: %w", object.GetName(), err)
	}

	// delete it when the selector does not match this manifest
	replaced := false
	if err == nil {
		existingSelector, _, _ := unstructured.NestedMap(existing.Object, "spec", "selector")
		desiredSelector, _, _ := unstructured.NestedMap(object.Object, "spec", "selector")
		if !reflect.DeepEqual(existingSelector, desiredSelector) {
			if err := kube.DeleteResource(object, kubeClient, *mapper); err != nil {
				return fmt.Errorf("failed to replace deployment %s: %w", object.GetName(), err)
			}
			replaced = true
		}
	}
	if replaced {
		if err := waitForDeploymentGone(object, kubeClient, mapper); err != nil {
			return err
		}
		if _, err := kube.CreateResource(object, kubeClient, *mapper); err != nil {
			return fmt.Errorf("failed to apply deployment %s: %w", object.GetName(), err)
		}
		return nil
	}

	// apply the manifest
	if _, err := kube.CreateOrUpdateResource(object, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to apply deployment %s: %w", object.GetName(), err)
	}

	return nil
}

// waitForDeploymentGone returns when Get cannot find the deployment.
func waitForDeploymentGone(object *unstructured.Unstructured, kubeClient dynamic.Interface, mapper *meta.RESTMapper) error {
	// read the deployment until Get cannot find it
	return util.Retry(deploymentGoneAttempts, deploymentGoneWaitSeconds, func() error {
		_, err := kube.GetResource(
			object.GroupVersionKind().Group,
			object.GroupVersionKind().Version,
			object.GetKind(),
			object.GetNamespace(),
			object.GetName(),
			kubeClient,
			*mapper,
		)
		if kubeerr.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		return fmt.Errorf("deployment %s is still present", object.GetName())
	})
}

// supportServicesOperatorObjects decodes the embedded install manifest.
func supportServicesOperatorObjects() ([]*unstructured.Unstructured, error) {
	raw, err := supportServicesOperatorYAML.ReadFile("assets/support-services-operator.yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to read support services operator manifest: %w", err)
	}

	var objects []*unstructured.Unstructured
	for _, doc := range bytes.Split(raw, []byte("\n---\n")) {
		if len(bytes.TrimSpace(doc)) == 0 {
			continue
		}
		var object map[string]interface{}
		if err := yaml.Unmarshal(doc, &object); err != nil {
			return nil, fmt.Errorf("failed to decode support services operator manifest: %w", err)
		}
		if object == nil {
			continue
		}
		prepared := prepareSupportServicesOperatorObject(object)
		if prepared == nil {
			continue
		}
		objects = append(objects, &unstructured.Unstructured{Object: prepared})
	}

	return objects, nil
}

// prepareSupportServicesOperatorObject sets the namespace and the controller image.
func prepareSupportServicesOperatorObject(object map[string]interface{}) map[string]interface{} {
	kind, _ := object["kind"].(string)
	if kind == "Namespace" {
		return nil
	}
	rewriteNamespace(object)
	if kind == "Deployment" {
		setManagerImage(object)
	}
	return object
}

// rewriteNamespace writes the control plane namespace onto each object.
func rewriteNamespace(value interface{}) {
	switch typed := value.(type) {
	case map[string]interface{}:
		for key, child := range typed {
			if text, ok := child.(string); ok && text == supportServicesOperatorNamespace {
				typed[key] = ControlPlaneNamespace
				continue
			}
			rewriteNamespace(child)
		}
	case []interface{}:
		for _, child := range typed {
			rewriteNamespace(child)
		}
	}
}

// setManagerImage sets the controller container image.
func setManagerImage(object map[string]interface{}) {
	containers, found, err := unstructured.NestedSlice(object, "spec", "template", "spec", "containers")
	if err != nil || !found {
		return
	}
	for _, container := range containers {
		typed, ok := container.(map[string]interface{})
		if !ok {
			continue
		}
		if typed["image"] == supportServicesOperatorImage {
			typed["image"] = SupportServicesOperatorImage
		}
	}
	_ = unstructured.SetNestedSlice(object, containers, "spec", "template", "spec", "containers")
}
