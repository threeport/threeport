package gateway

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-logr/logr"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	tpaws "github.com/threeport/threeport/pkg/aws/v0"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
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
	return createComponentWorkload(
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
		externalDnsYaml, err = getExternalDnsYaml(
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
		externalDnsYaml, err = getExternalDnsYaml(
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
		externalDnsYaml, err = getExternalDnsYaml(
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
	// read the workload the slot names
	workload, err := client.GetKubernetesWorkloadInstanceByID(
		a.reconciler.APIClient,
		a.reconciler.APIServer,
		id,
	)
	if errors.Is(err, client_lib.ErrObjectNotFound) {
		return false, true, nil
	}
	if err != nil {
		return false, false, err
	}
	if workload.DeletionScheduled != nil {
		return false, false, fmt.Errorf("dns component workload %d is being deleted", id)
	}
	if workload.Reconciled != nil && *workload.Reconciled {
		return true, false, nil
	}
	return false, false, nil
}

// SetCallerKubernetesWorkloadInstanceID records nothing because the domain stores no component id.
func (a *domainNameComponentAttacher) SetCallerKubernetesWorkloadInstanceID(id *uint) error {
	return nil
}

// ClaimRuntimeKubernetesWorkloadInstanceID writes the runtime slot.
func (a *domainNameComponentAttacher) ClaimRuntimeKubernetesWorkloadInstanceID(runtimeID uint, workloadID uint) error {
	// load the runtime and set the slot
	runtime, err := client.GetKubernetesRuntimeInstanceByID(
		a.reconciler.APIClient,
		a.reconciler.APIServer,
		runtimeID,
	)
	if err != nil {
		return fmt.Errorf("failed to get kubernetes runtime instance %d: %w", runtimeID, err)
	}
	runtime.IngressComponentInstanceID = util.Ptr(workloadID)
	_, err = client.UpdateKubernetesRuntimeInstance(
		a.reconciler.APIClient,
		a.reconciler.APIServer,
		runtime,
	)
	if err != nil {
		return fmt.Errorf(
			"failed to claim dns component slot on runtime %d: %w",
			runtimeID,
			err,
		)
	}
	return nil
}

// ClearRuntimeKubernetesWorkloadInstanceID clears the runtime slot when no caller requires the workload.
func (a *domainNameComponentAttacher) ClearRuntimeKubernetesWorkloadInstanceID(runtimeID uint, workloadID uint) (bool, error) {
	// load the runtime
	runtime, err := client.GetKubernetesRuntimeInstanceByID(
		a.reconciler.APIClient,
		a.reconciler.APIServer,
		runtimeID,
	)
	if err != nil {
		return false, fmt.Errorf(
			"failed to get kubernetes runtime instance %d: %w",
			runtimeID,
			err,
		)
	}
	if runtime.IngressComponentInstanceID == nil ||
		*runtime.IngressComponentInstanceID != workloadID {
		return false, nil
	}

	// replace the slot with null
	body, err := dnsComponentSlotNull(runtime)
	if err != nil {
		return false, err
	}
	_, err = client_lib.GetResponse(
		a.reconciler.APIClient,
		fmt.Sprintf("%s%s/%d", a.reconciler.APIServer, v0.PathKubernetesRuntimeInstances, runtimeID),
		http.MethodPut,
		bytes.NewBuffer(body),
		map[string]string{},
		http.StatusOK,
	)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, client_lib.ErrBadRequest) &&
		strings.Contains(err.Error(), v0.ErrMsgSupportServiceSlotHeld) {
		return false, nil
	}
	return false, fmt.Errorf(
		"failed to clear dns component slot on runtime %d: %w",
		runtimeID,
		err,
	)
}

// DeleteComponentKubernetesWorkloadInstance removes the workload instance.
func (a *domainNameComponentAttacher) DeleteComponentKubernetesWorkloadInstance(id uint) error {
	// treat a missing workload as already removed
	_, err := client.DeleteKubernetesWorkloadInstance(
		a.reconciler.APIClient,
		a.reconciler.APIServer,
		id,
	)
	if errors.Is(err, client_lib.ErrObjectNotFound) {
		return nil
	}
	return err
}

// dnsComponentSlotNull encodes the runtime with the dns slot set to null.
func dnsComponentSlotNull(runtime *v0.KubernetesRuntimeInstance) ([]byte, error) {
	// encode the runtime and set the slot to null
	encoded, err := json.Marshal(runtime)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal kubernetes runtime instance: %w", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(encoded, &fields); err != nil {
		return nil, fmt.Errorf("failed to unmarshal kubernetes runtime instance: %w", err)
	}
	delete(fields, "ID")
	delete(fields, "CreatedAt")
	delete(fields, "UpdatedAt")
	fields["IngressComponentInstanceID"] = nil
	body, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal kubernetes runtime instance: %w", err)
	}
	return body, nil
}
