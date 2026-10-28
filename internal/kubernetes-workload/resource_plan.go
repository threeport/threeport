package kubernetesworkload

import (
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"gorm.io/datatypes"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// namespaceKind is the kind a definition uses to manage its own namespace.
const namespaceKind = "Namespace"

// resourceKey identifies a Kubernetes resource the way the Kubernetes API does:
// by group, kind, namespace and name. The version is left out, so a manifest
// moved to a newer API version is the same resource rather than a new one.
//
// A document that is not an object - the null an empty YAML document parses
// to - is not a resource, and is reported as such rather than as an error.
func resourceKey(jsonDefinition []byte) (key string, isResource bool, err error) {
	var object map[string]interface{}
	if err := json.Unmarshal(jsonDefinition, &object); err != nil {
		return "", false, fmt.Errorf("failed to unmarshal resource json: %w", err)
	}
	if object == nil {
		return "", false, nil
	}

	resource := unstructured.Unstructured{Object: object}
	if resource.GetKind() == "" || resource.GetName() == "" {
		return "", false, fmt.Errorf("resource has no kind or no name: %s", jsonDefinition)
	}

	return fmt.Sprintf(
		"%s/%s/%s/%s",
		resource.GroupVersionKind().Group,
		resource.GetKind(),
		resource.GetNamespace(),
		resource.GetName(),
	), true, nil
}

// sameJSON reports whether two JSON documents hold the same content. The API
// stores them as jsonb, which does not keep the key order or spacing they were
// written with, so comparing bytes would see a change in every stored resource.
func sameJSON(left, right []byte) bool {
	var leftValue, rightValue interface{}
	if err := json.Unmarshal(left, &leftValue); err != nil {
		return false
	}
	if err := json.Unmarshal(right, &rightValue); err != nil {
		return false
	}

	return reflect.DeepEqual(leftValue, rightValue)
}

// managedNamespace returns the name of the first Namespace among a set of
// resources, or "" when there is none and threeport manages the namespace.
//
// kube.SetNamespaces makes the same choice for every instance, so this is the
// namespace each instance's namespaced resources are placed in.
func managedNamespace(jsonDefinitions [][]byte) (string, error) {
	for _, jsonDefinition := range jsonDefinitions {
		var object map[string]interface{}
		if err := json.Unmarshal(jsonDefinition, &object); err != nil {
			return "", fmt.Errorf("failed to unmarshal resource json: %w", err)
		}
		resource := unstructured.Unstructured{Object: object}
		if object != nil && resource.GetKind() == namespaceKind {
			return resource.GetName(), nil
		}
	}

	return "", nil
}

// resourceDefinitionPlan is the set of writes that brings a workload
// definition's resource definitions in line with its YAML document.
type resourceDefinitionPlan struct {
	// desired is the full set of resource definitions the YAML document
	// declares, in document order, carrying the ID of the stored resource
	// definition each one replaces.
	desired []v0.KubernetesWorkloadResourceDefinition

	create []v0.KubernetesWorkloadResourceDefinition
	update []v0.KubernetesWorkloadResourceDefinition
	remove []v0.KubernetesWorkloadResourceDefinition
}

// changed reports whether the YAML document differs from the stored resource
// definitions at all.
func (p *resourceDefinitionPlan) changed() bool {
	return len(p.create) > 0 || len(p.update) > 0 || len(p.remove) > 0
}

// planResourceDefinitions compares a workload definition's stored resource
// definitions with the resources its YAML document declares now.
//
// Resources are matched by resourceKey, so a stored resource definition keeps
// its ID when its manifest changes, and is only removed when the YAML no
// longer declares that resource at all.
func planResourceDefinitions(
	definitionID uint,
	existing []v0.KubernetesWorkloadResourceDefinition,
	declared [][]byte,
) (*resourceDefinitionPlan, error) {
	existingByKey := map[string]v0.KubernetesWorkloadResourceDefinition{}
	plan := &resourceDefinitionPlan{}
	for _, resourceDefinition := range existing {
		if resourceDefinition.JSONDefinition == nil {
			plan.remove = append(plan.remove, resourceDefinition)
			continue
		}
		key, isResource, err := resourceKey(*resourceDefinition.JSONDefinition)
		if err != nil {
			return nil, fmt.Errorf("failed to identify stored kubernetes workload resource definition with ID %d: %w", *resourceDefinition.ID, err)
		}
		if _, duplicate := existingByKey[key]; !isResource || duplicate {
			plan.remove = append(plan.remove, resourceDefinition)
			continue
		}
		existingByKey[key] = resourceDefinition
	}

	declaredKeys := map[string]bool{}
	for _, jsonContent := range declared {
		key, isResource, err := resourceKey(jsonContent)
		if err != nil {
			return nil, fmt.Errorf("failed to identify resource in YAML document: %w", err)
		}
		if !isResource {
			continue
		}
		if declaredKeys[key] {
			return nil, fmt.Errorf("YAML document declares %s more than once", key)
		}
		declaredKeys[key] = true

		var jsonDefinition datatypes.JSON
		if err := jsonDefinition.UnmarshalJSON(jsonContent); err != nil {
			return nil, fmt.Errorf("failed to unmarshal json to datatypes.JSON: %w", err)
		}

		stored, found := existingByKey[key]
		switch {
		case !found:
			created := v0.KubernetesWorkloadResourceDefinition{
				JSONDefinition:                 &jsonDefinition,
				KubernetesWorkloadDefinitionID: &definitionID,
			}
			plan.create = append(plan.create, created)
			plan.desired = append(plan.desired, created)
		case !sameJSON(*stored.JSONDefinition, jsonContent):
			updated := v0.KubernetesWorkloadResourceDefinition{
				Common:                         v0.Common{ID: stored.ID},
				JSONDefinition:                 &jsonDefinition,
				KubernetesWorkloadDefinitionID: &definitionID,
			}
			plan.update = append(plan.update, updated)
			plan.desired = append(plan.desired, updated)
		default:
			plan.desired = append(plan.desired, stored)
		}
	}

	for key, stored := range existingByKey {
		if !declaredKeys[key] {
			plan.remove = append(plan.remove, stored)
		}
	}

	return plan, nil
}

// resourceInstancePlan is the set of writes that brings one workload
// instance's resource instances in line with its definition's new resources.
type resourceInstancePlan struct {
	create []v0.KubernetesWorkloadResourceInstance
	update []v0.KubernetesWorkloadResourceInstance
	remove []v0.KubernetesWorkloadResourceInstance
}

// changed reports whether the instance needs any write at all.
func (p *resourceInstancePlan) changed() bool {
	return len(p.create) > 0 || len(p.update) > 0 || len(p.remove) > 0
}

// planResourceInstances compares an instance's resource instances with what
// its definition rendered before and after a change to the definition.
//
// before and after are both rendered for this instance - overlay applied and
// namespaces set - so they compare like for like with what the instance holds.
// Only resources the definition change touched are written:
//
//   - a resource whose rendering did not change is left as the instance holds
//     it, so a change another controller made to it survives (the gateway
//     controller edits resources of the workloads it serves)
//   - a resource rendered differently is replaced by the new rendering
//   - a resource the definition now declares and the instance lacks is created
//   - a resource the definition no longer declares is scheduled for deletion
//
// Resource instances that came from elsewhere - a gateway's or a secret's -
// match nothing the definition renders and are never touched. Neither is a
// Namespace: deleting one deletes everything in it, those resources included,
// so namespaces go when the instance does.
//
// Every write leaves the resource unreconciled, which is what has
// v0KubernetesWorkloadInstanceUpdated apply it to the cluster.
func planResourceInstances(
	instanceID uint,
	existing []v0.KubernetesWorkloadResourceInstance,
	before []v0.KubernetesWorkloadResourceInstance,
	after []v0.KubernetesWorkloadResourceInstance,
) (*resourceInstancePlan, error) {
	beforeByKey, err := resourceInstancesByKey(before)
	if err != nil {
		return nil, fmt.Errorf("failed to identify previously rendered resources: %w", err)
	}

	// a resource instance already scheduled for deletion is on its way out:
	// it is neither updated nor scheduled again, and a resource the definition
	// declares once more is created afresh beside it
	var live []v0.KubernetesWorkloadResourceInstance
	for _, resourceInstance := range existing {
		if resourceInstance.ScheduledForDeletion == nil {
			live = append(live, resourceInstance)
		}
	}
	existingByKey, err := resourceInstancesByKey(live)
	if err != nil {
		return nil, fmt.Errorf("failed to identify existing resource instances: %w", err)
	}

	plan := &resourceInstancePlan{}
	afterKeys := map[string]bool{}
	for _, rendered := range after {
		key, isResource, err := resourceKey(*rendered.JSONDefinition)
		if err != nil {
			return nil, fmt.Errorf("failed to identify rendered resource: %w", err)
		}
		if !isResource {
			continue
		}
		afterKeys[key] = true

		current, found := existingByKey[key]
		if !found {
			plan.create = append(plan.create, v0.KubernetesWorkloadResourceInstance{
				JSONDefinition:               rendered.JSONDefinition,
				KubernetesWorkloadInstanceID: &instanceID,
				Reconciled:                   util.Ptr(false),
			})
			continue
		}

		previous, renderedBefore := beforeByKey[key]
		if renderedBefore && sameJSON(*previous.JSONDefinition, *rendered.JSONDefinition) {
			continue
		}
		if sameJSON(*current.JSONDefinition, *rendered.JSONDefinition) {
			continue
		}
		plan.update = append(plan.update, v0.KubernetesWorkloadResourceInstance{
			Common:         v0.Common{ID: current.ID},
			JSONDefinition: rendered.JSONDefinition,
			Reconciled:     util.Ptr(false),
		})
	}

	removedAt := time.Now().UTC()
	for key, previous := range beforeByKey {
		if afterKeys[key] {
			continue
		}
		current, found := existingByKey[key]
		if !found {
			continue
		}
		var object map[string]interface{}
		if err := json.Unmarshal(*previous.JSONDefinition, &object); err != nil {
			return nil, fmt.Errorf("failed to unmarshal resource json: %w", err)
		}
		if (&unstructured.Unstructured{Object: object}).GetKind() == namespaceKind {
			continue
		}
		plan.remove = append(plan.remove, v0.KubernetesWorkloadResourceInstance{
			Common:               v0.Common{ID: current.ID},
			ScheduledForDeletion: &removedAt,
			Reconciled:           util.Ptr(false),
		})
	}

	return plan, nil
}

// resourceInstancesByKey indexes resource instances by resourceKey. When two
// share a key the first is kept: that is the one created first, which the
// Kubernetes object belongs to.
func resourceInstancesByKey(
	resourceInstances []v0.KubernetesWorkloadResourceInstance,
) (map[string]v0.KubernetesWorkloadResourceInstance, error) {
	byKey := map[string]v0.KubernetesWorkloadResourceInstance{}
	for _, resourceInstance := range resourceInstances {
		if resourceInstance.JSONDefinition == nil {
			continue
		}
		key, isResource, err := resourceKey(*resourceInstance.JSONDefinition)
		if err != nil {
			return nil, err
		}
		if _, found := byKey[key]; isResource && !found {
			byKey[key] = resourceInstance
		}
	}

	return byKey, nil
}
