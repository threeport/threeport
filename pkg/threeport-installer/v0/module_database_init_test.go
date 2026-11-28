package v0

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stesting "k8s.io/client-go/testing"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// legacyModuleAPIDeployment is a module API server's Deployment as modules
// were installed when they created a database of their own: a db-init init
// container mounting the db-create ConfigMap and the database's root client
// certificate, ahead of the database-migrator.
func legacyModuleAPIDeployment(namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]interface{}{
			"name":      "threeport-widget-api-server",
			"namespace": namespace,
		},
		"spec": map[string]interface{}{
			"replicas": int64(1),
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"initContainers": []interface{}{
						map[string]interface{}{
							"name":  "db-init",
							"image": "docker.io/cockroachdb/cockroach:v23.1.14",
							"volumeMounts": []interface{}{
								map[string]interface{}{"name": "db-create", "mountPath": "/etc/threeport/db-create"},
								map[string]interface{}{"name": "db-root-cert", "mountPath": "/etc/threeport/db-certs"},
							},
						},
						map[string]interface{}{
							"name": "database-migrator",
							"volumeMounts": []interface{}{
								map[string]interface{}{"name": "db-threeport-cert", "mountPath": "/etc/threeport/db-certs"},
							},
						},
					},
					"containers": []interface{}{
						map[string]interface{}{
							"name": "api-server",
							"volumeMounts": []interface{}{
								map[string]interface{}{"name": "db-threeport-cert", "mountPath": "/etc/threeport/db-certs"},
							},
						},
					},
					"volumes": []interface{}{
						map[string]interface{}{"name": "db-root-cert", "secret": map[string]interface{}{"secretName": "db-root-cert"}},
						map[string]interface{}{"name": "db-threeport-cert", "secret": map[string]interface{}{"secretName": "db-threeport-cert"}},
						map[string]interface{}{"name": "db-create", "configMap": map[string]interface{}{"name": "db-create"}},
					},
				},
			},
		},
	}}
}

// podSpecNames returns the names of a Deployment's init containers and
// volumes.
func podSpecNames(t *testing.T, deployment *unstructured.Unstructured) (initContainers, volumes []string) {
	t.Helper()

	for key, names := range map[string]*[]string{"initContainers": &initContainers, "volumes": &volumes} {
		items, _, err := unstructured.NestedSlice(deployment.Object, "spec", "template", "spec", key)
		require.NoError(t, err)
		for _, item := range items {
			*names = append(*names, item.(map[string]interface{})["name"].(string))
		}
	}

	return initContainers, volumes
}

// TestAModuleDeploymentDropsTheInitContainerThatCreatedItsDatabase covers
// #825: a module Deployment rolled by a redeploy or an upgrade loses its
// db-init init container, and the db-create and db-root-cert volumes only it
// mounted.  A volume something else in the pod still mounts is kept, and a
// Deployment without db-init is left as it is.
func TestAModuleDeploymentDropsTheInitContainerThatCreatedItsDatabase(t *testing.T) {
	deployment := legacyModuleAPIDeployment("threeport-widget")
	require.True(t, dropModuleDatabaseInit(deployment))
	initContainers, volumes := podSpecNames(t, deployment)
	assert.Equal(t, []string{"database-migrator"}, initContainers)
	assert.Equal(t, []string{"db-threeport-cert"}, volumes)

	assert.False(t, dropModuleDatabaseInit(deployment), "a Deployment without it")

	// a module whose own container mounts the root certificate keeps it
	mounted := legacyModuleAPIDeployment("threeport-widget")
	containers, _, _ := unstructured.NestedSlice(mounted.Object, "spec", "template", "spec", "containers")
	apiServer := containers[0].(map[string]interface{})
	apiServer["volumeMounts"] = append(apiServer["volumeMounts"].([]interface{}),
		map[string]interface{}{"name": "db-root-cert", "mountPath": "/root-certs"})
	require.NoError(t, unstructured.SetNestedSlice(mounted.Object, containers, "spec", "template", "spec", "containers"))
	require.True(t, dropModuleDatabaseInit(mounted))
	initContainers, volumes = podSpecNames(t, mounted)
	assert.Equal(t, []string{"database-migrator"}, initContainers)
	assert.Equal(t, []string{"db-root-cert", "db-threeport-cert"}, volumes)
}

