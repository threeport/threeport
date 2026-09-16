package v0

import (
	"context"
	"fmt"
	"sort"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"

	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// An API object group name such as kubernetes_workload maps onto
// the controller name kubernetes-workload-controller. Reinstall
// with no groups keeps only the controllers already installed.

// ParseApis splits a comma-separated list of API object group names.
func ParseApis(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// SelectControllersByGroup returns the controllers for the named
// API object groups. An empty list returns allControllers unchanged.
func SelectControllersByGroup(
	groupNames []string,
	allControllers []*v0.ControlPlaneComponent,
) ([]*v0.ControlPlaneComponent, error) {
	if len(groupNames) == 0 {
		return allControllers, nil
	}
	// select no controllers for group none
	if len(groupNames) == 1 && groupNames[0] == "none" {
		return []*v0.ControlPlaneComponent{}, nil
	}

	// index controllers by name
	byName := make(map[string]*v0.ControlPlaneComponent, len(allControllers))
	for _, controller := range allControllers {
		byName[controller.Name] = controller
	}

	// select controllers in group-name order
	selected := make([]*v0.ControlPlaneComponent, 0, len(groupNames))
	for _, groupName := range groupNames {
		controllerName := controllerNameForGroup(groupName)
		controller, ok := byName[controllerName]
		if !ok {
			return nil, fmt.Errorf(
				"unknown api object group %q: valid choices are %s",
				groupName,
				strings.Join(ApiObjectGroupNames, ", "),
			)
		}
		selected = append(selected, controller)
	}

	return selected, nil
}

// DetectInstalledControllerNames returns the names of installer-managed
// controllers in the namespace. labeledCount includes the API server and
// agent that names omits, so an API-only cluster still counts as labeled.
func DetectInstalledControllerNames(
	kubeClient dynamic.Interface,
	namespace string,
) (names []string, labeledCount int, err error) {
	selector := fmt.Sprintf("%s=%s", LabelManagedBy, LabelManagedByValue)

	// list installer-managed deployments
	list, err := kubeClient.Resource(deploymentGVR).Namespace(namespace).List(
		context.Background(),
		metav1.ListOptions{LabelSelector: selector},
	)
	if err != nil {
		return nil, 0, fmt.Errorf(
			"failed to list installer-managed deployments in namespace %q: %w",
			namespace, err,
		)
	}

	names = make([]string, 0, len(list.Items))
	for _, item := range list.Items {
		deployName := item.GetName()
		// skip the API server and agent, which are not group-scoped controllers
		if deployName == ThreeportAPIServiceResourceName || deployName == ThreeportAgentDeployName {
			continue
		}
		// strip the threeport- prefix to match controller names
		stripped := strings.TrimPrefix(deployName, "threeport-")
		names = append(names, stripped)
	}

	// sort the names
	sort.Strings(names)
	return names, len(list.Items), nil
}

// SelectControllersForReinstall returns the controllers to reinstall.
// An empty group list detects the installed set from the cluster.
func SelectControllersForReinstall(
	kubeClient dynamic.Interface,
	namespace string,
	explicitGroups []string,
	allControllers []*v0.ControlPlaneComponent,
) ([]*v0.ControlPlaneComponent, []string, bool, error) {
	if len(explicitGroups) > 0 {
		// select controllers for explicitGroups
		selected, err := SelectControllersByGroup(explicitGroups, allControllers)
		if err != nil {
			return nil, nil, false, err
		}
		return selected, controllerNames(selected), false, nil
	}

	// detect installed controller names from the cluster
	detectedNames, labeledCount, err := DetectInstalledControllerNames(kubeClient, namespace)
	if err != nil {
		return nil, nil, true, fmt.Errorf("failed to detect installed controllers: %w", err)
	}

	// keep the full controller set when nothing is labeled
	if labeledCount == 0 {
		return allControllers, controllerNames(allControllers), true, nil
	}

	// index detected names
	wanted := make(map[string]struct{}, len(detectedNames))
	for _, name := range detectedNames {
		wanted[name] = struct{}{}
	}

	// keep installed controllers in allControllers order
	selected := make([]*v0.ControlPlaneComponent, 0, len(detectedNames))
	selectedNames := make([]string, 0, len(detectedNames))
	for _, controller := range allControllers {
		if _, ok := wanted[controller.Name]; ok {
			selected = append(selected, controller)
			selectedNames = append(selectedNames, controller.Name)
		}
	}

	return selected, selectedNames, true, nil
}

// controllerNames returns each controller's name in the same order.
func controllerNames(controllers []*v0.ControlPlaneComponent) []string {
	names := make([]string, 0, len(controllers))
	for _, controller := range controllers {
		names = append(names, controller.Name)
	}
	return names
}

// controllerNameForGroup maps an API object group name onto a controller name.
func controllerNameForGroup(groupName string) string {
	return strings.ReplaceAll(fmt.Sprintf("%s-controller", groupName), "_", "-")
}
