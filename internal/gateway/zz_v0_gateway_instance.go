package gateway

import (
	"errors"
	"fmt"
	"slices"

	"github.com/go-logr/logr"
	"github.com/iancoleman/strcase"
	"gorm.io/datatypes"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	runtime "github.com/threeport/threeport/internal/kubernetes-runtime"
	workload_util "github.com/threeport/threeport/internal/kubernetes-workload/util"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	mapping "github.com/threeport/threeport/pkg/mapping/v0"
	supportservices "github.com/threeport/threeport/pkg/supportservices"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// changedGatewayResourceInstances returns the stored resource instances that
// need writing back, ready to write: the ones whose configured definition
// differs from what is stored, carrying the new definition and marked
// unreconciled so the kubernetes workload controller re-applies them.
//
// Resources whose definition already matches are left out. Writing one of those
// back would mark it unreconciled and re-apply an identical manifest to the
// cluster for nothing.
//
// metadata.namespace is set aside when comparing. Nothing configured here sets
// it: the kubernetes workload reconciler writes a namespace into every
// namespaced resource and persists that, so the stored copy always carries one
// and the configured copy never does. Comparing it would find a difference on
// every invocation and leave this function doing exactly what it did before.
// The namespace is that reconciler's to decide, not this one's.
func changedGatewayResourceInstances(
	gatewayInstanceObjects []string,
	existingWorkloadResourceInstances *[]v0.KubernetesWorkloadResourceInstance,
	updatedWorkloadResourceInstances *[]v0.KubernetesWorkloadResourceInstance,
) ([]v0.KubernetesWorkloadResourceInstance, error) {
	var changed []v0.KubernetesWorkloadResourceInstance

	// match each configured object to the stored object of the same kind and name
	for _, updated := range *updatedWorkloadResourceInstances {
		kind, name, err := resourceKindAndName(updated.JSONDefinition)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(gatewayInstanceObjects, kind) {
			continue
		}

		existing, err := workload_util.GetUniqueKubernetesWorkloadResourceInstanceByName(existingWorkloadResourceInstances, kind, name)
		if err != nil {
			return nil, fmt.Errorf("failed to get kubernetes workload resource instance: %w", err)
		}

		unchanged, err := util.JSONDefinitionsEqualIgnoring(
			existing.JSONDefinition,
			updated.JSONDefinition,
			[]string{"metadata", "namespace"},
		)
		if err != nil {
			return nil, fmt.Errorf("failed to compare kubernetes workload resource instance definitions: %w", err)
		}
		if unchanged {
			continue
		}

		existing.Reconciled = util.Ptr(false)
		existing.JSONDefinition = updated.JSONDefinition
		changed = append(changed, *existing)
	}

	return changed, nil
}

// resourceKindAndName returns the kind and the name on a resource definition.
func resourceKindAndName(definition *datatypes.JSON) (string, string, error) {
	// read the kind and the name
	if definition == nil {
		return "", "", errors.New("kubernetes workload resource instance definition must be set")
	}
	object, err := util.UnmarshalJSON(*definition)
	if err != nil {
		return "", "", fmt.Errorf("failed to unmarshal json: %w", err)
	}
	kind, kindFound, err := unstructured.NestedString(object, "kind")
	if err != nil {
		return "", "", fmt.Errorf("failed to get kind: %w", err)
	}
	if !kindFound {
		return "", "", errors.New("kind not found")
	}
	name, nameFound, err := unstructured.NestedString(object, "metadata", "name")
	if err != nil {
		return "", "", fmt.Errorf("failed to get name: %w", err)
	}
	if !nameFound {
		return "", "", errors.New("name not found")
	}
	return kind, name, nil
}

// gatewayResourcesToDelete returns stored resources whose kind and name match a rendered object.
func gatewayResourcesToDelete(
	stored *[]v0.KubernetesWorkloadResourceInstance,
	rendered *[]v0.KubernetesWorkloadResourceInstance,
) ([]v0.KubernetesWorkloadResourceInstance, error) {
	// index the kind and name of each rendered object
	owned := map[string]struct{}{}
	if rendered != nil {
		for _, resource := range *rendered {
			kind, name, err := resourceKindAndName(resource.JSONDefinition)
			if err != nil {
				return nil, err
			}
			owned[kind+"\n"+name] = struct{}{}
		}
	}

	// keep stored resources that match one rendered object
	matches := []v0.KubernetesWorkloadResourceInstance{}
	if stored == nil {
		return matches, nil
	}
	for _, resource := range *stored {
		kind, name, err := resourceKindAndName(resource.JSONDefinition)
		if err != nil {
			return nil, err
		}
		if _, ok := owned[kind+"\n"+name]; !ok {
			continue
		}
		matches = append(matches, resource)
	}

	return matches, nil
}

// errGatewayServiceMissing means the service the gateway routes to is not in the workload.
var errGatewayServiceMissing = errors.New("gateway service not found")

// serviceLookupErr wraps a missing service so delete can continue without it.
func serviceLookupErr(err error) error {
	// report a missing resource as its own error
	if errors.Is(err, workload_util.ErrKubernetesWorkloadResourceInstanceNotFound) {
		return fmt.Errorf("failed to unmarshal service kubernetes workload resource instance: %w", errGatewayServiceMissing)
	}
	return fmt.Errorf("failed to unmarshal service kubernetes workload resource instance: %w", err)
}

// ownedGatewayResources matches threeport workload resource instances to the gateway definition identities.
// Each instance is a record in the threeport API, not an object in the Kubernetes cluster.
func ownedGatewayResources(
	r *controller.Reconciler,
	gatewayDefinition *v0.GatewayDefinition,
	stored *[]v0.KubernetesWorkloadResourceInstance,
) ([]v0.KubernetesWorkloadResourceInstance, error) {
	if gatewayDefinition.KubernetesWorkloadDefinitionID == nil {
		return nil, errors.New("kubernetes workload definition ID is nil")
	}

	// read the threeport workload resource definitions
	definitions, err := client.GetKubernetesWorkloadResourceDefinitionsByID(
		r.APIClient,
		r.APIServer,
		*gatewayDefinition.KubernetesWorkloadDefinitionID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get gateway workload resource definitions: %w", err)
	}
	if definitions == nil {
		return []v0.KubernetesWorkloadResourceInstance{}, nil
	}

	// copy each definition identity onto a resource instance
	rendered := make([]v0.KubernetesWorkloadResourceInstance, 0, len(*definitions))
	for _, definition := range *definitions {
		rendered = append(rendered, v0.KubernetesWorkloadResourceInstance{
			JSONDefinition: definition.JSONDefinition,
		})
	}
	matches, err := gatewayResourcesToDelete(stored, &rendered)
	if err != nil {
		return nil, err
	}

	// add the certificate and the issuer, whose names are the domain
	matches, err = appendRenamedKind(matches, stored, definitions, "Certificate")
	if err != nil {
		return nil, err
	}
	return appendRenamedKind(matches, stored, definitions, "Issuer")
}

// appendRenamedKind adds one threeport workload resource instance when the definition has that kind.
func appendRenamedKind(
	matches []v0.KubernetesWorkloadResourceInstance,
	stored *[]v0.KubernetesWorkloadResourceInstance,
	definitions *[]v0.KubernetesWorkloadResourceDefinition,
	kind string,
) ([]v0.KubernetesWorkloadResourceInstance, error) {
	// skip a kind the definition does not own
	owned := false
	if definitions != nil {
		for _, definition := range *definitions {
			got, _, err := resourceKindAndName(definition.JSONDefinition)
			if err != nil {
				return nil, err
			}
			if got == kind {
				owned = true
				break
			}
		}
	}
	if !owned || stored == nil {
		return matches, nil
	}

	// keep the one threeport instance of this kind
	var found []v0.KubernetesWorkloadResourceInstance
	for _, resource := range *stored {
		got, _, err := resourceKindAndName(resource.JSONDefinition)
		if err != nil {
			return nil, err
		}
		if got != kind {
			continue
		}
		found = append(found, resource)
	}
	if len(found) > 1 {
		return nil, fmt.Errorf("multiple kubernetes workload resource instances found")
	}
	if len(found) == 0 {
		return matches, nil
	}
	for _, match := range matches {
		if match.ID != nil && found[0].ID != nil && *match.ID == *found[0].ID {
			return matches, nil
		}
	}
	return append(matches, found[0]), nil
}

// getThreeportobjects returns all threeport objects required for
// gateway instance reconciliation
func getThreeportObjects(
	r *controller.Reconciler,
	gatewayInstance *v0.GatewayInstance,
) (*v0.KubernetesRuntimeInstance, *v0.GatewayDefinition, *v0.KubernetesWorkloadInstance, error) {

	// get kubernetes runtime instance
	if gatewayInstance.KubernetesRuntimeInstanceID == nil {
		return nil, nil, nil, errors.New("kubernetes runtime instance ID is nil")
	}
	kubernetesRuntimeInstance, err := client.GetKubernetesRuntimeInstanceByID(r.APIClient, r.APIServer, *gatewayInstance.KubernetesRuntimeInstanceID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get gateway kubernetes runtime instance by ID: %w", err)
	}

	// get gateway definition
	if gatewayInstance.GatewayDefinitionID == nil {
		return nil, nil, nil, fmt.Errorf("gateway definition ID is nil")
	}
	gatewayDefinition, err := client.GetGatewayDefinitionByID(r.APIClient, r.APIServer, *gatewayInstance.GatewayDefinitionID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get gateway component kubernetes workload definition: %w", err)
	}

	// get kubernetes workload instance
	if gatewayInstance.KubernetesWorkloadInstanceID == nil {
		return nil, nil, nil, fmt.Errorf("kubernetes workload instance ID is nil")
	}
	workloadInstance, err := client.GetKubernetesWorkloadInstanceByID(r.APIClient, r.APIServer, *gatewayInstance.KubernetesWorkloadInstanceID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get kubernetes workload instance: %w", err)
	}

	return kubernetesRuntimeInstance, gatewayDefinition, workloadInstance, nil
}

// validateThreeportState validates the state of the threeport API
// prior to reconciling a gateway instance
func validateThreeportState(
	r *controller.Reconciler,
	gatewayInstance *v0.GatewayInstance,
	workloadInstance *v0.KubernetesWorkloadInstance,
	kubernetesRuntimeInstance *v0.KubernetesRuntimeInstance,
	log *logr.Logger,
) error {

	// ensure gateway and kubernetes workload definition are reconciled
	definitionsReconciled, err := confirmDefinitionsReconciled(r, gatewayInstance)
	if err != nil {
		return fmt.Errorf("failed to determine if kubernetes workload definition is reconciled: %w", err)
	}
	if !definitionsReconciled {
		return errors.New("kubernetes workload definition not reconciled")
	}

	// ensure kubernetes workload instance is reconciled
	if gatewayInstance.KubernetesWorkloadInstanceID == nil {
		return fmt.Errorf("failed to determine if kubernetes workload instance is reconciled, kubernetes workload instance ID is nil")
	}
	workloadInstanceReconciled, err := confirmWorkloadInstanceReconciled(r, *gatewayInstance.KubernetesWorkloadInstanceID)
	if err != nil {
		return fmt.Errorf("failed to determine if kubernetes workload instance is reconciled: %w", err)
	}
	if !workloadInstanceReconciled {
		return errors.New("kubernetes workload instance not reconciled")
	}

	// install or adopt the shared component, then record this gateway
	err = supportservices.EnsureIngressComponent(
		r,
		kubernetesRuntimeInstance,
		&gatewayIngressAttacher{reconciler: r, gateway: gatewayInstance},
		log,
	)
	if err != nil {
		return fmt.Errorf("failed to reconcile gateway component: %w", err)
	}

	// accept the stored component id without waiting for its pods
	if err := gatewayComponentPresent(kubernetesRuntimeInstance); err != nil {
		return err
	}

	// install cert-manager on eks and kind
	infraProvider, err := client.GetInfraProviderByKubernetesRuntimeInstanceID(
		r.APIClient,
		r.APIServer,
		kubernetesRuntimeInstance.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to get infra provider: %w", err)
	}
	if infraProvider != nil &&
		(*infraProvider == v0.KubernetesRuntimeInfraProviderEKS ||
			*infraProvider == v0.KubernetesRuntimeInfraProviderKind) {
		err = supportservices.EnsureCertificatesComponent(
			r,
			kubernetesRuntimeInstance,
			&gatewayCertificatesAttacher{
				reconciler: r,
				gateway:    gatewayInstance,
			},
			log,
		)
		if err != nil {
			return fmt.Errorf("failed to reconcile certificates component: %w", err)
		}
	}

	return nil
}

// gatewayComponentPresent reports that the runtime stores a component workload.
// The issuer is an API record and does not wait for that workload to reconcile.
func gatewayComponentPresent(kubernetesRuntimeInstance *v0.KubernetesRuntimeInstance) error {
	// require the stored workload id
	if kubernetesRuntimeInstance.IngressComponentInstanceID == nil {
		return fmt.Errorf("gateway component instance ID is nil")
	}
	return nil
}

// confirmDefinitionsReconciled confirms the gateway definition related to a
// gateway instance is reconciled.
func confirmDefinitionsReconciled(
	r *controller.Reconciler,
	gatewayInstance *v0.GatewayInstance,
) (bool, error) {

	// get gateway definition
	if gatewayInstance.GatewayDefinitionID == nil {
		return false, fmt.Errorf("failed to get gateway definition from gateway instance, gateway definition ID is nil")
	}
	gatewayDefinition, err := client.GetGatewayDefinitionByID(r.APIClient, r.APIServer, *gatewayInstance.GatewayDefinitionID)
	if err != nil {
		return false, fmt.Errorf("failed to get gateway definition by kubernetes workload definition ID: %w", err)
	}

	// if the gateway definition is not reconciled, return false
	if gatewayDefinition.Reconciled != nil && !*gatewayDefinition.Reconciled {
		return false, nil
	}

	// get kubernetes workload definition
	if gatewayDefinition.KubernetesWorkloadDefinitionID == nil {
		return false, fmt.Errorf("failed to get kubernetes workload definition from gateway definition, kubernetes workload definition ID is nil")
	}
	workloadDefinition, err := client.GetKubernetesWorkloadDefinitionByID(r.APIClient, r.APIServer, *gatewayDefinition.KubernetesWorkloadDefinitionID)
	if err != nil {
		return false, fmt.Errorf("failed to get kubernetes workload definition by kubernetes workload definition ID: %w", err)
	}

	// if the kubernetes workload definition is not reconciled, return false
	if workloadDefinition.Reconciled != nil && !*workloadDefinition.Reconciled {
		return false, nil
	}

	return true, nil
}

// releaseGatewayComponent deletes the shared component when no gateway remains.
func releaseGatewayComponent(r *controller.Reconciler, gatewayInstance *v0.GatewayInstance) error {
	// skip a gateway that names no runtime
	if gatewayInstance.KubernetesRuntimeInstanceID == nil {
		return nil
	}

	// load the runtime
	kubernetesRuntimeInstance, err := client.GetKubernetesRuntimeInstanceByID(
		r.APIClient,
		r.APIServer,
		*gatewayInstance.KubernetesRuntimeInstanceID,
	)
	if err != nil {
		return fmt.Errorf("failed to get kubernetes runtime instance: %w", err)
	}

	// skip a runtime that has no component workload
	if kubernetesRuntimeInstance.IngressComponentInstanceID == nil &&
		kubernetesRuntimeInstance.CertificatesComponentInstanceID == nil {
		return nil
	}

	// keep the components while any gateway is still live
	instances, err := client.GetGatewayInstancesByQueryString(
		r.APIClient,
		r.APIServer,
		fmt.Sprintf("kubernetesruntimeinstanceid=%d", *kubernetesRuntimeInstance.ID),
	)
	if err != nil {
		return fmt.Errorf("failed to get gateway instances for runtime: %w", err)
	}
	for _, instance := range *instances {
		if instance.DeletionScheduled == nil {
			return nil
		}
	}

	// clear the slot, then delete the workload when no caller still requires it
	err = supportservices.ReleaseIngressComponent(
		kubernetesRuntimeInstance,
		&gatewayIngressAttacher{reconciler: r, gateway: gatewayInstance},
	)
	if err != nil {
		return fmt.Errorf("failed to release ingress component: %w", err)
	}
	return supportservices.ReleaseCertificatesComponent(
		kubernetesRuntimeInstance,
		&gatewayCertificatesAttacher{
			reconciler: r,
			gateway:    gatewayInstance,
		},
	)
}

// configureGatewayManifests renders the ingress and transport objects for a gateway instance.
func configureGatewayManifests(
	r *controller.Reconciler,
	gatewayDefinition *v0.GatewayDefinition,
	workloadInstance *v0.KubernetesWorkloadInstance,
	kubernetesRuntimeInstance *v0.KubernetesRuntimeInstance,
) ([]*datatypes.JSON, error) {

	// get kubernetes workload resource instances
	workloadResourceInstances, err := client.GetKubernetesWorkloadResourceInstancesByID(r.APIClient, r.APIServer, *workloadInstance.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get kubernetes workload resource instances: %w", err)
	}

	// if a service name is provided, use it to get the service
	// otherwise, get the first service
	var service map[string]interface{}
	if gatewayDefinition.ServiceName != nil && *gatewayDefinition.ServiceName != "" {
		service, err = workload_util.UnmarshalKubernetesWorkloadResourceInstance(workloadResourceInstances, "Service", *gatewayDefinition.ServiceName)
		if err != nil {
			return nil, serviceLookupErr(err)
		}
	} else {
		service, err = workload_util.UnmarshalUniqueKubernetesWorkloadResourceInstance(workloadResourceInstances, "Service")
		if err != nil {
			return nil, serviceLookupErr(err)
		}
	}

	// unmarshal service namespace
	namespace, found, err := unstructured.NestedString(service, "metadata", "namespace")
	if err != nil || !found {
		return nil, fmt.Errorf("failed to unmarshal kubernetes service object's namespace field: %w", err)
	}

	// unmarshal service name
	name, found, err := unstructured.NestedString(service, "metadata", "name")
	if err != nil || !found {
		return nil, fmt.Errorf("failed to unmarshal kubernetes service object's name field: %w", err)
	}

	// get gateway kubernetes workload resource definitions
	gatewayWorkloadResourceDefinitions, err := client.GetKubernetesWorkloadResourceDefinitionsByID(r.APIClient, r.APIServer, *gatewayDefinition.KubernetesWorkloadDefinitionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get gateway kubernetes workload resource definitions: %w", err)
	}

	// set the backend on each ingress
	var jsonDefinitions []*datatypes.JSON
	ingressManifests, err := configureVirtualServiceRuntimeParameters(
		r,
		gatewayDefinition,
		workloadInstance,
		kubernetesRuntimeInstance,
		gatewayWorkloadResourceDefinitions,
		namespace,
		name,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to configure ingresses: %w", err)
	}
	jsonDefinitions = append(jsonDefinitions, ingressManifests...)

	// configure tcp gateway runtime parameters
	tcpGatewayManifests, err := configureTcpGatewayRuntimeParameters(
		r,
		gatewayDefinition,
		workloadInstance,
		kubernetesRuntimeInstance,
		gatewayWorkloadResourceDefinitions,
		namespace,
		name,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to configure transport servers: %w", err)
	}
	jsonDefinitions = append(jsonDefinitions, tcpGatewayManifests...)

	return jsonDefinitions, nil

}

// configureVirtualServiceRuntimeParameters sets the backend on each Ingress.
func configureVirtualServiceRuntimeParameters(
	r *controller.Reconciler,
	gatewayDefinition *v0.GatewayDefinition,
	workloadInstance *v0.KubernetesWorkloadInstance,
	kubernetesRuntimeInstance *v0.KubernetesRuntimeInstance,
	gatewayWorkloadResourceDefinitions *[]v0.KubernetesWorkloadResourceDefinition,
	namespace,
	name string,
) ([]*datatypes.JSON, error) {

	// get domain info
	domain, _, err := getDomainInfo(r, gatewayDefinition)
	if err != nil {
		return nil, fmt.Errorf("failed to get domain info: %w", err)
	}

	// get gateway http ports
	gatewayHttpPorts, err := client.GetGatewayHttpPortsByGatewayDefinitionId(r.APIClient, r.APIServer, *gatewayDefinition.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get gateway http and tcp ports: %w", err)
	}

	// configure the one ingress for the host
	if len(*gatewayHttpPorts) == 0 {
		return nil, nil
	}
	servicePort := int64(80)
	for _, httpPort := range *gatewayHttpPorts {
		if httpPort.TLSEnabled == nil || !*httpPort.TLSEnabled {
			servicePort = int64(*httpPort.Port)
		}
	}
	ingress, err := workload_util.UnmarshalUniqueKubernetesWorkloadResourceDefinitionByName(
		gatewayWorkloadResourceDefinitions,
		"Ingress",
		getIngressName(gatewayDefinition, domain),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal ingress kubernetes workload resource definition: %w", err)
	}
	if err := setIngressBackend(ingress, name, servicePort); err != nil {
		return nil, err
	}
	virtualServiceMarshaled, err := util.MarshalJSON(ingress)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal ingress: %w", err)
	}
	return []*datatypes.JSON{&virtualServiceMarshaled}, nil
}

