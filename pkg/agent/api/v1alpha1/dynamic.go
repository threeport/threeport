package v1alpha1

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// Unstructured takes a typed ThreeportWorkload instance a returns an
// unstructured object for use with a dynamic client.
func UnstructuredThreeportWorkload(threeportWorkload *ThreeportWorkload) (*unstructured.Unstructured, error) {
	var unstructured unstructured.Unstructured

	object, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&threeportWorkload)
	if err != nil {
		return &unstructured, fmt.Errorf("failed to convert ThreeportWorkload into unstructured object: %w", err)
	}
	unstructured.Object = object
	unstructured.SetAPIVersion(fmt.Sprintf("%s/%s", GroupVersion.Group, GroupVersion.Version))
	unstructured.SetKind(ThreeportWorkloadKind)
	unstructured.SetName(threeportWorkload.ObjectMeta.Name)

	return &unstructured, nil
}