// TestARedeployedModuleRestartsWithoutItsDatabaseInit covers the restart a
// redeploy and an upgrade make of a module Deployment: it stores the
// Deployment without db-init.
func TestARedeployedModuleRestartsWithoutItsDatabaseInit(t *testing.T) {
	kubeClient := testKubeClient(legacyModuleAPIDeployment("threeport-widget"))
	require.NoError(t, restartModuleDeployment(
		kubeClient, "threeport-widget", "threeport-widget-api-server", "threeport-control-plane",
		map[string]string{}, map[string]string{}, time.Now(),
	))

	stored, err := kubeClient.Resource(deploymentGVR).Namespace("threeport-widget").Get(
		context.Background(), "threeport-widget-api-server", metav1.GetOptions{},
	)
	require.NoError(t, err)
	initContainers, volumes := podSpecNames(t, stored)
	assert.Equal(t, []string{"database-migrator"}, initContainers)
	assert.Equal(t, []string{"db-threeport-cert"}, volumes)
}

// staleModuleObjects are what an install that created a module's database
// left in the module's namespace, and the control plane's own certificate.
func staleModuleObjects(moduleNamespace string) []runtime.Object {
	object := func(kind, name, namespace string) *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       kind,
			"metadata":   map[string]interface{}{"name": name, "namespace": namespace},
		}}
	}

	objects := []runtime.Object{
		object("ConfigMap", "db-create", moduleNamespace),
		object("Secret", "db-root-cert", moduleNamespace),
		object("Secret", "db-threeport-cert", moduleNamespace),
	}
	if moduleNamespace != "threeport-control-plane" {
		objects = append(objects,
			object("Secret", "db-root-cert", "threeport-control-plane"),
			object("ConfigMap", "db-create", "threeport-control-plane"),
		)
	}

	return objects
}

// TestStaleModuleDatabaseInitIsRemovedOnceNothingMountsIt covers #825's
// cleanup of a module's namespace: the db-create ConfigMap and the copy of
// the database's root client certificate are deleted once no Deployment in
// the namespace mounts them, and kept while one does, as its pods couldn't
// start without them.  The threeport user's certificate the module connects
// with stays, as does everything in the control plane's own namespace, even
// when the module was installed there.  What's already gone is no error.
func TestStaleModuleDatabaseInitIsRemovedOnceNothingMountsIt(t *testing.T) {
	check := func(t *testing.T, objects []runtime.Object, moduleNamespace string) map[string]bool {
		t.Helper()
		kubeClient := testKubeClient(objects...)
		require.NoError(t, RemoveStaleModuleDatabaseInit(kubeClient, "threeport-control-plane", moduleNamespace))
		left := map[string]bool{}
		for _, namespace := range []string{moduleNamespace, "threeport-control-plane"} {
			for _, name := range []string{"db-create"} {
				_, err := kubeClient.Resource(moduleConfigMapGVR).Namespace(namespace).Get(context.Background(), name, metav1.GetOptions{})
				left[namespace+"/configmap/"+name] = err == nil
			}
			for _, name := range []string{"db-root-cert", "db-threeport-cert"} {
				_, err := kubeClient.Resource(secretGVR).Namespace(namespace).Get(context.Background(), name, metav1.GetOptions{})
				left[namespace+"/secret/"+name] = err == nil
			}
		}
		return left
	}

	left := check(t, staleModuleObjects("threeport-widget"), "threeport-widget")
	assert.False(t, left["threeport-widget/configmap/db-create"])
	assert.False(t, left["threeport-widget/secret/db-root-cert"])
	assert.True(t, left["threeport-widget/secret/db-threeport-cert"], "the threeport user's certificate the module connects with")
	assert.True(t, left["threeport-control-plane/secret/db-root-cert"], "the control plane's own")
	assert.True(t, left["threeport-control-plane/configmap/db-create"])

	mounting := append(staleModuleObjects("threeport-widget"), legacyModuleAPIDeployment("threeport-widget"))
	left = check(t, mounting, "threeport-widget")
	assert.True(t, left["threeport-widget/configmap/db-create"], "while a Deployment mounts it")
	assert.True(t, left["threeport-widget/secret/db-root-cert"])

	rolled := legacyModuleAPIDeployment("threeport-widget")
	dropModuleDatabaseInit(rolled)
	left = check(t, append(staleModuleObjects("threeport-widget"), rolled), "threeport-widget")
	assert.False(t, left["threeport-widget/configmap/db-create"], "once its Deployment no longer mounts it")
	assert.False(t, left["threeport-widget/secret/db-root-cert"])

	left = check(t, staleModuleObjects("threeport-control-plane"), "threeport-control-plane")
	assert.True(t, left["threeport-control-plane/secret/db-root-cert"], "a module installed in the control plane's namespace")
	assert.True(t, left["threeport-control-plane/configmap/db-create"])

	left = check(t, nil, "threeport-widget")
	assert.False(t, left["threeport-widget/secret/db-root-cert"], "already gone")
}

