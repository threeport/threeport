package gateway

import (
	"fmt"
	"strconv"

	"github.com/go-logr/logr"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	tpaws "github.com/threeport/threeport/pkg/aws/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	"github.com/threeport/threeport/pkg/supportservices"
)

// domainNameComponentAttacher is the caller for one domain name instance.
// It uses the shared ingress workload and stores no component id.
type domainNameComponentAttacher struct {
	reconciler *controller.Reconciler
	domain     *v0.DomainNameInstance
}

// GetReconciler returns the reconciler that reaches the API.
func (a *domainNameComponentAttacher) GetReconciler() *controller.Reconciler {
	return a.reconciler
}

// Install creates the shared ingress workload for external-dns.
func (a *domainNameComponentAttacher) Install(
	runtime *v0.KubernetesRuntimeInstance,
	log *logr.Logger,
) (uint, error) {
	// build the external-dns manifest
	manifest, err := a.externalDnsManifest(runtime)
	if err != nil {
		return 0, err
	}

	// create the shared ingress workload
	return supportservices.CreateComponentWorkload(
		a.reconciler,
		runtime,
		fmt.Sprintf("ingress-%s", *runtime.Name),
		manifest,
		log,
	)
}

// externalDnsManifest returns the external-dns object for this runtime.
func (a *domainNameComponentAttacher) externalDnsManifest(
	runtime *v0.KubernetesRuntimeInstance,
) (string, error) {
	// read the provider and the domain
	infraProvider, err := client.GetInfraProviderByKubernetesRuntimeInstanceID(
		a.reconciler.APIClient,
		a.reconciler.APIServer,
		runtime.ID,
	)
	if err != nil {
		return "", fmt.Errorf("failed to get infra provider: %w", err)
	}
	domainNameDefinition, err := client.GetDomainNameDefinitionByID(
		a.reconciler.APIClient,
		a.reconciler.APIServer,
		*a.domain.DomainNameDefinitionID,
	)
	if err != nil {
		return "", fmt.Errorf("failed to get domain name definition: %w", err)
	}

	// build the external-dns manifest for the provider
	var externalDnsYaml string
	runtimeID := strconv.Itoa(int(*runtime.ID))
	switch *infraProvider {
	case v0.KubernetesRuntimeInfraProviderEKS:
		clusterIdentity, err := client.GetAwsEksClusterIdentityByK8sRuntimeInst(
			a.reconciler.APIClient,
			a.reconciler.APIServer,
			runtime.ID,
		)
		if err != nil {
			return "", fmt.Errorf("failed to get cluster identity: %w", err)
		}
		externalDnsYaml, err = supportservices.ExternalDnsYaml(
			*domainNameDefinition.Domain,
			"route53",
			tpaws.EksDnsManagementRoleArn(
				clusterIdentity.AccountId,
				clusterIdentity.ClusterName,
			),
			"",
			runtimeID,
		)
		if err != nil {
			return "", fmt.Errorf("failed to get external dns yaml: %w", err)
		}
	case v0.KubernetesRuntimeInfraProviderGKE:
		gkeRuntime, err := client.GetGcpGkeKubernetesRuntimeInstanceByK8sRuntimeInst(
			a.reconciler.APIClient,
			a.reconciler.APIServer,
			*runtime.ID,
		)
		if err != nil {
			return "", fmt.Errorf("failed to get gke runtime: %w", err)
		}
		gcpProvider, err := client.GetGcpProviderByID(
			a.reconciler.APIClient,
			a.reconciler.APIServer,
			*gkeRuntime.GcpProviderID,
		)
		if err != nil {
			return "", fmt.Errorf("failed to get gcp provider: %w", err)
		}
		externalDnsYaml, err = supportservices.ExternalDnsYaml(
			*domainNameDefinition.Domain,
			"google",
			"",
			*gcpProvider.ProjectID,
			runtimeID,
		)
		if err != nil {
			return "", fmt.Errorf("failed to get external dns yaml: %w", err)
		}
	case v0.KubernetesRuntimeInfraProviderKind:
		externalDnsYaml, err = supportservices.ExternalDnsYaml(
			*domainNameDefinition.Domain,
			"none",
			"",
			"",
			runtimeID,
		)
		if err != nil {
			return "", fmt.Errorf("failed to get external dns yaml: %w", err)
		}
	default:
		break
	}

	return externalDnsYaml, nil
}

// IsComponentReconciled reports whether the workload has reconciled.
// A missing row returns reconciled false and missing true.
func (a *domainNameComponentAttacher) IsComponentReconciled(id uint) (bool, bool, error) {
	return supportservices.ComponentReconciled(a.reconciler, id)
}

// SetCallerKubernetesWorkloadInstanceID records nothing because the domain stores no component id.
func (a *domainNameComponentAttacher) SetCallerKubernetesWorkloadInstanceID(id *uint) error {
	return nil
}

// ClaimRuntimeKubernetesWorkloadInstanceID writes the runtime slot.
func (a *domainNameComponentAttacher) ClaimRuntimeKubernetesWorkloadInstanceID(runtimeID uint, workloadID uint) error {
	return supportservices.ClaimComponentWorkload(
		a.reconciler,
		runtimeID,
		workloadID,
		supportservices.IngressColumn,
	)
}

// ClearRuntimeKubernetesWorkloadInstanceID clears the runtime slot when no caller requires the workload.
func (a *domainNameComponentAttacher) ClearRuntimeKubernetesWorkloadInstanceID(runtimeID uint, workloadID uint) (bool, error) {
	return supportservices.ClearComponentWorkload(
		a.reconciler,
		runtimeID,
		workloadID,
		supportservices.IngressColumn,
	)
}

// DeleteComponentKubernetesWorkloadInstance removes the workload instance.
func (a *domainNameComponentAttacher) DeleteComponentKubernetesWorkloadInstance(id uint) error {
	return supportservices.DeleteComponentWorkload(a.reconciler, id)
}
