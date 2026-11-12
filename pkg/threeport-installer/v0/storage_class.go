package v0

import (
	"context"
	"errors"
	"fmt"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// defaultStorageClassAnnotation marks the StorageClass a claim that names
// none is provisioned from.
const defaultStorageClassAnnotation = "storageclass.kubernetes.io/is-default-class"

// betaDefaultStorageClassAnnotation is the older key for the same marker,
// which Kubernetes still honors.
const betaDefaultStorageClassAnnotation = "storageclass.beta.kubernetes.io/is-default-class"

// isDefaultStorageClass reports whether Kubernetes treats a class as the
// default: either key set to "true", as its own check reads them.
func isDefaultStorageClass(class unstructured.Unstructured) bool {
	annotations := class.GetAnnotations()

	return annotations[defaultStorageClassAnnotation] == "true" ||
		annotations[betaDefaultStorageClassAnnotation] == "true"
}

// EksDefaultStorageClassName is the default StorageClass threeport gives an
// EKS cluster. It is named for threeport rather than "gp3" so it never
// collides with, or changes, a class of that name a user created.
const EksDefaultStorageClassName = "threeport-gp3"

// storageClassGVR is the cluster-scoped storage.k8s.io/v1 storageclasses
// resource.
var storageClassGVR = schema.GroupVersionResource{Group: "storage.k8s.io", Version: "v1", Resource: "storageclasses"}

// eksDefaultStorageClass returns the default StorageClass for an EKS cluster:
// gp3 EBS volumes from the EBS CSI driver threeport installs as an addon.
//
// WaitForFirstConsumer creates the volume once a pod using it is scheduled,
// in that pod's availability zone - an EBS volume can only attach to a node in
// its own zone, so creating it first can strand it where no pod can use it.
func eksDefaultStorageClass() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "storage.k8s.io/v1",
		"kind":       "StorageClass",
		"metadata": map[string]interface{}{
			"name": EksDefaultStorageClassName,
			"annotations": map[string]interface{}{
				defaultStorageClassAnnotation: "true",
			},
			"labels": map[string]interface{}{
				LabelManagedBy: LabelManagedByValue,
			},
		},
		"provisioner": "ebs.csi.aws.com",
		"parameters": map[string]interface{}{
			"type": "gp3",
		},
		"reclaimPolicy":        "Delete",
		"volumeBindingMode":    "WaitForFirstConsumer",
		"allowVolumeExpansion": true,
	}}
}

// EnsureEksDefaultStorageClass gives an EKS cluster a default StorageClass
// when it has none, and reports whether it created one.
//
// Recent EKS versions no longer mark any class as the default, so a claim that
// does not name a class - as most workload manifests do not - has nothing to
// be provisioned from and stays pending, and its pods never start. A cluster
// that already has a default keeps it, whoever set it.
func EnsureEksDefaultStorageClass(kubeClient dynamic.Interface) (bool, error) {
	classes, err := kubeClient.Resource(storageClassGVR).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return false, fmt.Errorf("failed to list storage classes: %w", err)
	}
	for _, class := range classes.Items {
		if isDefaultStorageClass(class) {
			return false, nil
		}
	}

	_, err = kubeClient.Resource(storageClassGVR).Create(context.TODO(), eksDefaultStorageClass(), metav1.CreateOptions{})
	if k8serrors.IsAlreadyExists(err) {
		// ours, with its default marker removed by someone - left as they
		// left it
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to create default storage class %s: %w", EksDefaultStorageClassName, err)
	}

	return true, nil
}

// persistentVolumeClaimGVR is the core v1 persistentvolumeclaims resource.
var persistentVolumeClaimGVR = schema.GroupVersionResource{Version: "v1", Resource: "persistentvolumeclaims"}