// TestAFailedCleanupIsReported covers a namespace that can't be listed or a
// delete that fails for another reason than the object being gone: the
// install or redeploy fails, rather than leaving the root certificate's copy
// behind unnoticed.
func TestAFailedCleanupIsReported(t *testing.T) {
	listing := testKubeClient()
	listing.PrependReactor("list", "deployments", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("connection refused")
	})
	assert.ErrorContains(t, RemoveStaleModuleDatabaseInit(listing, "threeport-control-plane", "threeport-widget"),
		"failed to list the deployments in threeport-widget: connection refused")

	deleting := testKubeClient(staleModuleObjects("threeport-widget")...)
	deleting.PrependReactor("delete", "secrets", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, k8serrors.NewForbidden(secretGVR.GroupResource(), "db-root-cert", errors.New("denied"))
	})
	assert.ErrorContains(t, RemoveStaleModuleDatabaseInit(deleting, "threeport-control-plane", "threeport-widget"),
		"failed to delete secret threeport-widget/db-root-cert")
}

// TestAnUpgradedModuleLosesWhatItsDatabaseWasCreatedWith covers #825 for a
// module installed when modules created a database of their own, upgraded
// to new images without running its installer again: its API Deployment is
// restarted without db-init, and the db-create ConfigMap and the copy of the
// database's root client certificate are deleted from its namespace.
func TestAnUpgradedModuleLosesWhatItsDatabaseWasCreatedWith(t *testing.T) {
	namespace := installControlPlaneNamespace
	deployment := legacyModuleAPIDeployment("custom-ns")
	deployment.SetName("threeport-example-api-server")
	spec, _, _ := unstructured.NestedMap(deployment.Object, "spec", "template", "spec")
	spec["initContainers"].([]interface{})[1].(map[string]interface{})["image"] = "registry.example/threeport-example-database-migrator:old"
	spec["containers"].([]interface{})[0].(map[string]interface{})["image"] = "registry.example/threeport-example-rest-api:old"
	require.NoError(t, unstructured.SetNestedMap(deployment.Object, spec, "spec", "template", "spec"))
	mapper := WorkloadRESTMapper()
	kubeClient := testKubeClient(append(staleModuleObjects("custom-ns"), deployment, installKeySecret(namespace))...)
	endpoint := "example-api.custom-ns.svc.cluster.local"
	module := &v0.ModuleApi{
		Name:           util.Ptr("example.com/example-module-api"),
		ImageNamespace: util.Ptr("registry.example"),
		ImageTag:       util.Ptr("new"),
		Endpoint:       &endpoint,
	}
	name := namespace
	apiEndpoint := "https://api.example"
	instance := &v0.ControlPlaneInstance{Name: &name, Namespace: &namespace, ApiServerEndpoint: &apiEndpoint}
	cpi := NewInstaller(Namespace(namespace))
	cpi.Opts.ControllerList = nil

	require.NoError(t, retagModuleAPI(kubeClient, &mapper, instance, cpi, module, "registry.example", "new", namespace))

	stored, err := kubeClient.Resource(deploymentGVR).Namespace("custom-ns").Get(
		context.Background(), "threeport-example-api-server", metav1.GetOptions{},
	)
	require.NoError(t, err)
	initContainers, _ := podSpecNames(t, stored)
	assert.Equal(t, []string{"database-migrator"}, initContainers)
	_, err = kubeClient.Resource(secretGVR).Namespace("custom-ns").Get(context.Background(), "db-root-cert", metav1.GetOptions{})
	assert.True(t, k8serrors.IsNotFound(err), "the root certificate's copy is deleted")
	_, err = kubeClient.Resource(moduleConfigMapGVR).Namespace("custom-ns").Get(context.Background(), "db-create", metav1.GetOptions{})
	assert.True(t, k8serrors.IsNotFound(err))
	_, err = kubeClient.Resource(secretGVR).Namespace("threeport-control-plane").Get(context.Background(), "db-root-cert", metav1.GetOptions{})
	assert.NoError(t, err, "another namespace's is kept")
}

