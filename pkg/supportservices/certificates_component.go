package supportservices

import (
	platformv1alpha1 "github.com/nukleros/support-services-operator/apis/platform/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// certificatesComponent is the CertificatesComponent this install starts from.
func certificatesComponent() *platformv1alpha1.CertificatesComponent {
	return &platformv1alpha1.CertificatesComponent{
		TypeMeta: metav1.TypeMeta{
			APIVersion: platformv1alpha1.GroupVersion.String(),
			Kind:       "CertificatesComponent",
		},
		ObjectMeta: metav1.ObjectMeta{Name: CertificatesComponentName},
		Spec: platformv1alpha1.CertificatesComponentSpec{
			Namespace: "nukleros-certs-system",
			CertManager: platformv1alpha1.CertificatesComponentSpecCertManager{
				Cainjector: platformv1alpha1.CertificatesComponentSpecCertManagerCainjector{
					Replicas: 2,
					Image:    "quay.io/jetstack/cert-manager-cainjector",
				},
				Version: "v1.9.1",
				Controller: platformv1alpha1.CertificatesComponentSpecCertManagerController{
					Replicas: 2,
					Image:    "quay.io/jetstack/cert-manager-controller",
				},
				Webhook: platformv1alpha1.CertificatesComponentSpecCertManagerWebhook{
					Replicas: 2,
					Image:    "quay.io/jetstack/cert-manager-webhook",
				},
				ContactEmail: "admin@nukleros.io",
			},
		},
	}
}
