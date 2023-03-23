package install

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/threeport/threeport/internal/tptctl/output"
)

const (
	SupportServicesOperatorManifestPath        = "/tmp/support-services-operator.yaml"
	SupportServicesCRDsManifestPath            = "/tmp/support-services-crds.yaml"
	SupportServicesComponentsManifestPath      = "/tmp/support-services-components.yaml"
	SupportServicesOperatorImage               = "ghcr.io/nukleros/support-services-operator:v0.1.12"
	SupportServicesIngressComponentName        = "threeport-control-plane-ingress"
	SupportServicesIngressNamespace            = "threeport-ingress"
	SupportServicesIngressServiceName          = "threeport-ingress-service"
	SupportServicesDNSManagementServiceAccount = "external-dns"
)

// InstallSupportServicesOperator installs the support services operator into a
// target control plane or compute space cluster.
// https://github.com/nukleros/support-services-operator
func InstallSupportServicesOperator(
	kubeconfig string,
	iamDNSRoleARN string,
	rootDomain string,
	adminEmail string,
) (string, error) {
	var loadBalancerURL string
	// write support services operator manifests to /tmp directory
	supportServicesCRDsManifest, err := os.Create(SupportServicesCRDsManifestPath)
	if err != nil {
		return loadBalancerURL, fmt.Errorf("failed to write support services CRDs manifest to disk: %w", err)
	}
	defer supportServicesCRDsManifest.Close()
	supportServicesCRDsManifest.WriteString(SupportServicesCRDsManifest())

	supportServicesOperatorManifest, err := os.Create(SupportServicesOperatorManifestPath)
	if err != nil {
		return loadBalancerURL, fmt.Errorf("failed to write support services operator manifest to disk: %w", err)
	}
	defer supportServicesOperatorManifest.Close()
	supportServicesOperatorManifest.WriteString(SupportServicesOperatorManifest())

	supportServicesComponentsManifest, err := os.Create(SupportServicesComponentsManifestPath)
	if err != nil {
		return loadBalancerURL, fmt.Errorf("failed to write support services operator manifest to disk: %w", err)
	}
	defer supportServicesComponentsManifest.Close()
	supportServicesComponentsManifest.WriteString(
		SupportServicesComponentsManifest(iamDNSRoleARN, rootDomain, adminEmail))
	output.Info("Threeport support services operator manifests written to /tmp directory")

	// install support services CRDs
	supportServicesCRDsCreate := exec.Command(
		"kubectl",
		"--kubeconfig",
		kubeconfig,
		"apply",
		"-f",
		SupportServicesCRDsManifestPath,
	)
	supportServicesCRDsCreateOut, err := supportServicesCRDsCreate.CombinedOutput()
	if err != nil {
		output.Error(fmt.Sprintf("kubectl error: %s", supportServicesCRDsCreateOut), nil)
		return loadBalancerURL, fmt.Errorf("failed to create support services custom resource definitions: %w", err)
	}

	// install support services operator
	supportServicesOperatorCreate := exec.Command(
		"kubectl",
		"--kubeconfig",
		kubeconfig,
		"apply",
		"-f",
		SupportServicesOperatorManifestPath,
	)
	supportServicesOperatorCreateOut, err := supportServicesOperatorCreate.CombinedOutput()
	if err != nil {
		output.Error(fmt.Sprintf("kubectl error: %s", supportServicesOperatorCreateOut), nil)
		return loadBalancerURL, fmt.Errorf("failed to create support services operator: %w", err)
	}

	// install support services components
	supportServicesComponentsCreate := exec.Command(
		"kubectl",
		"--kubeconfig",
		kubeconfig,
		"apply",
		"-f",
		SupportServicesComponentsManifestPath,
	)
	supportServicesComponentsCreateOut, err := supportServicesComponentsCreate.CombinedOutput()
	if err != nil {
		output.Error(fmt.Sprintf("kubectl error: %s", supportServicesComponentsCreateOut), nil)
		return loadBalancerURL, fmt.Errorf("failed to create support services operator: %w", err)
	}

	output.Info("Threeport support services operator created")

	return loadBalancerURL, nil
}

// UninstallIngressComponent removes the support services ingress component.
// This must be done before deleting cluster infra so the load balancer for the
// ingress layer is deleted.
func UninstallIngressComponent(kubeconfig string) error {
	supportServicesIngressComponentDelete := exec.Command(
		"kubectl",
		"--kubeconfig",
		kubeconfig,
		"delete",
		"ingresscomponent",
		SupportServicesIngressComponentName,
	)
	supportServicesIngressComponentDeleteOut, err := supportServicesIngressComponentDelete.CombinedOutput()
	if err != nil {
		output.Error(fmt.Sprintf("kubectl error: %s", supportServicesIngressComponentDeleteOut), nil)
		return fmt.Errorf("failed to delete support services ingress component: %w", err)
	}

	output.Info("Threeport ingress component removed")

	return nil
}

