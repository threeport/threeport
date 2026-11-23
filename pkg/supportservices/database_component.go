package supportservices

import (
	applicationv1alpha1 "github.com/nukleros/support-services-operator/apis/application/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// databaseComponent is the DatabaseComponent this install starts from.
func databaseComponent() *applicationv1alpha1.DatabaseComponent {
	return &applicationv1alpha1.DatabaseComponent{
		TypeMeta: metav1.TypeMeta{
			APIVersion: applicationv1alpha1.GroupVersion.String(),
			Kind:       "DatabaseComponent",
		},
		ObjectMeta: metav1.ObjectMeta{Name: DatabaseComponentName},
		Spec: applicationv1alpha1.DatabaseComponentSpec{
			Namespace: "nukleros-database-system",
			Provider:  "cloudnative-pg",
			CloudNativePg: applicationv1alpha1.DatabaseComponentSpecCloudNativePg{
				Image:    "ghcr.io/cloudnative-pg/cloudnative-pg",
				Replicas: 2,
				Version:  "1.30.1",
			},
		},
	}
}