// configureTcpGatewayRuntimeParameters configures runtime parameters
// for tcp gateways
func configureTcpGatewayRuntimeParameters(
	r *controller.Reconciler,
	gatewayDefinition *v0.GatewayDefinition,
	workloadInstance *v0.KubernetesWorkloadInstance,
	kubernetesRuntimeInstance *v0.KubernetesRuntimeInstance,
	gatewayWorkloadResourceDefinitions *[]v0.KubernetesWorkloadResourceDefinition,
	namespace,
	name string,
) ([]*datatypes.JSON, error) {

	// get tcp ports
	gatewayTcpPorts, err := client.GetGatewayTcpPortsByGatewayDefinitionId(r.APIClient, r.APIServer, *gatewayDefinition.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get gateway http ports: %w", err)
	}

	// configure transport server runtime parameters
	var tcpGateways []*datatypes.JSON
	for _, tcpPort := range *gatewayTcpPorts {
		server, err := workload_util.UnmarshalUniqueKubernetesWorkloadResourceDefinitionByName(
			gatewayWorkloadResourceDefinitions,
			"TransportServer",
			fmt.Sprintf("%s-%d", *gatewayDefinition.Name, *tcpPort.Port),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal transport server kubernetes workload resource definition: %w", err)
		}
		if err := setTransportBackend(server, name, int64(*tcpPort.Port)); err != nil {
			return nil, err
		}
		tcpGatewayMarshaled, err := util.MarshalJSON(server)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal transport server: %w", err)
		}
		tcpGateways = append(tcpGateways, &tcpGatewayMarshaled)
	}

	return tcpGateways, nil
}

