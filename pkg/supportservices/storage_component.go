package supportservices

import (
	platformv1alpha1 "github.com/nukleros/support-services-operator/apis/platform/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// storageComponent is the StorageComponent this install starts from.
func storageComponent() *platformv1alpha1.StorageComponent {
	return &platformv1alpha1.StorageComponent{
		TypeMeta: metav1.TypeMeta{
			APIVersion: platformv1alpha1.GroupVersion.String(),
			Kind:       "StorageComponent",
		},
		ObjectMeta: metav1.ObjectMeta{Name: StorageComponentName},
		Spec: platformv1alpha1.StorageComponentSpec{
			Namespace: "nukleros-storage-system",
			SeaweedfsOperator: platformv1alpha1.StorageComponentSpecSeaweedfsOperator{
				Replicas: 1,
			},
		},
	}
}
