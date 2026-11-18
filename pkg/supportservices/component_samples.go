package supportservices

import (
	applicationv1alpha1 "github.com/nukleros/support-services-operator/apis/application/v1alpha1"
	platformv1alpha1 "github.com/nukleros/support-services-operator/apis/platform/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ingressComponentSample is the IngressComponent this install starts from.
func ingressComponentSample() *platformv1alpha1.IngressComponent {
	return &platformv1alpha1.IngressComponent{
		TypeMeta: metav1.TypeMeta{
			APIVersion: platformv1alpha1.GroupVersion.String(),
			Kind:       "IngressComponent",
		},
		ObjectMeta: metav1.ObjectMeta{Name: "ingresscomponent-sample"},
		Spec: platformv1alpha1.IngressComponentSpec{
			Nginx: platformv1alpha1.IngressComponentSpecNginx{
				InstallType: "deployment",
				Include:     false,
				Image:       "nginx/nginx-ingress",
				Version:     "5.5.4",
				Replicas:    2,
			},
			Kong: platformv1alpha1.IngressComponentSpecKong{
				Include:  true,
				Replicas: 2,
				Gateway: platformv1alpha1.IngressComponentSpecKongGateway{
					Image:   "kong/kong-gateway",
					Version: "2.8",
				},
				ProxyServiceName: "kong-proxy",
				IngressController: platformv1alpha1.IngressComponentSpecKongIngressController{
					Image:   "kong/kubernetes-ingress-controller",
					Version: "2.5.0",
				},
			},
			ExternalDNS: platformv1alpha1.IngressComponentSpecExternalDNS{
				Provider:           "none",
				ZoneType:           "private",
				ExtraArgs:          []string{"{}"},
				Image:              "k8s.gcr.io/external-dns/external-dns",
				Version:            "v0.12.2",
				ServiceAccountName: "external-dns",
				IamRoleArn:         "iam_role_arn",
			},
			Namespace:  "nukleros-ingress-system",
			DomainName: "nukleros.io",
		},
	}
}

// certificatesComponentSample is the CertificatesComponent this install starts from.
func certificatesComponentSample() *platformv1alpha1.CertificatesComponent {
	return &platformv1alpha1.CertificatesComponent{
		TypeMeta: metav1.TypeMeta{
			APIVersion: platformv1alpha1.GroupVersion.String(),
			Kind:       "CertificatesComponent",
		},
		ObjectMeta: metav1.ObjectMeta{Name: "certificatescomponent-sample"},
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

// databaseComponentSample is the DatabaseComponent this install starts from.
func databaseComponentSample() *applicationv1alpha1.DatabaseComponent {
	return &applicationv1alpha1.DatabaseComponent{
		TypeMeta: metav1.TypeMeta{
			APIVersion: applicationv1alpha1.GroupVersion.String(),
			Kind:       "DatabaseComponent",
		},
		ObjectMeta: metav1.ObjectMeta{Name: "databasecomponent-sample"},
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

// secretsComponentSample is the SecretsComponent this install starts from.
func secretsComponentSample() *platformv1alpha1.SecretsComponent {
	return &platformv1alpha1.SecretsComponent{
		TypeMeta: metav1.TypeMeta{
			APIVersion: platformv1alpha1.GroupVersion.String(),
			Kind:       "SecretsComponent",
		},
		ObjectMeta: metav1.ObjectMeta{Name: "secretscomponent-sample"},
		Spec: platformv1alpha1.SecretsComponentSpec{
			Namespace: "nukleros-secrets-system",
			ExternalSecrets: platformv1alpha1.SecretsComponentSpecExternalSecrets{
				Image:   "ghcr.io/external-secrets/external-secrets",
				Version: "v0.9.11",
			},
		},
	}
}

// storageComponentSample is the StorageComponent this install starts from.
func storageComponentSample() *platformv1alpha1.StorageComponent {
	return &platformv1alpha1.StorageComponent{
		TypeMeta: metav1.TypeMeta{
			APIVersion: platformv1alpha1.GroupVersion.String(),
			Kind:       "StorageComponent",
		},
		ObjectMeta: metav1.ObjectMeta{Name: "storagecomponent-sample"},
		Spec: platformv1alpha1.StorageComponentSpec{
			Namespace: "nukleros-storage-system",
			SeaweedfsOperator: platformv1alpha1.StorageComponentSpecSeaweedfsOperator{
				Replicas: 1,
			},
		},
	}
}
