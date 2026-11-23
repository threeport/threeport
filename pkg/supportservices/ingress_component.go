package supportservices

import (
	platformv1alpha1 "github.com/nukleros/support-services-operator/apis/platform/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ingressComponent is the IngressComponent this install starts from.
func ingressComponent() *platformv1alpha1.IngressComponent {
	return &platformv1alpha1.IngressComponent{
		TypeMeta: metav1.TypeMeta{
			APIVersion: platformv1alpha1.GroupVersion.String(),
			Kind:       "IngressComponent",
		},
		ObjectMeta: metav1.ObjectMeta{Name: IngressComponentName},
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
