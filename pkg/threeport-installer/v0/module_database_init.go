package v0

import (
	"context"
	"fmt"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// A module installed before modules stopped creating a database of their own
// has, in its namespace, a db-create ConfigMap with the statements that
// created it, a copy of the database's root client certificate, and a db-init
// init container on its API server that ran the statements as root.  Nothing
// reads that database: a module's tables are in the control plane's.  The
// container and its volumes are dropped from a module's Deployments when they
// are rolled, and the ConfigMap and the certificate's copy are deleted once
// nothing in the namespace mounts them.
const (
	// moduleDbInitContainerName is the init container that created a
	// module's database.
	moduleDbInitContainerName = "db-init"

	// moduleDbCreateName is the ConfigMap, and the volume it was mounted
	// as, with the statements that created a module's database.
	moduleDbCreateName = "db-create"
)

// moduleDbInitVolumes are the volumes only a module's db-init container
// mounted.
var moduleDbInitVolumes = []string{moduleDbCreateName, dbRootCertSecretName}

// dropModuleDatabaseInit removes a module Deployment's db-init init
// container, and the db-create and db-root-cert volumes once no container of
// the pod mounts them, and reports whether it changed the Deployment.
func dropModuleDatabaseInit(deployment *unstructured.Unstructured) bool {
	spec, found, err := unstructured.NestedMap(deployment.Object, "spec", "template", "spec")
	if err != nil || !found {
		return false
	}

	changed := false
	initContainers, _, _ := unstructured.NestedSlice(spec, "initContainers")
	kept := make([]interface{}, 0, len(initContainers))
	for _, item := range initContainers {
		container, ok := item.(map[string]interface{})
		if ok && container["name"] == moduleDbInitContainerName {
			changed = true
			continue
		}
		kept = append(kept, item)
	}
	if changed {
		_ = unstructured.SetNestedSlice(spec, kept, "initContainers")
	}

	volumes, _, _ := unstructured.NestedSlice(spec, "volumes")
	keptVolumes := make([]interface{}, 0, len(volumes))
	for _, item := range volumes {
		volume, ok := item.(map[string]interface{})
		name, _ := volume["name"].(string)
		if ok && isModuleDbInitVolume(name) && !podMountsVolume(spec, name) {
			changed = true
			continue
		}
		keptVolumes = append(keptVolumes, item)
	}
	if len(keptVolumes) != len(volumes) {
		_ = unstructured.SetNestedSlice(spec, keptVolumes, "volumes")
	}

	if !changed {
		return false
	}
	_ = unstructured.SetNestedMap(deployment.Object, spec, "spec", "template", "spec")

	return true
}

// isModuleDbInitVolume reports whether a volume is one only a module's
// db-init container mounted.
func isModuleDbInitVolume(name string) bool {
	for _, volume := range moduleDbInitVolumes {
		if name == volume {
			return true
		}
	}

	return false
}

// podMountsVolume reports whether any container or init container of a pod
// spec mounts the volume named name.
func podMountsVolume(spec map[string]interface{}, name string) bool {
	for _, listKey := range []string{"initContainers", "containers"} {
		containers, _, _ := unstructured.NestedSlice(spec, listKey)
		for _, item := range containers {
			container, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			mounts, _, _ := unstructured.NestedSlice(container, "volumeMounts")
			for _, mount := range mounts {
				mountMap, ok := mount.(map[string]interface{})
				if ok && mountMap["name"] == name {
					return true
				}
			}
		}
	}

	return false
}

// RemoveStaleModuleDatabaseInit deletes from a module's namespace the
// db-create ConfigMap and the copy of the database's root client certificate
// a module installed before modules stopped creating a database of their own
// was given.  One that a Deployment in the namespace still mounts is kept, as
// its pods couldn't start without it, until a rollout without db-init.  The
// control plane's own namespace is never touched: its db-root-cert is the
// original, which backups, restores and reinstalls use.
func RemoveStaleModuleDatabaseInit(
	kubeClient dynamic.Interface,
	threeportNamespace string,
	moduleNamespace string,
) error {
	if moduleNamespace == "" || moduleNamespace == threeportNamespace {
		return nil
	}

	deployments, err := kubeClient.Resource(deploymentGVR).Namespace(moduleNamespace).List(
		context.Background(), metav1.ListOptions{},
	)
	if err != nil {
		return fmt.Errorf("failed to list the deployments in %s: %w", moduleNamespace, err)
	}
	mounted := map[string]bool{}
	for _, deployment := range deployments.Items {
		volumes, _, _ := unstructured.NestedSlice(deployment.Object, "spec", "template", "spec", "volumes")
		for _, item := range volumes {
			volume, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			if configMap, found, _ := unstructured.NestedString(volume, "configMap", "name"); found {
				mounted["configmap/"+configMap] = true
			}
			if secret, found, _ := unstructured.NestedString(volume, "secret", "secretName"); found {
				mounted["secret/"+secret] = true
			}
		}
	}

	for _, stale := range []struct {
		resource schema.GroupVersionResource
		kind     string
		name     string
	}{
		{moduleConfigMapGVR, "configmap", moduleDbCreateName},
		{secretGVR, "secret", dbRootCertSecretName},
	} {
		if mounted[stale.kind+"/"+stale.name] {
			continue
		}
		if err := kubeClient.Resource(stale.resource).Namespace(moduleNamespace).Delete(
			context.Background(), stale.name, metav1.DeleteOptions{},
		); err != nil && !k8serrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete %s %s/%s: %w", stale.kind, moduleNamespace, stale.name, err)
		}
	}

	return nil
}