// getSubDomain returns the subdomain for a gateway definition and domain name definition.
func getSubDomain(gatewayDefinition *v0.GatewayDefinition, domainNameDefinition *v0.DomainNameDefinition) string {
	return fmt.Sprintf("%s.%s", *gatewayDefinition.SubDomain, *domainNameDefinition.Domain)
}

// configureIssuer configures an Issuer custom resource.
func configureIssuer(
	r *controller.Reconciler,
	gatewayDefinition *v0.GatewayDefinition,
	workloadInstance *v0.KubernetesWorkloadInstance,
	kubernetesRuntimeInstance *v0.KubernetesRuntimeInstance,
	domainNameDefinition *v0.DomainNameDefinition,
) (*datatypes.JSON, error) {

	// get gateway kubernetes workload resource definitions
	gatewayWorkloadResourceDefinitions, err := client.GetKubernetesWorkloadResourceDefinitionsByID(r.APIClient, r.APIServer, *gatewayDefinition.KubernetesWorkloadDefinitionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get gateway kubernetes workload resource definitions: %w", err)
	}

	// unmarshal virtual service
	issuer, err := workload_util.UnmarshalUniqueKubernetesWorkloadResourceDefinition(gatewayWorkloadResourceDefinitions, "Issuer")
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal virtual service kubernetes workload resource definition: %w", err)
	}

	// set issuer name
	kebabDomain := strcase.ToKebab(*domainNameDefinition.Name)
	err = unstructured.SetNestedField(issuer, kebabDomain, "metadata", "name")
	if err != nil {
		return nil, fmt.Errorf("failed to set name on issuer: %w", err)
	}

	// add domain to list of dns zones
	var dnsZones = []interface{}{
		*domainNameDefinition.Domain,
	}

	// if a subdomain is provided, append it to the list of dns zones
	if gatewayDefinition.SubDomain != nil && *gatewayDefinition.SubDomain != "" {
		dnsZones = append(dnsZones, getSubDomain(gatewayDefinition, domainNameDefinition))
	}

	// get kubernetes runtime definition
	kubernetesRuntimeDefinition, err := client.GetKubernetesRuntimeDefinitionByID(r.APIClient, r.APIServer, *kubernetesRuntimeInstance.KubernetesRuntimeDefinitionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get kubernetes runtime definition by kubernetes runtime definition ID: %w", err)
	}

	// kind has no dns provider, so cert-manager issues a local certificate
	if util.PtrEqual(kubernetesRuntimeDefinition.InfraProvider, v0.KubernetesRuntimeInfraProviderKind) {
		if err := setSelfSignedIssuer(issuer); err != nil {
			return nil, fmt.Errorf("failed to set self-signed issuer: %w", err)
		}
		issuerMarshaled, err := util.MarshalJSON(issuer)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal issuer: %w", err)
		}
		return &issuerMarshaled, nil
	}

	// get infra provider region
	provider, err := runtime.GetCloudProviderForInfraProvider(*kubernetesRuntimeDefinition.InfraProvider)
	if err != nil {
		return nil, fmt.Errorf("failed to get cloud provider for infra provider: %w", err)
	}
	infraProviderRegion, err := mapping.GetProviderRegionForLocation(provider, *kubernetesRuntimeInstance.Location)
	if err != nil {
		return nil, fmt.Errorf("failed to get infra provider region for location: %w", err)
	}

	solver := map[string]interface{}{
		"selector": map[string]interface{}{
			"dnsZones": dnsZones,
		},
		"dns01": map[string]interface{}{
			"route53": map[string]interface{}{
				"region": infraProviderRegion,
			},
		},
	}

	// set solver
	solverList := []interface{}{solver}
	err = unstructured.SetNestedSlice(issuer, solverList, "spec", "acme", "solvers")
	if err != nil {
		return nil, fmt.Errorf("failed to set solvers on issuer: %w", err)
	}

	// marshal into json
	issuerMarshaled, err := util.MarshalJSON(issuer)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal issuer: %w", err)
	}

	return &issuerMarshaled, nil
}