// SupportServicesOperatorManifest returns a yaml manifest for the support
// service operator.
func SupportServicesOperatorManifest() string {
	return fmt.Sprintf(`---
apiVersion: v1
kind: Namespace
metadata:
  labels:
    control-plane: controller-manager
    kubernetes.io/metadata.name: support-services-operator-system
  name: support-services-operator-system
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: support-services-operator-controller-manager
  namespace: support-services-operator-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: support-services-operator-leader-election-role
  namespace: support-services-operator-system
rules:
- apiGroups:
  - ""
  resources:
  - configmaps
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - patch
  - delete
- apiGroups:
  - coordination.k8s.io
  resources:
  - leases
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - patch
  - delete
- apiGroups:
  - ""
  resources:
  - events
  verbs:
  - create
  - patch
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: support-services-operator-manager-role
rules:
- apiGroups:
  - acid.zalan.do
  resources:
  - operatorconfigurations
  verbs:
  - create
  - delete
  - deletecollection
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - acid.zalan.do
  resources:
  - postgresqls
  verbs:
  - create
  - delete
  - deletecollection
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - acid.zalan.do
  resources:
  - postgresqls/status
  verbs:
  - create
  - delete
  - deletecollection
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - acid.zalan.do
  resources:
  - postgresteams
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - acme.cert-manager.io
  resources:
  - challenges
  verbs:
  - create
  - delete
  - deletecollection
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - acme.cert-manager.io
  resources:
  - challenges/finalizers
  verbs:
  - update
- apiGroups:
  - acme.cert-manager.io
  resources:
  - challenges/status
  verbs:
  - patch
  - update
- apiGroups:
  - acme.cert-manager.io
  resources:
  - orders
  verbs:
  - create
  - delete
  - deletecollection
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - acme.cert-manager.io
  resources:
  - orders/finalizers
  verbs:
  - update
- apiGroups:
  - acme.cert-manager.io
  resources:
  - orders/status
  verbs:
  - patch
  - update
- apiGroups:
  - admissionregistration.k8s.io
  resources:
  - mutatingwebhookconfigurations
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - admissionregistration.k8s.io
  resources:
  - validatingwebhookconfigurations
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - apiextensions.k8s.io
  resources:
  - customresourcedefinitions
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - apiregistration.k8s.io
  resources:
  - apiservices
  verbs:
  - get
  - list
  - update
  - watch
- apiGroups:
  - application.addons.nukleros.io
  resources:
  - databasecomponents
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - application.addons.nukleros.io
  resources:
  - databasecomponents/status
  verbs:
  - get
  - patch
  - update
- apiGroups:
  - apps
  resources:
  - daemonsets
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - apps
  resources:
  - deployments
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - apps
  resources:
  - statefulsets
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
- apiGroups:
  - authorization.k8s.io
  resources:
  - subjectaccessreviews
  verbs:
  - create
- apiGroups:
  - batch
  resources:
  - cronjobs
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
- apiGroups:
  - cert-manager.io
  resources:
  - certificaterequests
  verbs:
  - create
  - delete
  - deletecollection
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - cert-manager.io
  resources:
  - certificaterequests/finalizers
  verbs:
  - update
- apiGroups:
  - cert-manager.io
  resources:
  - certificaterequests/status
  verbs:
  - patch
  - update
- apiGroups:
  - cert-manager.io
  resources:
  - certificates
  verbs:
  - create
  - delete
  - deletecollection
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - cert-manager.io
  resources:
  - certificates/finalizers
  verbs:
  - update
- apiGroups:
  - cert-manager.io
  resources:
  - certificates/status
  verbs:
  - patch
  - update
- apiGroups:
  - cert-manager.io
  resources:
  - clusterissuers
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - cert-manager.io
  resources:
  - clusterissuers/status
  verbs:
  - patch
  - update
- apiGroups:
  - cert-manager.io
  resources:
  - issuers
  verbs:
  - create
  - delete
  - deletecollection
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - cert-manager.io
  resources:
  - issuers/status
  verbs:
  - patch
  - update
- apiGroups:
  - cert-manager.io
  resources:
  - signers
  verbs:
  - approve
- apiGroups:
  - certificates.k8s.io
  resources:
  - certificatesigningrequests
  verbs:
  - get
  - list
  - update
  - watch
- apiGroups:
  - certificates.k8s.io
  resources:
  - certificatesigningrequests/status
  verbs:
  - patch
  - update
- apiGroups:
  - certificates.k8s.io
  resources:
  - signers
  verbs:
  - sign
- apiGroups:
  - cis.f5.com
  resources:
  - ingresslinks
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - configuration.konghq.com
  resources:
  - kongclusterplugins
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - configuration.konghq.com
  resources:
  - kongclusterplugins/status
  verbs:
  - get
  - patch
  - update
- apiGroups:
  - configuration.konghq.com
  resources:
  - kongconsumers
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - configuration.konghq.com
  resources:
  - kongconsumers/status
  verbs:
  - get
  - patch
  - update
- apiGroups:
  - configuration.konghq.com
  resources:
  - kongingresses
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - configuration.konghq.com
  resources:
  - kongingresses/status
  verbs:
  - get
  - patch
  - update
- apiGroups:
  - configuration.konghq.com
  resources:
  - kongplugins
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - configuration.konghq.com
  resources:
  - kongplugins/status
  verbs:
  - get
  - patch
  - update
- apiGroups:
  - configuration.konghq.com
  resources:
  - tcpingresses
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - configuration.konghq.com
  resources:
  - tcpingresses/status
  verbs:
  - get
  - patch
  - update
- apiGroups:
  - configuration.konghq.com
  resources:
  - udpingresses
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - configuration.konghq.com
  resources:
  - udpingresses/status
  verbs:
  - get
  - patch
  - update
- apiGroups:
  - coordination.k8s.io
  resources:
  - configmaps
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - coordination.k8s.io
  resources:
  - leases
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - ""
  resources:
  - configmaps
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - ""
  resources:
  - deployments
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - ""
  resources:
  - endpoints
  verbs:
  - create
  - delete
  - deletecollection
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - ""
  resources:
  - endpoints/status
  verbs:
  - get
  - patch
  - update
- apiGroups:
  - ""
  resources:
  - events
  verbs:
  - create
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - ""
  resources:
  - leases
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - ""
  resources:
  - namespaces
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - ""
  resources:
  - nodes
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - ""
  resources:
  - persistentvolumeclaims
  verbs:
  - delete
  - get
  - list
  - patch
  - update
- apiGroups:
  - ""
  resources:
  - persistentvolumes
  verbs:
  - get
  - list
  - update
- apiGroups:
  - ""
  resources:
  - pods
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - ""
  resources:
  - pods/exec
  verbs:
  - create
- apiGroups:
  - ""
  resources:
  - secrets
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - ""
  resources:
  - secrets/status
  verbs:
  - get
  - patch
  - update
- apiGroups:
  - ""
  resources:
  - serviceaccounts
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - ""
  resources:
  - serviceaccounts/token
  verbs:
  - create
- apiGroups:
  - ""
  resources:
  - services
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - ""
  resources:
  - services/status
  verbs:
  - get
  - patch
  - update
- apiGroups:
  - extensions
  resources:
  - daemonsets
  verbs:
  - get
  - list
  - patch
  - update
- apiGroups:
  - extensions
  resources:
  - deployments
  verbs:
  - get
  - list
  - patch
  - update
- apiGroups:
  - extensions
  resources:
  - ingresses
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - extensions
  resources:
  - ingresses/status
  verbs:
  - get
  - patch
  - update
- apiGroups:
  - external-secrets.io
  resources:
  - clusterexternalsecrets
  verbs:
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - external-secrets.io
  resources:
  - clusterexternalsecrets/finalizers
  verbs:
  - patch
  - update
- apiGroups:
  - external-secrets.io
  resources:
  - clusterexternalsecrets/status
  verbs:
  - patch
  - update
- apiGroups:
  - external-secrets.io
  resources:
  - clustersecretstores
  verbs:
  - create
  - delete
  - deletecollection
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - external-secrets.io
  resources:
  - clustersecretstores/finalizers
  verbs:
  - patch
  - update
- apiGroups:
  - external-secrets.io
  resources:
  - clustersecretstores/status
  verbs:
  - patch
  - update
- apiGroups:
  - external-secrets.io
  resources:
  - externalsecrets
  verbs:
  - create
  - delete
  - deletecollection
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - external-secrets.io
  resources:
  - externalsecrets/finalizers
  verbs:
  - patch
  - update
- apiGroups:
  - external-secrets.io
  resources:
  - externalsecrets/status
  verbs:
  - patch
  - update
- apiGroups:
  - external-secrets.io
  resources:
  - secretstores
  verbs:
  - create
  - delete
  - deletecollection
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - external-secrets.io
  resources:
  - secretstores/finalizers
  verbs:
  - patch
  - update
- apiGroups:
  - external-secrets.io
  resources:
  - secretstores/status
  verbs:
  - patch
  - update
- apiGroups:
  - externaldns.nginx.org
  resources:
  - dnsendpoints
  verbs:
  - create
  - delete
  - get
  - list
  - update
  - watch
- apiGroups:
  - externaldns.nginx.org
  resources:
  - dnsendpoints/status
  verbs:
  - update
- apiGroups:
  - gateway.networking.k8s.io
  resources:
  - gatewayclasses
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - gateway.networking.k8s.io
  resources:
  - gatewayclasses/status
  verbs:
  - get
  - update
- apiGroups:
  - gateway.networking.k8s.io
  resources:
  - gateways
  verbs:
  - get
  - list
  - update
  - watch
- apiGroups:
  - gateway.networking.k8s.io
  resources:
  - gateways/finalizers
  verbs:
  - update
- apiGroups:
  - gateway.networking.k8s.io
  resources:
  - gateways/status
  verbs:
  - get
  - update
- apiGroups:
  - gateway.networking.k8s.io
  resources:
  - httproutes
  verbs:
  - create
  - delete
  - get
  - list
  - update
  - watch
- apiGroups:
  - gateway.networking.k8s.io
  resources:
  - httproutes/finalizers
  verbs:
  - update
- apiGroups:
  - gateway.networking.k8s.io
  resources:
  - httproutes/status
  verbs:
  - get
  - update
- apiGroups:
  - gateway.networking.k8s.io
  resources:
  - referencepolicies
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - gateway.networking.k8s.io
  resources:
  - referencepolicies/finalizers
  verbs:
  - update
- apiGroups:
  - gateway.networking.k8s.io
  resources:
  - referencepolicies/status
  verbs:
  - get
  - patch
  - update
- apiGroups:
  - gateway.networking.k8s.io
  resources:
  - tcproutes
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - gateway.networking.k8s.io
  resources:
  - tcproutes/status
  verbs:
  - get
  - update
- apiGroups:
  - gateway.networking.k8s.io
  resources:
  - tlsroutes
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - gateway.networking.k8s.io
  resources:
  - tlsroutes/status
  verbs:
  - get
  - update
- apiGroups:
  - gateway.networking.k8s.io
  resources:
  - udproutes
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - gateway.networking.k8s.io
  resources:
  - udproutes/status
  verbs:
  - get
  - update
- apiGroups:
  - k8s.nginx.org
  resources:
  - dnsendpoints/status
  verbs:
  - update
- apiGroups:
  - k8s.nginx.org
  resources:
  - globalconfigurations
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - k8s.nginx.org
  resources:
  - policies
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - k8s.nginx.org
  resources:
  - policies/status
  verbs:
  - update
- apiGroups:
  - k8s.nginx.org
  resources:
  - transportservers
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - k8s.nginx.org
  resources:
  - transportservers/status
  verbs:
  - update
- apiGroups:
  - k8s.nginx.org
  resources:
  - virtualserverroutes
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - k8s.nginx.org
  resources:
  - virtualserverroutes/status
  verbs:
  - update
- apiGroups:
  - k8s.nginx.org
  resources:
  - virtualservers
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - k8s.nginx.org
  resources:
  - virtualservers/status
  verbs:
  - update
- apiGroups:
  - networking.internal.knative.dev
  resources:
  - ingresses
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - networking.internal.knative.dev
  resources:
  - ingresses/status
  verbs:
  - get
  - patch
  - update
- apiGroups:
  - networking.k8s.io
  resources:
  - ingressclasses
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - networking.k8s.io
  resources:
  - ingresses
  verbs:
  - create
  - delete
  - get
  - list
  - update
  - watch
- apiGroups:
  - networking.k8s.io
  resources:
  - ingresses/finalizers
  verbs:
  - update
- apiGroups:
  - networking.k8s.io
  resources:
  - ingresses/status
  verbs:
  - get
  - patch
  - update
- apiGroups:
  - platform.addons.nukleros.io
  resources:
  - certificatescomponents
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - platform.addons.nukleros.io
  resources:
  - certificatescomponents/status
  verbs:
  - get
  - patch
  - update
- apiGroups:
  - platform.addons.nukleros.io
  resources:
  - ingresscomponents
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - platform.addons.nukleros.io
  resources:
  - ingresscomponents/status
  verbs:
  - get
  - patch
  - update
- apiGroups:
  - platform.addons.nukleros.io
  resources:
  - secretscomponents
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - platform.addons.nukleros.io
  resources:
  - secretscomponents/status
  verbs:
  - get
  - patch
  - update
- apiGroups:
  - policy
  resources:
  - poddisruptionbudgets
  verbs:
  - create
  - delete
  - get
- apiGroups:
  - rbac.authorization.k8s.io
  resources:
  - clusterrolebindings
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - rbac.authorization.k8s.io
  resources:
  - clusterroles
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - rbac.authorization.k8s.io
  resources:
  - rolebindings
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - rbac.authorization.k8s.io
  resources:
  - roles
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - route.openshift.io
  resources:
  - routes/custom-host
  verbs:
  - create
- apiGroups:
  - setup.addons.nukleros.io
  resources:
  - supportservices
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - setup.addons.nukleros.io
  resources:
  - supportservices/status
  verbs:
  - get
  - patch
  - update
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: support-services-operator-metrics-reader
rules:
- nonResourceURLs:
  - /metrics
  verbs:
  - get
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: support-services-operator-proxy-role
rules:
- apiGroups:
  - authentication.k8s.io
  resources:
  - tokenreviews
  verbs:
  - create
- apiGroups:
  - authorization.k8s.io
  resources:
  - subjectaccessreviews
  verbs:
  - create
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: support-services-operator-leader-election-rolebinding
  namespace: support-services-operator-system
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: support-services-operator-leader-election-role
subjects:
- kind: ServiceAccount
  name: support-services-operator-controller-manager
  namespace: support-services-operator-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: support-services-operator-manager-rolebinding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: support-services-operator-manager-role
subjects:
- kind: ServiceAccount
  name: support-services-operator-controller-manager
  namespace: support-services-operator-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: support-services-operator-proxy-rolebinding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: support-services-operator-proxy-role
subjects:
- kind: ServiceAccount
  name: support-services-operator-controller-manager
  namespace: support-services-operator-system
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: support-services-operator-manager-config
  namespace: support-services-operator-system
data:
  controller_manager_config.yaml: |
    apiVersion: controller-runtime.sigs.k8s.io/v1alpha1
    kind: ControllerManagerConfig
    health:
      healthProbeBindAddress: :8081
    metrics:
      bindAddress: 127.0.0.1:8080
    webhook:
      port: 9443
    leaderElection:
      leaderElect: true
      resourceName: bb9cd6ef.addons.nukleros.io
---
apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    control-plane: controller-manager
  name: support-services-operator-controller-manager
  namespace: support-services-operator-system
spec:
  progressDeadlineSeconds: 600
  replicas: 1
  revisionHistoryLimit: 10
  selector:
    matchLabels:
      control-plane: controller-manager
  strategy:
    rollingUpdate:
      maxSurge: 25%%
      maxUnavailable: 25%%
    type: RollingUpdate
  template:
    metadata:
      creationTimestamp: null
      labels:
        control-plane: controller-manager
    spec:
      containers:
      - args:
        - --secure-listen-address=0.0.0.0:8443
        - --upstream=http://127.0.0.1:8080/
        - --logtostderr=true
        - --v=10
        image: gcr.io/kubebuilder/kube-rbac-proxy:v0.8.0
        imagePullPolicy: IfNotPresent
        name: kube-rbac-proxy
        ports:
        - containerPort: 8443
          name: https
          protocol: TCP
        resources: {}
        terminationMessagePath: /dev/termination-log
        terminationMessagePolicy: File
      - args:
        - --health-probe-bind-address=:8081
        - --metrics-bind-address=127.0.0.1:8080
        - --leader-elect
        command:
        - /manager
        image: %[1]s
        imagePullPolicy: IfNotPresent
        livenessProbe:
          failureThreshold: 3
          httpGet:
            path: /healthz
            port: 8081
            scheme: HTTP
          initialDelaySeconds: 15
          periodSeconds: 20
          successThreshold: 1
          timeoutSeconds: 1
        name: manager
        readinessProbe:
          failureThreshold: 3
          httpGet:
            path: /readyz
            port: 8081
            scheme: HTTP
          initialDelaySeconds: 5
          periodSeconds: 10
          successThreshold: 1
          timeoutSeconds: 1
        resources:
          limits:
            cpu: 100m
            memory: 30Mi
          requests:
            cpu: 100m
            memory: 20Mi
        securityContext:
          allowPrivilegeEscalation: false
        terminationMessagePath: /dev/termination-log
        terminationMessagePolicy: File
      dnsPolicy: ClusterFirst
      restartPolicy: Always
      schedulerName: default-scheduler
      securityContext:
        runAsNonRoot: true
        fsGroup: 2000
        runAsUser: 1000
      serviceAccount: support-services-operator-controller-manager
      serviceAccountName: support-services-operator-controller-manager
      terminationGracePeriodSeconds: 10
---
apiVersion: v1
kind: Service
metadata:
  labels:
    control-plane: controller-manager
  name: support-services-operator-controller-manager-metrics-service
  namespace: support-services-operator-system
spec:
  internalTrafficPolicy: Cluster
  ipFamilies:
  - IPv4
  ipFamilyPolicy: SingleStack
  ports:
  - name: https
    port: 8443
    protocol: TCP
    targetPort: https
  selector:
    control-plane: controller-manager
  sessionAffinity: None
  type: ClusterIP
`, SupportServicesOperatorImage)
}

