package controller

type ControllerConfig struct {
	// The name of the controller in kebab case, e.g.
	// kubernetes-runtime-controller
	Name string

	// The name of the controller in kebab case sans "-controler", e.g
	// kubernetes-runtime
	ShortName string

	// The name of the controller in lower case, no spaces, e.g.
	// kubernetesruntime
	PackageName string

	// The name of a NATS Jetstream stream for a controller, e.g.
	// KubernetesRuntimeStreamName
	StreamName string

	// The objects for which reconcilers should be generated.
	ReconciledObjects []ReconciledObject

	// The struct values parsed from the controller's model file.
	// The data model can be interpreted as:
	// map[objectName]map[fieldName]map[tagKey]tagValue
	// An example of this data model with a WorkloadDefinition is:
	// map["WorkloadDefinition"]map["YAMLDocument"]map["validate"]"required"
	StructTags map[string]map[string]map[string]string
}

// ReconciledObject is a struct that contains the name and version of a
// reconciled object.
type ReconciledObject struct {
	Name                           string
	Version                        string
	DisableNotificationPersistence bool
}

// CheckStructTagMap checks if a struct tag map contains a specific value.
func (cc *ControllerConfig) CheckStructTagMap(
	object,
	field,
	tagKey,
	expectedTagValue string,
) bool {
	if fieldTagMap, objectKeyFound := cc.StructTags[object]; objectKeyFound {
		if tagValueMap, fieldKeyFound := fieldTagMap[field]; fieldKeyFound {
			if tagValue, tagKeyFound := tagValueMap[tagKey]; tagKeyFound {
				if tagValue == expectedTagValue {
					return true
				}
			}
		}
	}
	return false
}