// configureCertificate configures a Certificate custom resource.
func configureCertificate(
	r *controller.Reconciler,
	gatewayDefinition *v0.GatewayDefinition,
	workloadInstance *v0.KubernetesWorkloadInstance,
	kubernetesRuntimeInstance *v0.KubernetesRuntimeInstance,
	domainNameDefinition *v0.DomainNameDefinition,
) (*datatypes.JSON, error) {

	// get gateway kubernetes workload resource definitions
	gatewayWorkloadResourceDefinitions, err := client.GetKubernetesWorkloadResourceDefinitionsByID(r.APIClient, r.APIServer, *gatewayDefinition.KubernetesWorkloadDefinitionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get gateway kubernetes workload resource definitions: %w", err)
	}

	// unmarshal virtual service
	certificate, err := workload_util.UnmarshalUniqueKubernetesWorkloadResourceDefinition(gatewayWorkloadResourceDefinitions, "Certificate")
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal virtual service kubernetes workload resource definition: %w", err)
	}

	// set certificate name
	kebabDomain := strcase.ToKebab(*domainNameDefinition.Name)
	err = unstructured.SetNestedField(certificate, kebabDomain, "metadata", "name")
	if err != nil {
		return nil, fmt.Errorf("failed to set name on issuer: %w", err)
	}

	dnsNames := []interface{}{}
	switch {
	case gatewayDefinition.SubDomain != nil && *gatewayDefinition.SubDomain != "":
		dnsNames = append(dnsNames, getSubDomain(gatewayDefinition, domainNameDefinition))
	case domainNameDefinition.Domain != nil:
		dnsNames = append(dnsNames, *domainNameDefinition.Domain)
	default:
		return nil, fmt.Errorf("failed to configure certificate, domain name definition domain is nil and no subdomain was provided")
	}

	// set dns names
	err = unstructured.SetNestedSlice(certificate, dnsNames, "spec", "dnsNames")
	if err != nil {
		return nil, fmt.Errorf("failed to set dns names on certificate: %w", err)
	}

	// set issuerRef name
	err = unstructured.SetNestedField(certificate, kebabDomain, "spec", "issuerRef", "name")
	if err != nil {
		return nil, fmt.Errorf("failed to set issuerRef on certificate: %w", err)
	}

	// marshal into json
	certificateMarshaled, err := util.MarshalJSON(certificate)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal certificate: %w", err)
	}

	return &certificateMarshaled, nil
}