// SupportServicesCRDsManifest returns a yaml manifest for the support
// service operator.
func SupportServicesCRDsManifest() string {
	return fmt.Sprintf(`---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  annotations:
    controller-gen.kubebuilder.io/version: v0.9.0
  creationTimestamp: null
  name: certificatescomponents.platform.addons.nukleros.io
spec:
  group: platform.addons.nukleros.io
  names:
    kind: CertificatesComponent
    listKind: CertificatesComponentList
    plural: certificatescomponents
    singular: certificatescomponent
  scope: Cluster
  versions:
  - name: v1alpha1
    schema:
      openAPIV3Schema:
        description: CertificatesComponent is the Schema for the certificatescomponents
          API.
        properties:
          apiVersion:
            description: 'APIVersion defines the versioned schema of this representation
              of an object. Servers should convert recognized schemas to the latest
              internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources'
            type: string
          kind:
            description: 'Kind is a string value representing the REST resource this
              object represents. Servers may infer this from the endpoint the client
              submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds'
            type: string
          metadata:
            type: object
          spec:
            description: CertificatesComponentSpec defines the desired state of CertificatesComponent.
            properties:
              certManager:
                properties:
                  cainjector:
                    properties:
                      image:
                        default: quay.io/jetstack/cert-manager-cainjector
                        description: "(Default: \"quay.io/jetstack/cert-manager-cainjector\")
                          \n Image repo and name to use for cert-manager cainjector."
                        type: string
                      replicas:
                        default: 2
                        description: "(Default: 2) \n Number of replicas to use for
                          the cert-manager cainjector deployment."
                        type: integer
                    type: object
                  contactEmail:
                    description: Contact e-mail address for receiving updates about
                      certificates from LetsEncrypt.
                    type: string
                  controller:
                    properties:
                      image:
                        default: quay.io/jetstack/cert-manager-controller
                        description: "(Default: \"quay.io/jetstack/cert-manager-controller\")
                          \n Image repo and name to use for cert-manager controller."
                        type: string
                      replicas:
                        default: 2
                        description: "(Default: 2) \n Number of replicas to use for
                          the cert-manager controller deployment."
                        type: integer
                    type: object
                  version:
                    default: v1.9.1
                    description: "(Default: \"v1.9.1\") \n Version of cert-manager
                      to use."
                    type: string
                  webhook:
                    properties:
                      image:
                        default: quay.io/jetstack/cert-manager-webhook
                        description: "(Default: \"quay.io/jetstack/cert-manager-webhook\")
                          \n Image repo and name to use for cert-manager webhook."
                        type: string
                      replicas:
                        default: 2
                        description: "(Default: 2) \n Number of replicas to use for
                          the cert-manager webhook deployment."
                        type: integer
                    type: object
                type: object
              collection:
                description: Specifies a reference to the collection to use for this
                  workload. Requires the name and namespace input to find the collection.
                  If no collection field is set, default to selecting the only workload
                  collection in the cluster, which will result in an error if not
                  exactly one collection is found.
                properties:
                  name:
                    description: Required if specifying collection.  The name of the
                      collection within a specific collection.namespace to reference.
                    type: string
                  namespace:
                    description: '(Default: "") The namespace where the collection
                      exists.  Required only if the collection is namespace scoped
                      and not cluster scoped.'
                    type: string
                required:
                - name
                type: object
              namespace:
                default: nukleros-certs-system
                description: "(Default: \"nukleros-certs-system\") \n Namespace to
                  use for certificate support services."
                type: string
            type: object
          status:
            description: CertificatesComponentStatus defines the observed state of
              CertificatesComponent.
            properties:
              conditions:
                items:
                  description: PhaseCondition describes an event that has occurred
                    during a phase of the controller reconciliation loop.
                  properties:
                    lastModified:
                      description: LastModified defines the time in which this component
                        was updated.
                      type: string
                    message:
                      description: Message defines a helpful message from the phase.
                      type: string
                    phase:
                      description: Phase defines the phase in which the condition
                        was set.
                      type: string
                    state:
                      description: PhaseState defines the current state of the phase.
                      enum:
                      - Complete
                      - Reconciling
                      - Failed
                      - Pending
                      type: string
                  required:
                  - lastModified
                  - message
                  - phase
                  - state
                  type: object
                type: array
              created:
                type: boolean
              dependenciesSatisfied:
                type: boolean
              resources:
                items:
                  description: ChildResource is the resource and its condition as
                    stored on the workload custom resource's status field.
                  properties:
                    condition:
                      description: ResourceCondition defines the current condition
                        of this resource.
                      properties:
                        created:
                          description: Created defines whether this object has been
                            successfully created or not.
                          type: boolean
                        lastModified:
                          description: LastModified defines the time in which this
                            resource was updated.
                          type: string
                        message:
                          description: Message defines a helpful message from the
                            resource phase.
                          type: string
                      required:
                      - created
                      type: object
                    group:
                      description: Group defines the API Group of the resource.
                      type: string
                    kind:
                      description: Kind defines the kind of the resource.
                      type: string
                    name:
                      description: Name defines the name of the resource from the
                        metadata.name field.
                      type: string
                    namespace:
                      description: Namespace defines the namespace in which this resource
                        exists in.
                      type: string
                    version:
                      description: Version defines the API Version of the resource.
                      type: string
                  required:
                  - group
                  - kind
                  - name
                  - namespace
                  - version
                  type: object
                type: array
            type: object
        type: object
    served: true
    storage: true
    subresources:
      status: {}
---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  annotations:
    controller-gen.kubebuilder.io/version: v0.9.0
  creationTimestamp: null
  name: databasecomponents.application.addons.nukleros.io
spec:
  group: application.addons.nukleros.io
  names:
    kind: DatabaseComponent
    listKind: DatabaseComponentList
    plural: databasecomponents
    singular: databasecomponent
  scope: Cluster
  versions:
  - name: v1alpha1
    schema:
      openAPIV3Schema:
        description: DatabaseComponent is the Schema for the databasecomponents API.
        properties:
          apiVersion:
            description: 'APIVersion defines the versioned schema of this representation
              of an object. Servers should convert recognized schemas to the latest
              internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources'
            type: string
          kind:
            description: 'Kind is a string value representing the REST resource this
              object represents. Servers may infer this from the endpoint the client
              submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds'
            type: string
          metadata:
            type: object
          spec:
            description: DatabaseComponentSpec defines the desired state of DatabaseComponent.
            properties:
              collection:
                description: Specifies a reference to the collection to use for this
                  workload. Requires the name and namespace input to find the collection.
                  If no collection field is set, default to selecting the only workload
                  collection in the cluster, which will result in an error if not
                  exactly one collection is found.
                properties:
                  name:
                    description: Required if specifying collection.  The name of the
                      collection within a specific collection.namespace to reference.
                    type: string
                  namespace:
                    description: '(Default: "") The namespace where the collection
                      exists.  Required only if the collection is namespace scoped
                      and not cluster scoped.'
                    type: string
                required:
                - name
                type: object
              namespace:
                default: nukleros-database-system
                description: "(Default: \"nukleros-database-system\") \n Namespace
                  to use for database support services."
                type: string
              zalandoPostgres:
                properties:
                  image:
                    default: registry.opensource.zalan.do/acid/postgres-operator
                    description: "(Default: \"registry.opensource.zalan.do/acid/postgres-operator\")
                      \n Image repo and name to use for postgres operator."
                    type: string
                  replicas:
                    default: 1
                    description: "(Default: 1) \n Number of replicas to use for the
                      postgres-operator deployment."
                    type: integer
                  version:
                    default: v1.8.2
                    description: "(Default: \"v1.8.2\") \n Version of postgres operator
                      to use."
                    type: string
                type: object
            type: object
          status:
            description: DatabaseComponentStatus defines the observed state of DatabaseComponent.
            properties:
              conditions:
                items:
                  description: PhaseCondition describes an event that has occurred
                    during a phase of the controller reconciliation loop.
                  properties:
                    lastModified:
                      description: LastModified defines the time in which this component
                        was updated.
                      type: string
                    message:
                      description: Message defines a helpful message from the phase.
                      type: string
                    phase:
                      description: Phase defines the phase in which the condition
                        was set.
                      type: string
                    state:
                      description: PhaseState defines the current state of the phase.
                      enum:
                      - Complete
                      - Reconciling
                      - Failed
                      - Pending
                      type: string
                  required:
                  - lastModified
                  - message
                  - phase
                  - state
                  type: object
                type: array
              created:
                type: boolean
              dependenciesSatisfied:
                type: boolean
              resources:
                items:
                  description: ChildResource is the resource and its condition as
                    stored on the workload custom resource's status field.
                  properties:
                    condition:
                      description: ResourceCondition defines the current condition
                        of this resource.
                      properties:
                        created:
                          description: Created defines whether this object has been
                            successfully created or not.
                          type: boolean
                        lastModified:
                          description: LastModified defines the time in which this
                            resource was updated.
                          type: string
                        message:
                          description: Message defines a helpful message from the
                            resource phase.
                          type: string
                      required:
                      - created
                      type: object
                    group:
                      description: Group defines the API Group of the resource.
                      type: string
                    kind:
                      description: Kind defines the kind of the resource.
                      type: string
                    name:
                      description: Name defines the name of the resource from the
                        metadata.name field.
                      type: string
                    namespace:
                      description: Namespace defines the namespace in which this resource
                        exists in.
                      type: string
                    version:
                      description: Version defines the API Version of the resource.
                      type: string
                  required:
                  - group
                  - kind
                  - name
                  - namespace
                  - version
                  type: object
                type: array
            type: object
        type: object
    served: true
    storage: true
    subresources:
      status: {}
---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  annotations:
    controller-gen.kubebuilder.io/version: v0.9.0
  creationTimestamp: null
  name: ingresscomponents.platform.addons.nukleros.io
spec:
  group: platform.addons.nukleros.io
  names:
    kind: IngressComponent
    listKind: IngressComponentList
    plural: ingresscomponents
    singular: ingresscomponent
  scope: Cluster
  versions:
  - name: v1alpha1
    schema:
      openAPIV3Schema:
        description: IngressComponent is the Schema for the ingresscomponents API.
        properties:
          apiVersion:
            description: 'APIVersion defines the versioned schema of this representation
              of an object. Servers should convert recognized schemas to the latest
              internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources'
            type: string
          kind:
            description: 'Kind is a string value representing the REST resource this
              object represents. Servers may infer this from the endpoint the client
              submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds'
            type: string
          metadata:
            type: object
          spec:
            description: IngressComponentSpec defines the desired state of IngressComponent.
            properties:
              collection:
                description: Specifies a reference to the collection to use for this
                  workload. Requires the name and namespace input to find the collection.
                  If no collection field is set, default to selecting the only workload
                  collection in the cluster, which will result in an error if not
                  exactly one collection is found.
                properties:
                  name:
                    description: Required if specifying collection.  The name of the
                      collection within a specific collection.namespace to reference.
                    type: string
                  namespace:
                    description: '(Default: "") The namespace where the collection
                      exists.  Required only if the collection is namespace scoped
                      and not cluster scoped.'
                    type: string
                required:
                - name
                type: object
              domainName:
                type: string
              externalDNS:
                properties:
                  iamRoleArn:
                    description: On AWS, the IAM Role ARN that gives external-dns
                      access to Route53
                    type: string
                  image:
                    default: k8s.gcr.io/external-dns/external-dns
                    description: "(Default: \"k8s.gcr.io/external-dns/external-dns\")
                      \n Image repo and name to use for external-dns."
                    type: string
                  provider:
                    default: none
                    description: "(Default: \"none\") \n The DNS provider to use for
                      setting DNS records with external-dns.  One of: none | active-directory
                      | google | route53."
                    enum:
                    - none
                    - active-directory
                    - google
                    - route53
                    type: string
                  serviceAccountName:
                    default: external-dns
                    description: "(Default: \"external-dns\") \n The name of the external-dns
                      service account which is referenced in role policy doc for AWS."
                    type: string
                  version:
                    default: v0.12.2
                    description: "(Default: \"v0.12.2\") \n Version of external-dns
                      to use."
                    type: string
                  zoneType:
                    default: private
                    description: "(Default: \"private\") \n Type of DNS hosted zone
                      to manage."
                    enum:
                    - private
                    - public
                    type: string
                type: object
              kong:
                properties:
                  gateway:
                    properties:
                      image:
                        default: kong/kong-gateway
                        description: "(Default: \"kong/kong-gateway\") \n Image repo
                          and name to use for kong gateway."
                        type: string
                      version:
                        default: "2.8"
                        description: "(Default: \"2.8\") \n Version of kong gateway
                          to use."
                        type: string
                    type: object
                  include:
                    default: true
                    description: "(Default: true) \n Include the Kong ingress controller
                      when installing ingress components."
                    type: boolean
                  ingressController:
                    properties:
                      image:
                        default: kong/kubernetes-ingress-controller
                        description: "(Default: \"kong/kubernetes-ingress-controller\")
                          \n Image repo and name to use for kong ingress controller."
                        type: string
                      version:
                        default: 2.5.0
                        description: "(Default: \"2.5.0\") \n Version of kong ingress
                          controller to use."
                        type: string
                    type: object
                  proxyServiceName:
                    default: kong-proxy
                    description: '(Default: "kong-proxy")'
                    type: string
                  replicas:
                    default: 2
                    description: "(Default: 2) \n Number of replicas to use for the
                      kong ingress deployment."
                    type: integer
                type: object
              namespace:
                default: nukleros-ingress-system
                description: "(Default: \"nukleros-ingress-system\") \n Namespace
                  to use for ingress support services."
                type: string
              nginx:
                properties:
                  image:
                    default: nginx/nginx-ingress
                    description: "(Default: \"nginx/nginx-ingress\") \n Image repo
                      and name to use for nginx."
                    type: string
                  include:
                    default: false
                    description: "(Default: false) \n Include the Nginx ingress controller
                      when installing ingress components."
                    type: boolean
                  installType:
                    default: deployment
                    description: "(Default: \"deployment\") \n Method of install nginx
                      ingress controller.  One of: deployment | daemonset."
                    enum:
                    - deployment
                    - daemonset
                    type: string
                  replicas:
                    default: 2
                    description: "(Default: 2) \n Number of replicas to use for the
                      nginx ingress controller deployment."
                    type: integer
                  version:
                    default: 2.3.0
                    description: "(Default: \"2.3.0\") \n Version of nginx to use."
                    type: string
                type: object
            type: object
          status:
            description: IngressComponentStatus defines the observed state of IngressComponent.
            properties:
              conditions:
                items:
                  description: PhaseCondition describes an event that has occurred
                    during a phase of the controller reconciliation loop.
                  properties:
                    lastModified:
                      description: LastModified defines the time in which this component
                        was updated.
                      type: string
                    message:
                      description: Message defines a helpful message from the phase.
                      type: string
                    phase:
                      description: Phase defines the phase in which the condition
                        was set.
                      type: string
                    state:
                      description: PhaseState defines the current state of the phase.
                      enum:
                      - Complete
                      - Reconciling
                      - Failed
                      - Pending
                      type: string
                  required:
                  - lastModified
                  - message
                  - phase
                  - state
                  type: object
                type: array
              created:
                type: boolean
              dependenciesSatisfied:
                type: boolean
              resources:
                items:
                  description: ChildResource is the resource and its condition as
                    stored on the workload custom resource's status field.
                  properties:
                    condition:
                      description: ResourceCondition defines the current condition
                        of this resource.
                      properties:
                        created:
                          description: Created defines whether this object has been
                            successfully created or not.
                          type: boolean
                        lastModified:
                          description: LastModified defines the time in which this
                            resource was updated.
                          type: string
                        message:
                          description: Message defines a helpful message from the
                            resource phase.
                          type: string
                      required:
                      - created
                      type: object
                    group:
                      description: Group defines the API Group of the resource.
                      type: string
                    kind:
                      description: Kind defines the kind of the resource.
                      type: string
                    name:
                      description: Name defines the name of the resource from the
                        metadata.name field.
                      type: string
                    namespace:
                      description: Namespace defines the namespace in which this resource
                        exists in.
                      type: string
                    version:
                      description: Version defines the API Version of the resource.
                      type: string
                  required:
                  - group
                  - kind
                  - name
                  - namespace
                  - version
                  type: object
                type: array
            type: object
        type: object
    served: true
    storage: true
    subresources:
      status: {}
---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  annotations:
    controller-gen.kubebuilder.io/version: v0.9.0
  creationTimestamp: null
  name: secretscomponents.platform.addons.nukleros.io
spec:
  group: platform.addons.nukleros.io
  names:
    kind: SecretsComponent
    listKind: SecretsComponentList
    plural: secretscomponents
    singular: secretscomponent
  scope: Cluster
  versions:
  - name: v1alpha1
    schema:
      openAPIV3Schema:
        description: SecretsComponent is the Schema for the secretscomponents API.
        properties:
          apiVersion:
            description: 'APIVersion defines the versioned schema of this representation
              of an object. Servers should convert recognized schemas to the latest
              internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources'
            type: string
          kind:
            description: 'Kind is a string value representing the REST resource this
              object represents. Servers may infer this from the endpoint the client
              submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds'
            type: string
          metadata:
            type: object
          spec:
            description: SecretsComponentSpec defines the desired state of SecretsComponent.
            properties:
              collection:
                description: Specifies a reference to the collection to use for this
                  workload. Requires the name and namespace input to find the collection.
                  If no collection field is set, default to selecting the only workload
                  collection in the cluster, which will result in an error if not
                  exactly one collection is found.
                properties:
                  name:
                    description: Required if specifying collection.  The name of the
                      collection within a specific collection.namespace to reference.
                    type: string
                  namespace:
                    description: '(Default: "") The namespace where the collection
                      exists.  Required only if the collection is namespace scoped
                      and not cluster scoped.'
                    type: string
                required:
                - name
                type: object
              externalSecrets:
                properties:
                  certController:
                    properties:
                      replicas:
                        default: 1
                        description: "(Default: 1) \n Number of replicas to use for
                          the external-secrets cert-controller deployment."
                        type: integer
                    type: object
                  controller:
                    properties:
                      replicas:
                        default: 2
                        description: "(Default: 2) \n Number of replicas to use for
                          the external-secrets controller deployment."
                        type: integer
                    type: object
                  image:
                    default: ghcr.io/external-secrets/external-secrets
                    description: "(Default: \"ghcr.io/external-secrets/external-secrets\")
                      \n Image repo and name to use for external-secrets."
                    type: string
                  version:
                    default: v0.5.9
                    description: "(Default: \"v0.5.9\") \n Version of external-secrets
                      to use."
                    type: string
                  webhook:
                    properties:
                      replicas:
                        default: 2
                        description: "(Default: 2) \n Number of replicas to use for
                          the external-secrets webhook deployment."
                        type: integer
                    type: object
                type: object
              namespace:
                default: nukleros-secrets-system
                description: "(Default: \"nukleros-secrets-system\") \n Namespace
                  to use for secrets support services."
                type: string
              reloader:
                properties:
                  image:
                    default: stakater/reloader
                    description: "(Default: \"stakater/reloader\") \n Image repo and
                      name to use for reloader."
                    type: string
                  replicas:
                    default: 1
                    description: "(Default: 1) \n Number of replicas to use for the
                      reloader deployment."
                    type: integer
                  version:
                    default: v0.0.119
                    description: "(Default: \"v0.0.119\") \n Version of reloader to
                      use."
                    type: string
                type: object
            type: object
          status:
            description: SecretsComponentStatus defines the observed state of SecretsComponent.
            properties:
              conditions:
                items:
                  description: PhaseCondition describes an event that has occurred
                    during a phase of the controller reconciliation loop.
                  properties:
                    lastModified:
                      description: LastModified defines the time in which this component
                        was updated.
                      type: string
                    message:
                      description: Message defines a helpful message from the phase.
                      type: string
                    phase:
                      description: Phase defines the phase in which the condition
                        was set.
                      type: string
                    state:
                      description: PhaseState defines the current state of the phase.
                      enum:
                      - Complete
                      - Reconciling
                      - Failed
                      - Pending
                      type: string
                  required:
                  - lastModified
                  - message
                  - phase
                  - state
                  type: object
                type: array
              created:
                type: boolean
              dependenciesSatisfied:
                type: boolean
              resources:
                items:
                  description: ChildResource is the resource and its condition as
                    stored on the workload custom resource's status field.
                  properties:
                    condition:
                      description: ResourceCondition defines the current condition
                        of this resource.
                      properties:
                        created:
                          description: Created defines whether this object has been
                            successfully created or not.
                          type: boolean
                        lastModified:
                          description: LastModified defines the time in which this
                            resource was updated.
                          type: string
                        message:
                          description: Message defines a helpful message from the
                            resource phase.
                          type: string
                      required:
                      - created
                      type: object
                    group:
                      description: Group defines the API Group of the resource.
                      type: string
                    kind:
                      description: Kind defines the kind of the resource.
                      type: string
                    name:
                      description: Name defines the name of the resource from the
                        metadata.name field.
                      type: string
                    namespace:
                      description: Namespace defines the namespace in which this resource
                        exists in.
                      type: string
                    version:
                      description: Version defines the API Version of the resource.
                      type: string
                  required:
                  - group
                  - kind
                  - name
                  - namespace
                  - version
                  type: object
                type: array
            type: object
        type: object
    served: true
    storage: true
    subresources:
      status: {}
---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  annotations:
    controller-gen.kubebuilder.io/version: v0.9.0
  creationTimestamp: null
  name: supportservices.setup.addons.nukleros.io
spec:
  group: setup.addons.nukleros.io
  names:
    kind: SupportServices
    listKind: SupportServicesList
    plural: supportservices
    singular: supportservices
  scope: Cluster
  versions:
  - name: v1alpha1
    schema:
      openAPIV3Schema:
        description: SupportServices is the Schema for the supportservices API.
        properties:
          apiVersion:
            description: 'APIVersion defines the versioned schema of this representation
              of an object. Servers should convert recognized schemas to the latest
              internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources'
            type: string
          kind:
            description: 'Kind is a string value representing the REST resource this
              object represents. Servers may infer this from the endpoint the client
              submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds'
            type: string
          metadata:
            type: object
          spec:
            description: SupportServicesSpec defines the desired state of SupportServices.
            properties:
              defaultIngressController:
                default: kong
                description: "(Default: \"kong\") \n The default ingress for setting
                  TLS certs.  One of: kong | nginx."
                enum:
                - kong
                - nginx
                type: string
              tier:
                default: development
                description: "(Default: \"development\") \n The tier of cluster being
                  used.  One of: development | staging | production."
                enum:
                - development
                - staging
                - production
                type: string
            type: object
          status:
            description: SupportServicesStatus defines the observed state of SupportServices.
            properties:
              conditions:
                items:
                  description: PhaseCondition describes an event that has occurred
                    during a phase of the controller reconciliation loop.
                  properties:
                    lastModified:
                      description: LastModified defines the time in which this component
                        was updated.
                      type: string
                    message:
                      description: Message defines a helpful message from the phase.
                      type: string
                    phase:
                      description: Phase defines the phase in which the condition
                        was set.
                      type: string
                    state:
                      description: PhaseState defines the current state of the phase.
                      enum:
                      - Complete
                      - Reconciling
                      - Failed
                      - Pending
                      type: string
                  required:
                  - lastModified
                  - message
                  - phase
                  - state
                  type: object
                type: array
              created:
                type: boolean
              dependenciesSatisfied:
                type: boolean
              resources:
                items:
                  description: ChildResource is the resource and its condition as
                    stored on the workload custom resource's status field.
                  properties:
                    condition:
                      description: ResourceCondition defines the current condition
                        of this resource.
                      properties:
                        created:
                          description: Created defines whether this object has been
                            successfully created or not.
                          type: boolean
                        lastModified:
                          description: LastModified defines the time in which this
                            resource was updated.
                          type: string
                        message:
                          description: Message defines a helpful message from the
                            resource phase.
                          type: string
                      required:
                      - created
                      type: object
                    group:
                      description: Group defines the API Group of the resource.
                      type: string
                    kind:
                      description: Kind defines the kind of the resource.
                      type: string
                    name:
                      description: Name defines the name of the resource from the
                        metadata.name field.
                      type: string
                    namespace:
                      description: Namespace defines the namespace in which this resource
                        exists in.
                      type: string
                    version:
                      description: Version defines the API Version of the resource.
                      type: string
                  required:
                  - group
                  - kind
                  - name
                  - namespace
                  - version
                  type: object
                type: array
            type: object
        type: object
    served: true
    storage: true
    subresources:
      status: {}
`)
}

