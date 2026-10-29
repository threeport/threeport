package v0

import (
	"context"
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