// TestARedeployedModuleLosesWhatItsDatabaseWasCreatedWith covers #825 for a
// control plane upgrade or restore that redeploys a module installed when
// modules created a database of their own: the root certificate is no
// longer copied into its namespace, its API Deployment is restarted without
// db-init, and the db-create ConfigMap and the certificate's copy are
// deleted.  A module registered without an ID is cleaned up too.
func TestARedeployedModuleLosesWhatItsDatabaseWasCreatedWith(t *testing.T) {
	controlPlane := "threeport-control-plane"
	secret := func(name, namespace string) *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata":   map[string]interface{}{"name": name, "namespace": namespace},
			"data":       map[string]interface{}{"key": "dmFsdWU="},
		}}
	}
	objects := staleModuleObjects("threeport-widget")
	for _, name := range append(ModuleCopiedSecretNames(), natsCredentialsSecretName) {
		objects = append(objects, secret(name, controlPlane))
	}
	objects = append(objects,
		&unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata":   map[string]interface{}{"name": "threeport-widget"},
		}},
		&unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata":   map[string]interface{}{"name": "threeport-widget-api-server", "namespace": "threeport-widget"},
			"spec": map[string]interface{}{
				"selector": map[string]interface{}{"app.kubernetes.io/name": "threeport-widget-api-server"},
			},
		}},
		legacyModuleAPIDeployment("threeport-widget"),
	)
	kubeClient := testKubeClient(objects...)
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{{Version: "v1"}, {Group: "networking.k8s.io", Version: "v1"}})
	for _, kind := range []string{"Secret", "Service"} {
		mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: kind}, meta.RESTScopeNamespace)
	}
	mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "Namespace"}, meta.RESTScopeRoot)
	mapper.Add(schema.GroupVersionKind{Group: "networking.k8s.io", Version: "v1", Kind: "NetworkPolicy"}, meta.RESTScopeNamespace)
	cpi := NewInstaller(Namespace(controlPlane))

	endpoint := "threeport-widget-api-server.threeport-widget.svc.cluster.local"
	require.NoError(t, cpi.redeployInstalledModule(kubeClient, mapper, v0.ModuleApi{
		Name:     util.Ptr("widget.example.com/widget-module-api"),
		Endpoint: &endpoint,
	}, nil, time.Now()))

	stored, err := kubeClient.Resource(deploymentGVR).Namespace("threeport-widget").Get(
		context.Background(), "threeport-widget-api-server", metav1.GetOptions{},
	)
	require.NoError(t, err)
	initContainers, volumes := podSpecNames(t, stored)
	assert.Equal(t, []string{"database-migrator"}, initContainers)
	assert.Equal(t, []string{"db-threeport-cert"}, volumes)
	_, err = kubeClient.Resource(secretGVR).Namespace("threeport-widget").Get(context.Background(), "db-root-cert", metav1.GetOptions{})
	assert.True(t, k8serrors.IsNotFound(err), "the root certificate's copy is deleted, and not copied again")
	_, err = kubeClient.Resource(moduleConfigMapGVR).Namespace("threeport-widget").Get(context.Background(), "db-create", metav1.GetOptions{})
	assert.True(t, k8serrors.IsNotFound(err))
	_, err = kubeClient.Resource(secretGVR).Namespace("threeport-widget").Get(context.Background(), "db-threeport-cert", metav1.GetOptions{})
	assert.NoError(t, err, "the threeport user's certificate is copied")
	_, err = kubeClient.Resource(secretGVR).Namespace(controlPlane).Get(context.Background(), "db-root-cert", metav1.GetOptions{})
	assert.NoError(t, err, "the control plane's own is kept")
}

