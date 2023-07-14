// +threeport-codegen route-exclude
// +threeport-codegen database-exclude
package v0

import "errors"

// GetSubjectByReconcilerName returns the subject for a reconciler's name.
func GetSubjectByReconcilerName(name string) (string, error) {
	switch name {
	case "WorkloadDefinitionReconciler":
		return WorkloadDefinitionSubject, nil
	case "WorkloadInstanceReconciler":
		return WorkloadInstanceSubject, nil
	case "GatewayDefinitionReconciler":
		return GatewayDefinitionSubject, nil
	case "GatewayInstanceReconciler":
		return GatewayInstanceSubject, nil
	default:
		return "", errors.New("unrecognized reconciler name")
	}
}
