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

// instanceResourceKey identifies a resource within one instance: by group,
// kind and name. An instance places every namespaced resource of its definition
// in the same namespace, so the namespace tells nothing apart there - and
// leaving it out lets resources be matched before namespaces are set, which
// needs no word from the cluster.
func instanceResourceKey(jsonDefinition []byte) (key string, isResource bool, err error) {
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
		"%s/%s/%s",
		resource.GroupVersionKind().Group,
		resource.GetKind(),
		resource.GetName(),
	), true, nil
}

// resourceInstancePlan is the set of writes that brings one workload
// instance's resource instances in line with its definition's new resources.
type resourceInstancePlan struct {
	create []v0.KubernetesWorkloadResourceInstance
	update []v0.KubernetesWorkloadResourceInstance
	remove []v0.KubernetesWorkloadResourceInstance

	// leaving names resources the definition declares again while the
	// instance still holds a resource instance scheduled for deletion for
	// them. Nothing is written for the instance until those are gone: a second
	// resource instance for the same object would be applied and deleted in
	// the same pass, in whichever order they are listed.
	leaving []string
}

// changed reports whether the instance needs any write at all.
func (p *resourceInstancePlan) changed() bool {
	return len(p.create) > 0 || len(p.update) > 0 || len(p.remove) > 0
}

// planResourceInstances compares an instance's resource instances with what
// its definition renders for it before and after a change to the definition.
//
// before and after are the definition's resources with this instance's
// overlay applied, and afterPlaced is after with namespaces set, as the
// instance deploys it. Comparing before with after tells which resources the
// change touched without asking the cluster about the old ones, whose API
// version it may no longer serve. Only those resources are written:
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
	afterPlaced []v0.KubernetesWorkloadResourceInstance,
) (*resourceInstancePlan, error) {
	beforeByKey, err := resourceInstancesByKey(before)
	if err != nil {
		return nil, fmt.Errorf("failed to identify previously rendered resources: %w", err)
	}
	afterByKey, err := resourceInstancesByKey(after)
	if err != nil {
		return nil, fmt.Errorf("failed to identify rendered resources: %w", err)
	}

	// a resource instance already scheduled for deletion is on its way out:
	// it is neither updated nor scheduled again
	var live, leaving []v0.KubernetesWorkloadResourceInstance
	for _, resourceInstance := range existing {
		if resourceInstance.ScheduledForDeletion == nil {
			live = append(live, resourceInstance)
		} else {
			leaving = append(leaving, resourceInstance)
		}
	}
	existingByKey, err := resourceInstancesByKey(live)
	if err != nil {
		return nil, fmt.Errorf("failed to identify existing resource instances: %w", err)
	}
	leavingByKey, err := resourceInstancesByKey(leaving)
	if err != nil {
		return nil, fmt.Errorf("failed to identify resource instances scheduled for deletion: %w", err)
	}

	plan := &resourceInstancePlan{}
	for _, placed := range afterPlaced {
		key, isResource, err := instanceResourceKey(*placed.JSONDefinition)
		if err != nil {
			return nil, fmt.Errorf("failed to identify rendered resource: %w", err)
		}
		if !isResource {
			continue
		}

		current, found := existingByKey[key]
		if !found {
			if _, stillLeaving := leavingByKey[key]; stillLeaving {
				plan.leaving = append(plan.leaving, key)
				continue
			}
			plan.create = append(plan.create, v0.KubernetesWorkloadResourceInstance{
				JSONDefinition:               placed.JSONDefinition,
				KubernetesWorkloadInstanceID: &instanceID,
				Reconciled:                   util.Ptr(false),
			})
			continue
		}

		// the namespace threeport adds is no resource of the definition's,
		// so only what the instance holds tells whether it needs a write
		previous, renderedBefore := beforeByKey[key]
		rendered, renderedAfter := afterByKey[key]
		if renderedBefore && renderedAfter && samePlacedRendering(*previous.JSONDefinition, *rendered.JSONDefinition) {
			continue
		}
		if sameJSON(*current.JSONDefinition, *placed.JSONDefinition) {
			continue
		}
		plan.update = append(plan.update, v0.KubernetesWorkloadResourceInstance{
			Common:         v0.Common{ID: current.ID},
			JSONDefinition: placed.JSONDefinition,
			Reconciled:     util.Ptr(false),
		})
	}

	removedAt := time.Now().UTC()
	for key, previous := range beforeByKey {
		if _, declared := afterByKey[key]; declared {
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

// samePlacedRendering reports whether two renderings of a resource deploy the
// same thing once an instance places them.
//
// The namespace a manifest names never takes effect in an instance: placing a
// namespaced resource overwrites it with the instance's namespace, and the
// Kubernetes API drops it from a cluster-scoped one. So an edit to it alone is
// no change, and must not count as one - that would replace the resource with
// the definition's rendering, undoing edits another controller made to it.
func samePlacedRendering(left, right []byte) bool {
	var leftValue, rightValue map[string]interface{}
	if err := json.Unmarshal(left, &leftValue); err != nil {
		return false
	}
	if err := json.Unmarshal(right, &rightValue); err != nil {
		return false
	}
	unstructured.RemoveNestedField(leftValue, "metadata", "namespace")
	unstructured.RemoveNestedField(rightValue, "metadata", "namespace")

	return reflect.DeepEqual(leftValue, rightValue)
}

// knownNamespacing answers whether a resource is namespaced from the resource
// instances an instance already holds, which carry the namespace the cluster
// placed them in. ask is consulted only for a resource the instance does not
// hold yet, so a resource moving off an API version the cluster no longer
// serves is placed without asking about the old version.
func knownNamespacing(
	existing []v0.KubernetesWorkloadResourceInstance,
	ask func(jsonDef string) (bool, error),
) (func(jsonDef string) (bool, error), error) {
	held := map[string]bool{}
	for _, resourceInstance := range existing {
		if resourceInstance.JSONDefinition == nil {
			continue
		}
		key, isResource, err := instanceResourceKey(*resourceInstance.JSONDefinition)
		if err != nil {
			return nil, fmt.Errorf("failed to identify existing resource instance: %w", err)
		}
		if !isResource {
			continue
		}
		var object map[string]interface{}
		if err := json.Unmarshal(*resourceInstance.JSONDefinition, &object); err != nil {
			return nil, fmt.Errorf("failed to unmarshal resource json: %w", err)
		}
		if _, found := held[key]; !found {
			held[key] = (&unstructured.Unstructured{Object: object}).GetNamespace() != ""
		}
	}

	return func(jsonDef string) (bool, error) {
		key, isResource, err := instanceResourceKey([]byte(jsonDef))
		if err != nil {
			return false, err
		}
		if namespaced, found := held[key]; isResource && found {
			return namespaced, nil
		}
		return ask(jsonDef)
	}, nil
}

// resourceInstancesByKey indexes resource instances by instanceResourceKey.
// When two share a key the first is kept: that is the one created first, which
// the Kubernetes object belongs to.
func resourceInstancesByKey(
	resourceInstances []v0.KubernetesWorkloadResourceInstance,
) (map[string]v0.KubernetesWorkloadResourceInstance, error) {
	byKey := map[string]v0.KubernetesWorkloadResourceInstance{}
	for _, resourceInstance := range resourceInstances {
		if resourceInstance.JSONDefinition == nil {
			continue
		}
		key, isResource, err := instanceResourceKey(*resourceInstance.JSONDefinition)
		if err != nil {
			return nil, err
		}
		if _, found := byKey[key]; isResource && !found {
			byKey[key] = resourceInstance
		}
	}

	return byKey, nil
}
