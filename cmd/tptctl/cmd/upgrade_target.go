package cmd

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// upgradeRollTarget returns the control plane whose deployments the upgrade rolls.
// An empty name rolls the connected control plane.
// A name rolls that descendant, and only when the connection is its parent or an ancestor.
func upgradeRollTarget(
	self *api_v0.ControlPlaneInstance,
	name string,
	byName func(string) (*api_v0.ControlPlaneInstance, error),
	byID func(uint) (*api_v0.ControlPlaneInstance, error),
) (*api_v0.ControlPlaneInstance, error) {
	// a connection to a child is refused before a name is considered
	if err := api_v0.ChildControlPlaneRequest(self); err != nil {
		return nil, err
	}
	if name == "" {
		return self, nil
	}

	// load the named control plane from the API this command is connected to
	target, err := byName(name)
	if err != nil {
		return nil, err
	}
	if !controlPlaneDescendsFrom(target, self, byID) {
		return nil, fmt.Errorf(
			"%s is not a control plane of %s",
			name,
			util.DerefString(self.Name),
		)
	}
	return target, nil
}

// rollNamespaceImages selects the control plane and sets its deployment image tags.
// A refused connection returns before any deployment is changed.
func rollNamespaceImages(
	self *api_v0.ControlPlaneInstance,
	name string,
	byName func(string) (*api_v0.ControlPlaneInstance, error),
	byID func(uint) (*api_v0.ControlPlaneInstance, error),
	kube dynamic.Interface,
	tag string,
) error {
	// choose the control plane before touching a deployment
	target, err := upgradeRollTarget(self, name, byName, byID)
	if err != nil {
		return err
	}
	return tagDeployments(kube, util.DerefString(target.Namespace), tag)
}

// tagDeployments sets the image tag on every deployment in one namespace.
func tagDeployments(kube dynamic.Interface, namespace, tag string) error {
	// set the tag on each deployment in the selected namespace
	gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	list, err := kube.Resource(gvr).Namespace(namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	for i := range list.Items {
		item := &list.Items[i]
		containers, _, _ := unstructured.NestedSlice(item.Object, "spec", "template", "spec", "containers")
		for n, raw := range containers {
			container, _ := raw.(map[string]interface{})
			image, _ := container["image"].(string)
			if colon := strings.LastIndex(image, ":"); colon >= 0 {
				image = image[:colon]
			}
			container["image"] = image + ":" + tag
			containers[n] = container
		}
		if err := unstructured.SetNestedSlice(item.Object, containers, "spec", "template", "spec", "containers"); err != nil {
			return err
		}
		if _, err := kube.Resource(gvr).Namespace(namespace).Update(context.Background(), item, metav1.UpdateOptions{}); err != nil {
			return err
		}
	}
	return nil
}

// controlPlaneDescendsFrom reports whether target's parent chain reaches ancestor.
func controlPlaneDescendsFrom(
	target *api_v0.ControlPlaneInstance,
	ancestor *api_v0.ControlPlaneInstance,
	byID func(uint) (*api_v0.ControlPlaneInstance, error),
) bool {
	if target == nil || ancestor == nil || ancestor.ID == nil {
		return false
	}
	// walk parents until the connected control plane or a root
	current := target
	for i := 0; i < 16; i++ {
		if current.ParentControlPlaneInstanceID == nil {
			return false
		}
		if *current.ParentControlPlaneInstanceID == *ancestor.ID {
			return true
		}
		parent, err := byID(*current.ParentControlPlaneInstanceID)
		if err != nil || parent == nil {
			return false
		}
		current = parent
	}
	return false
}