// getGatewayInstanceObjects returns the objects that should be created.
func getGatewayInstanceObjects(r *controller.Reconciler, gatewayInstance *v0.GatewayInstance) ([]string, error) {

	gatewayDefinition, err := client.GetGatewayDefinitionByID(r.APIClient, r.APIServer, *gatewayInstance.GatewayDefinitionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get gateway definition: %w", err)
	}

	httpPorts, err := client.GetGatewayHttpPortsByGatewayDefinitionId(r.APIClient, r.APIServer, *gatewayDefinition.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get gateway http ports: %w", err)
	}
	tcpPorts, err := client.GetGatewayTcpPortsByGatewayDefinitionId(r.APIClient, r.APIServer, *gatewayDefinition.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get gateway tcp ports: %w", err)
	}

	var objects []string
	if len(*httpPorts) > 0 {
		objects = append(objects, "Ingress")
	}
	if len(*tcpPorts) > 0 {
		objects = append(objects, "TransportServer")
	}

	tlsEnabled, err := getTlsEnabled(r, gatewayDefinition)
	if err != nil {
		return nil, fmt.Errorf("failed to get tls enabled: %w", err)
	}
	if tlsEnabled {
		objects = append(objects, "Issuer", "Certificate")
	}

	return objects, nil
}

// configureGatewayKubernetesWorkloadResourceInstances configures the kubernetes workload resource instances
// required for a gateway instance
func configureGatewayKubernetesWorkloadResourceInstances(
	r *controller.Reconciler,
	gatewayDefinition *v0.GatewayDefinition,
	workloadInstance *v0.KubernetesWorkloadInstance,
	kubernetesRuntimeInstance *v0.KubernetesRuntimeInstance,
) (*[]v0.KubernetesWorkloadResourceInstance, error) {

	var jsonManifests []*datatypes.JSON

	// get ingress and transport server manifests
	gatewayManifests, err := configureGatewayManifests(r, gatewayDefinition, workloadInstance, kubernetesRuntimeInstance)
	if err != nil {
		return nil, fmt.Errorf("failed to configure gateway manifests: %w", err)
	}
	jsonManifests = append(jsonManifests, gatewayManifests...)

	tlsEnabled, err := getTlsEnabled(r, gatewayDefinition)
	if err != nil {
		return nil, fmt.Errorf("failed to get tls enabled: %w", err)
	}
	if tlsEnabled {

		if gatewayDefinition.DomainNameDefinitionID == nil {
			return nil, fmt.Errorf("failed to create certificate, domain name definition ID is nil")
		}

		// get domain name definition
		domainNameDefinition, err := client.GetDomainNameDefinitionByID(r.APIClient, r.APIServer, *gatewayDefinition.DomainNameDefinitionID)
		if err != nil {
			return nil, fmt.Errorf("failed to get domain name definition: %w", err)
		}

		// configure issuerManifest manifest
		issuerManifest, err := configureIssuer(r, gatewayDefinition, workloadInstance, kubernetesRuntimeInstance, domainNameDefinition)
		if err != nil {
			return nil, fmt.Errorf("failed to configure issuer: %w", err)
		}
		jsonManifests = append(jsonManifests, issuerManifest)

		// configure certificateManifest manifest
		certificateManifest, err := configureCertificate(r, gatewayDefinition, workloadInstance, kubernetesRuntimeInstance, domainNameDefinition)
		if err != nil {
			return nil, fmt.Errorf("failed to configure certificate: %w", err)
		}
		jsonManifests = append(jsonManifests, certificateManifest)
	}

	var workloadResourceInstances []v0.KubernetesWorkloadResourceInstance
	for _, jsonManifest := range jsonManifests {
		workloadResourceInstance := v0.KubernetesWorkloadResourceInstance{
			JSONDefinition:               jsonManifest,
			KubernetesWorkloadInstanceID: workloadInstance.ID,
			Reconciled:                   util.Ptr(false),
		}
		workloadResourceInstances = append(workloadResourceInstances, workloadResourceInstance)
	}

	return &workloadResourceInstances, nil
}