// checkDatabaseStorage refuses a production install or reinstall whose
// database would get a volume before its pod is scheduled.  A replica whose
// claim already exists keeps its volume, whose zone is already set, so the
// storage class is checked only when a replica's claim has yet to be created.
// The class checked is the one the database's claims are created with: that
// of the database's StatefulSet, when it is there, as on a reinstall, and the
// installer's otherwise.
func (cpi *ControlPlaneInstaller) checkDatabaseStorage(kubeClient dynamic.Interface) error {
	if string(cpi.Opts.Tier) != ControlPlaneTierProd {
		return nil
	}

	for replica := int64(0); replica < crdbReplicas(cpi.Opts.Tier); replica++ {
		name := fmt.Sprintf("datadir-crdb-%d", replica)
		_, err := kubeClient.Resource(persistentVolumeClaimGVR).Namespace(cpi.Opts.Namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if k8serrors.IsNotFound(err) {
			claimSpec, err := cpi.databaseClaimSpec(kubeClient)
			if err != nil {
				return err
			}
			return checkDatabaseVolumeBinding(kubeClient, claimSpec)
		}
		if err != nil {
			return fmt.Errorf("failed to get the database's volume claim %s: %w", name, err)
		}
	}

	return nil
}

// databaseClaimSpec returns the spec the database's claims are created with.
// A database's StatefulSet that is already there, as on a reinstall, creates
// a missing replica's claim from its own template, which the installer's can
// differ from: a reinstall isn't given the infra provider whose storage class
// the template names.
func (cpi *ControlPlaneInstaller) databaseClaimSpec(kubeClient dynamic.Interface) (map[string]interface{}, error) {
	statefulSet, err := kubeClient.Resource(statefulSetGVR).Namespace(cpi.Opts.Namespace).Get(context.TODO(), "crdb", metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return cpi.getVolClaimTemplateSpec(""), nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get the database's stateful set: %w", err)
	}

	templates, _, _ := unstructured.NestedSlice(statefulSet.Object, "spec", "volumeClaimTemplates")
	for _, template := range templates {
		claim, ok := template.(map[string]interface{})
		if !ok {
			continue
		}
		if name, _, _ := unstructured.NestedString(claim, "metadata", "name"); name != "datadir" {
			continue
		}
		if spec, found, _ := unstructured.NestedMap(claim, "spec"); found {
			return spec, nil
		}
	}

	return nil, errors.New("the database's stateful set has no datadir volume claim template")
}

// checkDatabaseVolumeBinding refuses a production database whose volumes
// would be created before its pods are scheduled.
//
// A production database is spread strictly across zones, and a zonal volume
// holds its pod to the volume's zone.  A class that binds immediately creates
// each volume in a zone of the provisioner's choosing, and two of them in one
// zone leave a replica Pending: its zone already has one, and its volume
// can't follow it to another.  With WaitForFirstConsumer, the volume is
// created in the zone its pod is scheduled to.  The class is the one the
// database's claims name, or the cluster's default.  A cluster with neither
// is left to fail as it would have, on the claims.
func checkDatabaseVolumeBinding(kubeClient dynamic.Interface, claimSpec map[string]interface{}) error {
	var class *unstructured.Unstructured
	if name, ok := claimSpec["storageClassName"].(string); ok && name != "" {
		named, err := kubeClient.Resource(storageClassGVR).Get(context.TODO(), name, metav1.GetOptions{})
		if k8serrors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("failed to get storage class %s: %w", name, err)
		}
		class = named
	} else {
		classes, err := kubeClient.Resource(storageClassGVR).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return fmt.Errorf("failed to list storage classes: %w", err)
		}
		for i := range classes.Items {
			if isDefaultStorageClass(classes.Items[i]) {
				class = &classes.Items[i]
				break
			}
		}
	}
	if class == nil {
		return nil
	}

	// an unset binding mode is Immediate
	bindingMode, _, _ := unstructured.NestedString(class.Object, "volumeBindingMode")
	if bindingMode == "WaitForFirstConsumer" {
		return nil
	}
	if bindingMode == "" {
		bindingMode = "Immediate"
	}

	return fmt.Errorf(
		"storage class %s creates volumes before their pods are scheduled (volumeBindingMode %s): "+
			"a production control plane's database is spread across zones, and a volume created first can hold a replica "+
			"to a zone another replica already has; use a class with volumeBindingMode WaitForFirstConsumer for the database",
		class.GetName(), bindingMode,
	)
}
