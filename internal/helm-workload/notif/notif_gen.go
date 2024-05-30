// generated by 'threeport-sdk gen' - do not edit

package notif

const (
	HelmWorkloadStreamName = "helm-workloadStream"

	HelmWorkloadDefinitionSubject       = "helmWorkloadDefinition.*"
	HelmWorkloadDefinitionCreateSubject = "helmWorkloadDefinition.create"
	HelmWorkloadDefinitionUpdateSubject = "helmWorkloadDefinition.update"
	HelmWorkloadDefinitionDeleteSubject = "helmWorkloadDefinition.delete"

	HelmWorkloadInstanceSubject       = "helmWorkloadInstance.*"
	HelmWorkloadInstanceCreateSubject = "helmWorkloadInstance.create"
	HelmWorkloadInstanceUpdateSubject = "helmWorkloadInstance.update"
	HelmWorkloadInstanceDeleteSubject = "helmWorkloadInstance.delete"
)

// Get GetHelmWorkloadDefinitionSubjects returns the NATS subjects
// for helm workload definitions.
func GetHelmWorkloadDefinitionSubjects() []string {
	return []string{
		HelmWorkloadDefinitionCreateSubject,
		HelmWorkloadDefinitionUpdateSubject,
		HelmWorkloadDefinitionDeleteSubject,
	}
}

// Get GetHelmWorkloadInstanceSubjects returns the NATS subjects
// for helm workload instances.
func GetHelmWorkloadInstanceSubjects() []string {
	return []string{
		HelmWorkloadInstanceCreateSubject,
		HelmWorkloadInstanceUpdateSubject,
		HelmWorkloadInstanceDeleteSubject,
	}
}

// GetHelmWorkloadSubjects returns the NATS subjects
// for all helm workload objects.
func GetHelmWorkloadSubjects() []string {
	var helmWorkloadSubjects []string

	helmWorkloadSubjects = append(helmWorkloadSubjects, GetHelmWorkloadDefinitionSubjects()...)
	helmWorkloadSubjects = append(helmWorkloadSubjects, GetHelmWorkloadInstanceSubjects()...)

	return helmWorkloadSubjects
}