func SupportServicesComponentsManifest(iamDNSRoleARN, rootDomain, adminEmail string) string {
	return fmt.Sprintf(`---
apiVersion: setup.addons.nukleros.io/v1alpha1
kind: SupportServices
metadata:
  name: threeport-support-services
spec:
  tier: "development"
---
apiVersion: platform.addons.nukleros.io/v1alpha1
kind: CertificatesComponent
metadata:
  name: threeport-control-plane-certs
spec:
  namespace: "threeport-certs"
  certManager:
    cainjector:
      replicas: 1
      image: "quay.io/jetstack/cert-manager-cainjector"
    version: "v1.9.1"
    controller:
      replicas: 1
      image: "quay.io/jetstack/cert-manager-controller"
    webhook:
      replicas: 1
      image: "quay.io/jetstack/cert-manager-webhook"
    contactEmail: %[7]s
---
apiVersion: platform.addons.nukleros.io/v1alpha1
kind: IngressComponent
metadata:
  name: %[1]s
spec:
  nginx:
    include: false
    installType: "deployment"
    image: "nginx/nginx-ingress"
    version: "2.3.0"
    replicas: 2
  namespace: %[2]s
  externalDNS:
    provider: route53
    image: "k8s.gcr.io/external-dns/external-dns"
    version: "v0.12.2"
    serviceAccountName: %[3]s
    iamRoleArn: %[4]s
    zoneType: public
  domainName: %[5]s
  kong:
    include: true
    replicas: 1
    gateway:
      image: "kong/kong-gateway"
      version: "2.8"
    ingressController:
      image: "kong/kubernetes-ingress-controller"
      version: "2.5.0"
    proxyServiceName: %[6]s
`, SupportServicesIngressComponentName, SupportServicesIngressNamespace,
		SupportServicesDNSManagementServiceAccount, iamDNSRoleARN, rootDomain,
		SupportServicesIngressServiceName, adminEmail)
}