// TestAFailedModuleCleanupIsRetriedOnceTheImagesAreCurrent covers an image
// upgrade whose cleanup fails after it restarted the module's API
// Deployment onto the new images: the next reconcile finds the images
// current, and still removes what the module's own database was created
// with, rather than leaving the root certificate's copy for good.
func TestAFailedModuleCleanupIsRetriedOnceTheImagesAreCurrent(t *testing.T) {
	namespace := installControlPlaneNamespace
	deployment := legacyModuleAPIDeployment("custom-ns")
	deployment.SetName("threeport-example-api-server")
	spec, _, _ := unstructured.NestedMap(deployment.Object, "spec", "template", "spec")
	spec["initContainers"].([]interface{})[1].(map[string]interface{})["image"] = "registry.example/threeport-example-database-migrator:old"
	spec["containers"].([]interface{})[0].(map[string]interface{})["image"] = "registry.example/threeport-example-rest-api:old"
	require.NoError(t, unstructured.SetNestedMap(deployment.Object, spec, "spec", "template", "spec"))
	mapper := WorkloadRESTMapper()
	kubeClient := testKubeClient(append(staleModuleObjects("custom-ns"), deployment, installKeySecret(namespace))...)
	failures := 1
	kubeClient.PrependReactor("delete", "secrets", func(k8stesting.Action) (bool, runtime.Object, error) {
		if failures == 0 {
			return false, nil, nil
		}
		failures--
		return true, nil, errors.New("connection refused")
	})
	endpoint := "example-api.custom-ns.svc.cluster.local"
	module := &v0.ModuleApi{
		Name:           util.Ptr("example.com/example-module-api"),
		ImageNamespace: util.Ptr("registry.example"),
		ImageTag:       util.Ptr("new"),
		Endpoint:       &endpoint,
	}
	name := namespace
	apiEndpoint := "https://api.example"
	instance := &v0.ControlPlaneInstance{Name: &name, Namespace: &namespace, ApiServerEndpoint: &apiEndpoint}
	cpi := NewInstaller(Namespace(namespace))
	cpi.Opts.ControllerList = nil
	certificateLeft := func() bool {
		_, err := kubeClient.Resource(secretGVR).Namespace("custom-ns").Get(context.Background(), "db-root-cert", metav1.GetOptions{})
		return err == nil
	}

	err := retagModuleAPI(kubeClient, &mapper, instance, cpi, module, "registry.example", "new", namespace)
	require.ErrorContains(t, err, "connection refused")
	assert.True(t, certificateLeft())
	stored, err := kubeClient.Resource(deploymentGVR).Namespace("custom-ns").Get(
		context.Background(), "threeport-example-api-server", metav1.GetOptions{},
	)
	require.NoError(t, err)
	current, _ := namedContainerImage(stored, "containers", moduleAPIContainerName)
	require.Equal(t, "registry.example/threeport-example-rest-api:new", current, "restarted onto the new images first")

	require.NoError(t, retagModuleAPI(kubeClient, &mapper, instance, cpi, module, "registry.example", "new", namespace))
	assert.False(t, certificateLeft(), "the retry removes it, with the images already current")
}
