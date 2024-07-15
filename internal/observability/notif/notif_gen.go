// generated by 'threeport-sdk gen' - do not edit

package notif

const (
	ObservabilityStreamName = "observabilityStream"

	ObservabilityStackDefinitionSubject       = "observabilityStackDefinition.*"
	ObservabilityStackDefinitionCreateSubject = "observabilityStackDefinition.create"
	ObservabilityStackDefinitionUpdateSubject = "observabilityStackDefinition.update"
	ObservabilityStackDefinitionDeleteSubject = "observabilityStackDefinition.delete"

	ObservabilityStackInstanceSubject       = "observabilityStackInstance.*"
	ObservabilityStackInstanceCreateSubject = "observabilityStackInstance.create"
	ObservabilityStackInstanceUpdateSubject = "observabilityStackInstance.update"
	ObservabilityStackInstanceDeleteSubject = "observabilityStackInstance.delete"

	ObservabilityDashboardDefinitionSubject       = "observabilityDashboardDefinition.*"
	ObservabilityDashboardDefinitionCreateSubject = "observabilityDashboardDefinition.create"
	ObservabilityDashboardDefinitionUpdateSubject = "observabilityDashboardDefinition.update"
	ObservabilityDashboardDefinitionDeleteSubject = "observabilityDashboardDefinition.delete"

	ObservabilityDashboardInstanceSubject       = "observabilityDashboardInstance.*"
	ObservabilityDashboardInstanceCreateSubject = "observabilityDashboardInstance.create"
	ObservabilityDashboardInstanceUpdateSubject = "observabilityDashboardInstance.update"
	ObservabilityDashboardInstanceDeleteSubject = "observabilityDashboardInstance.delete"

	MetricsDefinitionSubject       = "metricsDefinition.*"
	MetricsDefinitionCreateSubject = "metricsDefinition.create"
	MetricsDefinitionUpdateSubject = "metricsDefinition.update"
	MetricsDefinitionDeleteSubject = "metricsDefinition.delete"

	MetricsInstanceSubject       = "metricsInstance.*"
	MetricsInstanceCreateSubject = "metricsInstance.create"
	MetricsInstanceUpdateSubject = "metricsInstance.update"
	MetricsInstanceDeleteSubject = "metricsInstance.delete"

	LoggingDefinitionSubject       = "loggingDefinition.*"
	LoggingDefinitionCreateSubject = "loggingDefinition.create"
	LoggingDefinitionUpdateSubject = "loggingDefinition.update"
	LoggingDefinitionDeleteSubject = "loggingDefinition.delete"

	LoggingInstanceSubject       = "loggingInstance.*"
	LoggingInstanceCreateSubject = "loggingInstance.create"
	LoggingInstanceUpdateSubject = "loggingInstance.update"
	LoggingInstanceDeleteSubject = "loggingInstance.delete"
)

// Get GetObservabilityStackDefinitionSubjects returns the NATS subjects
// for observability stack definitions.
func GetObservabilityStackDefinitionSubjects() []string {
	return []string{
		ObservabilityStackDefinitionCreateSubject,
		ObservabilityStackDefinitionUpdateSubject,
		ObservabilityStackDefinitionDeleteSubject,
	}
}

// Get GetObservabilityStackInstanceSubjects returns the NATS subjects
// for observability stack instances.
func GetObservabilityStackInstanceSubjects() []string {
	return []string{
		ObservabilityStackInstanceCreateSubject,
		ObservabilityStackInstanceUpdateSubject,
		ObservabilityStackInstanceDeleteSubject,
	}
}

// Get GetObservabilityDashboardDefinitionSubjects returns the NATS subjects
// for observability dashboard definitions.
func GetObservabilityDashboardDefinitionSubjects() []string {
	return []string{
		ObservabilityDashboardDefinitionCreateSubject,
		ObservabilityDashboardDefinitionUpdateSubject,
		ObservabilityDashboardDefinitionDeleteSubject,
	}
}

// Get GetObservabilityDashboardInstanceSubjects returns the NATS subjects
// for observability dashboard instances.
func GetObservabilityDashboardInstanceSubjects() []string {
	return []string{
		ObservabilityDashboardInstanceCreateSubject,
		ObservabilityDashboardInstanceUpdateSubject,
		ObservabilityDashboardInstanceDeleteSubject,
	}
}

// Get GetMetricsDefinitionSubjects returns the NATS subjects
// for metrics definitions.
func GetMetricsDefinitionSubjects() []string {
	return []string{
		MetricsDefinitionCreateSubject,
		MetricsDefinitionUpdateSubject,
		MetricsDefinitionDeleteSubject,
	}
}

// Get GetMetricsInstanceSubjects returns the NATS subjects
// for metrics instances.
func GetMetricsInstanceSubjects() []string {
	return []string{
		MetricsInstanceCreateSubject,
		MetricsInstanceUpdateSubject,
		MetricsInstanceDeleteSubject,
	}
}

// Get GetLoggingDefinitionSubjects returns the NATS subjects
// for logging definitions.
func GetLoggingDefinitionSubjects() []string {
	return []string{
		LoggingDefinitionCreateSubject,
		LoggingDefinitionUpdateSubject,
		LoggingDefinitionDeleteSubject,
	}
}

// Get GetLoggingInstanceSubjects returns the NATS subjects
// for logging instances.
func GetLoggingInstanceSubjects() []string {
	return []string{
		LoggingInstanceCreateSubject,
		LoggingInstanceUpdateSubject,
		LoggingInstanceDeleteSubject,
	}
}

// GetObservabilitySubjects returns the NATS subjects
// for all observability objects.
func GetObservabilitySubjects() []string {
	var observabilitySubjects []string

	observabilitySubjects = append(observabilitySubjects, GetObservabilityStackDefinitionSubjects()...)
	observabilitySubjects = append(observabilitySubjects, GetObservabilityStackInstanceSubjects()...)
	observabilitySubjects = append(observabilitySubjects, GetObservabilityDashboardDefinitionSubjects()...)
	observabilitySubjects = append(observabilitySubjects, GetObservabilityDashboardInstanceSubjects()...)
	observabilitySubjects = append(observabilitySubjects, GetMetricsDefinitionSubjects()...)
	observabilitySubjects = append(observabilitySubjects, GetMetricsInstanceSubjects()...)
	observabilitySubjects = append(observabilitySubjects, GetLoggingDefinitionSubjects()...)
	observabilitySubjects = append(observabilitySubjects, GetLoggingInstanceSubjects()...)

	return observabilitySubjects
}