package supportservices

import (
	platformv1alpha1 "github.com/nukleros/support-services-operator/apis/platform/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// secretsComponent is the SecretsComponent this install starts from.
func secretsComponent() *platformv1alpha1.SecretsComponent {
	return &platformv1alpha1.SecretsComponent{
		TypeMeta: metav1.TypeMeta{
			APIVersion: platformv1alpha1.GroupVersion.String(),
			Kind:       "SecretsComponent",
		},
		ObjectMeta: metav1.ObjectMeta{Name: SecretsComponentName},
		Spec: platformv1alpha1.SecretsComponentSpec{
			Namespace: "nukleros-secrets-system",
			ExternalSecrets: platformv1alpha1.SecretsComponentSpecExternalSecrets{
				Image:   "ghcr.io/external-secrets/external-secrets",
				Version: "v0.9.11",
			},
		},
	}
}
