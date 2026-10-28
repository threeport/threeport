package kubernetesworkload

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"

	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	tp_errors "github.com/threeport/threeport/pkg/errors/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// fakeThreeportAPI holds the workload objects a definition update reads and
// writes, and records every write it receives.
type fakeThreeportAPI struct {
	mu        sync.Mutex
	nextID    uint
	wrds      map[uint]*v0.KubernetesWorkloadResourceDefinition
	instances map[uint]*v0.KubernetesWorkloadInstance
	wris      map[uint]*v0.KubernetesWorkloadResourceInstance
	writes    []string

	// failWrite, when set, fails every write to a path with this prefix
	failWrite string
}

func newFakeThreeportAPI() *fakeThreeportAPI {
	return &fakeThreeportAPI{
		nextID:    100,
		wrds:      map[uint]*v0.KubernetesWorkloadResourceDefinition{},
		instances: map[uint]*v0.KubernetesWorkloadInstance{},
		wris:      map[uint]*v0.KubernetesWorkloadResourceInstance{},
	}
}

func (f *fakeThreeportAPI) id() *uint {
	f.nextID++
	id := f.nextID
	return &id
}

func (f *fakeThreeportAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	respond := func(status int, objects ...apiserver_lib.Object) {
		body, _ := json.Marshal(apiserver_lib.Response{
			Status: apiserver_lib.Status{Code: status},
			Data:   objects,
		})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}
	queryID := func(name string) uint {
		id, _ := strconv.ParseUint(r.URL.Query().Get(name), 10, 64)
		return uint(id)
	}
	pathID := func(prefix string) uint {
		id, _ := strconv.ParseUint(strings.TrimPrefix(r.URL.Path, prefix+"/"), 10, 64)
		return uint(id)
	}
	if r.Method != http.MethodGet {
		if f.failWrite != "" && strings.HasPrefix(r.URL.Path, f.failWrite) {
			respond(http.StatusInternalServerError)
			return
		}
		f.writes = append(f.writes, r.Method+" "+r.URL.Path)
	}

	switch {
	case r.URL.Path == v0.PathKubernetesWorkloadResourceDefinitions && r.Method == http.MethodGet:
		var found []apiserver_lib.Object
		for _, id := range sortedIDs(f.wrds) {
			if *f.wrds[id].KubernetesWorkloadDefinitionID == queryID("kubernetesworkloaddefinitionid") {
				found = append(found, *f.wrds[id])
			}
		}
		respond(http.StatusOK, found...)
	case r.URL.Path == v0.PathKubernetesWorkloadResourceDefinitions && r.Method == http.MethodPost:
		var wrd v0.KubernetesWorkloadResourceDefinition
		_ = json.NewDecoder(r.Body).Decode(&wrd)
		wrd.ID = f.id()
		f.wrds[*wrd.ID] = &wrd
		respond(http.StatusCreated, wrd)
	case strings.HasPrefix(r.URL.Path, v0.PathKubernetesWorkloadResourceDefinitions+"/"):
		id := pathID(v0.PathKubernetesWorkloadResourceDefinitions)
		wrd, found := f.wrds[id]
		if !found {
			respond(http.StatusNotFound)
			return
		}
		if r.Method == http.MethodDelete {
			delete(f.wrds, id)
		} else {
			_ = json.NewDecoder(r.Body).Decode(wrd)
		}
		respond(http.StatusOK, *wrd)

	case r.URL.Path == v0.PathKubernetesWorkloadInstances && r.Method == http.MethodGet:
		var found []apiserver_lib.Object
		for _, id := range sortedIDs(f.instances) {
			if *f.instances[id].KubernetesWorkloadDefinitionID == queryID("kubernetesworkloaddefinitionid") {
				found = append(found, *f.instances[id])
			}
		}
		respond(http.StatusOK, found...)
	case strings.HasPrefix(r.URL.Path, v0.PathKubernetesWorkloadInstances+"/"):
		instance := f.instances[pathID(v0.PathKubernetesWorkloadInstances)]
		_ = json.NewDecoder(r.Body).Decode(instance)
		respond(http.StatusOK, *instance)

	case r.URL.Path == v0.PathKubernetesWorkloadResourceInstances && r.Method == http.MethodGet:
		var found []apiserver_lib.Object
		for _, id := range sortedIDs(f.wris) {
			if *f.wris[id].KubernetesWorkloadInstanceID == queryID("kubernetesworkloadinstanceid") {
				found = append(found, *f.wris[id])
			}
		}
		respond(http.StatusOK, found...)
	case r.URL.Path == v0.PathKubernetesWorkloadResourceInstances && r.Method == http.MethodPost:
		var wri v0.KubernetesWorkloadResourceInstance
		_ = json.NewDecoder(r.Body).Decode(&wri)
		wri.ID = f.id()
		f.wris[*wri.ID] = &wri
		respond(http.StatusCreated, wri)
	case strings.HasPrefix(r.URL.Path, v0.PathKubernetesWorkloadResourceInstances+"/"):
		wri := f.wris[pathID(v0.PathKubernetesWorkloadResourceInstances)]
		_ = json.NewDecoder(r.Body).Decode(wri)
		respond(http.StatusOK, *wri)

	default:
		respond(http.StatusNotFound)
	}
}

func sortedIDs[T any](objects map[uint]*T) []uint {
	var ids []uint
	for id := range objects {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// fakeKubeDiscovery answers the discovery requests kube.SetNamespaces makes,
// so the namespaces are set exactly as they are for a real runtime.
func fakeKubeDiscovery(t *testing.T) *discovery.DiscoveryClient {
	t.Helper()

	resources := map[string]metav1.APIResourceList{
		"/api/v1": {GroupVersion: "v1", APIResources: []metav1.APIResource{
			{Name: "namespaces", Kind: "Namespace", Namespaced: false},
			{Name: "services", Kind: "Service", Namespaced: true},
			{Name: "configmaps", Kind: "ConfigMap", Namespaced: true},
		}},
		"/apis/apps/v1": {GroupVersion: "apps/v1", APIResources: []metav1.APIResource{
			{Name: "deployments", Kind: "Deployment", Namespaced: true},
		}},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		list, found := resources[r.URL.Path]
		if !found {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(list)
	}))
	t.Cleanup(srv.Close)

	return discovery.NewDiscoveryClientForConfigOrDie(&rest.Config{Host: srv.URL})
}

const (
	webDeployment = `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
spec:
  replicas: %d
  selector:
    matchLabels:
      app: web
  template:
    metadata:
      labels:
        app: web
    spec:
      containers:
      - name: web
        image: %s
`
	webService = `
apiVersion: v1
kind: Service
metadata:
  name: web
spec:
  selector:
    app: web
  ports:
  - port: 80
`
	webConfigMap = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: web
data:
  greeting: hello
`
	// an overlay that patches the definition's deployment, so what the
	// overlaid instance holds is never a copy of the resource definitions
	envOverlay = `
patches:
- target:
    kind: Deployment
    name: web
  patch: |-
    - op: add
      path: /spec/template/spec/containers/0/env
      value:
      - name: TIER
        value: canary
`
)

func yamlDocument(documents ...string) *string {
	document := strings.Join(documents, "\n---\n")
	return &document
}

// definitionUpdateFixture is a deployed workload definition with instances in
// each state an update meets them in.
type definitionUpdateFixture struct {
	api        *fakeThreeportAPI
	reconciler *controller.Reconciler
	definition *v0.KubernetesWorkloadDefinition

	plain, overlaid, undeployed, deleting uint
}

func newDefinitionUpdateFixture(t *testing.T, yaml *string) *definitionUpdateFixture {
	t.Helper()

	api := newFakeThreeportAPI()
	srv := httptest.NewServer(api)
	t.Cleanup(srv.Close)

	discoveryClient := fakeKubeDiscovery(t)
	original := runtimeNamespaceSetter
	runtimeNamespaceSetter = func(*controller.Reconciler, *v0.KubernetesWorkloadInstance) (namespaceSetter, error) {
		return func(
			wris *[]v0.KubernetesWorkloadResourceInstance,
			instance *v0.KubernetesWorkloadInstance,
		) (*[]v0.KubernetesWorkloadResourceInstance, error) {
			return kube.SetNamespaces(wris, instance.Name, instance.ID, discoveryClient)
		}, nil
	}
	t.Cleanup(func() { runtimeNamespaceSetter = original })

	definitionID := uint(1)
	fixture := &definitionUpdateFixture{
		api:        api,
		reconciler: &controller.Reconciler{APIClient: &http.Client{}, APIServer: strings.TrimPrefix(srv.URL, "http://")},
		definition: &v0.KubernetesWorkloadDefinition{
			Common:       v0.Common{ID: &definitionID},
			Definition:   v0.Definition{Name: util.Ptr("web")},
			YAMLDocument: yaml,
		},
	}

	// the definition's resources, as v0KubernetesWorkloadDefinitionCreated stores them
	jsonObjects, err := kube.GetJsonResourcesFromYamlDoc(*yaml)
	require.NoError(t, err)
	var wrds []v0.KubernetesWorkloadResourceDefinition
	for _, jsonContent := range jsonObjects {
		var jsonDefinition datatypes.JSON
		require.NoError(t, jsonDefinition.UnmarshalJSON(jsonContent))
		wrd := v0.KubernetesWorkloadResourceDefinition{
			Common:                         v0.Common{ID: api.id()},
			JSONDefinition:                 &jsonDefinition,
			KubernetesWorkloadDefinitionID: &definitionID,
		}
		api.wrds[*wrd.ID] = &wrd
		wrds = append(wrds, wrd)
	}

	// instances, deployed the way v0KubernetesWorkloadInstanceCreated deploys them
	addInstance := func(name string, overlay *string, deployed bool, deleting bool) uint {
		instance := &v0.KubernetesWorkloadInstance{
			Common:                         v0.Common{ID: api.id()},
			Instance:                       v0.Instance{Name: util.Ptr(name)},
			Reconciliation:                 v0.Reconciliation{Reconciled: util.Ptr(true)},
			KubernetesWorkloadDefinitionID: &definitionID,
			KubernetesRuntimeInstanceID:    util.Ptr(uint(9)),
			KustomizeOverlay:               overlay,
		}
		if deleting {
			instance.DeletionScheduled = util.Ptr(time.Now())
		}
		api.instances[*instance.ID] = instance
		if deployed {
			wris, err := instanceResources(instance, wrds)
			require.NoError(t, err)
			processed, err := kube.SetNamespaces(&wris, instance.Name, instance.ID, discoveryClient)
			require.NoError(t, err)
			for _, wri := range *processed {
				wri.ID = api.id()
				wri.Reconciled = util.Ptr(true)
				api.wris[*wri.ID] = &wri
			}
		}
		return *instance.ID
	}
	fixture.plain = addInstance("plain", nil, true, false)
	fixture.overlaid = addInstance("overlaid", util.Ptr(envOverlay), true, false)
	fixture.undeployed = addInstance("undeployed", nil, false, false)
	fixture.deleting = addInstance("deleting", nil, true, true)
	api.writes = nil

	return fixture
}

// update runs the definition update reconciler against the fixture with the
// definition's YAML document replaced.
func (f *definitionUpdateFixture) update(yaml *string) error {
	f.definition.YAMLDocument = yaml
	log := logr.Discard()
	_, err := v0KubernetesWorkloadDefinitionUpdated(f.reconciler, f.definition, &log)
	return err
}

// resources returns what an instance holds, by resource, ignoring those on
// their way out.
func (f *definitionUpdateFixture) resources(t *testing.T, instanceID uint) map[string]v0.KubernetesWorkloadResourceInstance {
	t.Helper()
	byKey := map[string]v0.KubernetesWorkloadResourceInstance{}
	for _, wri := range f.api.wris {
		if *wri.KubernetesWorkloadInstanceID != instanceID {
			continue
		}
		key, _, err := resourceKey(*wri.JSONDefinition)
		require.NoError(t, err)
		if wri.ScheduledForDeletion != nil {
			key = "leaving:" + key
		}
		byKey[key] = *wri
	}
	return byKey
}

func resourceObject(t *testing.T, wri v0.KubernetesWorkloadResourceInstance) *unstructured.Unstructured {
	t.Helper()
	object := &unstructured.Unstructured{}
	require.NoError(t, object.UnmarshalJSON(*wri.JSONDefinition))
	return object
}

// TestDefinitionUpdateReachesRunningInstances is threeport#573: an edited YAML
// document now reaches the resource definitions and every running instance.
func TestDefinitionUpdateReachesRunningInstances(t *testing.T) {
	fixture := newDefinitionUpdateFixture(t, yamlDocument(
		fmt.Sprintf(webDeployment, 1, "nginx:1.26"),
		webService,
	))
	var plainNamespace string
	for key := range fixture.resources(t, fixture.plain) {
		if strings.HasPrefix(key, "/Namespace//") {
			plainNamespace = strings.TrimPrefix(key, "/Namespace//")
		}
	}
	require.NotEmpty(t, plainNamespace, "threeport manages the instance's namespace")

	// new image, the Service dropped, a ConfigMap added
	require.NoError(t, fixture.update(yamlDocument(
		fmt.Sprintf(webDeployment, 1, "nginx:1.27"),
		webConfigMap,
	)))

	t.Run("resource definitions follow the YAML document", func(t *testing.T) {
		var kinds []string
		for _, wrd := range fixture.api.wrds {
			object := &unstructured.Unstructured{}
			require.NoError(t, object.UnmarshalJSON(*wrd.JSONDefinition))
			kinds = append(kinds, object.GetKind())
			if object.GetKind() == "Deployment" {
				image, _, _ := unstructured.NestedSlice(object.Object, "spec", "template", "spec", "containers")
				assert.Equal(t, "nginx:1.27", image[0].(map[string]interface{})["image"])
			}
		}
		assert.ElementsMatch(t, []string{"Deployment", "ConfigMap"}, kinds)
	})

	t.Run("a running instance gets the change and is marked for update", func(t *testing.T) {
		resources := fixture.resources(t, fixture.plain)

		deployment, found := resources["apps/Deployment/"+plainNamespace+"/web"]
		require.True(t, found, "the deployment keeps its namespace and its resource instance")
		assert.False(t, *deployment.Reconciled, "left unreconciled so the instance applies it")
		containers, _, _ := unstructured.NestedSlice(resourceObject(t, deployment).Object, "spec", "template", "spec", "containers")
		assert.Equal(t, "nginx:1.27", containers[0].(map[string]interface{})["image"])

		configMap, found := resources["/ConfigMap/"+plainNamespace+"/web"]
		require.True(t, found, "the added resource is created in the instance's namespace")
		assert.False(t, *configMap.Reconciled)

		_, leaving := resources["leaving:/Service/"+plainNamespace+"/web"]
		assert.True(t, leaving, "the dropped resource is scheduled for deletion")

		namespace := resources["/Namespace//"+plainNamespace]
		assert.True(t, *namespace.Reconciled, "the namespace was not touched")

		assert.False(t, *fixture.api.instances[fixture.plain].Reconciled)
	})

	t.Run("an overlaid instance gets the change with its overlay still applied", func(t *testing.T) {
		var deployment *unstructured.Unstructured
		for key, wri := range fixture.resources(t, fixture.overlaid) {
			if strings.HasPrefix(key, "apps/Deployment/") {
				deployment = resourceObject(t, wri)
			}
		}
		require.NotNil(t, deployment)
		containers, _, _ := unstructured.NestedSlice(deployment.Object, "spec", "template", "spec", "containers")
		container := containers[0].(map[string]interface{})
		assert.Equal(t, "nginx:1.27", container["image"])
		assert.Equal(t, []interface{}{map[string]interface{}{"name": "TIER", "value": "canary"}}, container["env"])
		assert.False(t, *fixture.api.instances[fixture.overlaid].Reconciled)
	})

	t.Run("instances not deployed or being deleted are left alone", func(t *testing.T) {
		assert.Empty(t, fixture.resources(t, fixture.undeployed))
		assert.True(t, *fixture.api.instances[fixture.undeployed].Reconciled)
		assert.True(t, *fixture.api.instances[fixture.deleting].Reconciled)
		for _, write := range fixture.api.writes {
			assert.NotContains(t, write, fmt.Sprintf("/%d", fixture.undeployed))
			assert.NotContains(t, write, fmt.Sprintf("/%d", fixture.deleting))
		}
	})

	t.Run("running the update again writes nothing", func(t *testing.T) {
		fixture.api.writes = nil
		require.NoError(t, fixture.update(fixture.definition.YAMLDocument))
		assert.Empty(t, fixture.api.writes)
	})
}

// TestDefinitionUpdateInterruptedIsFinishedByTheRetry covers an update that
// fails part way through, which the reconciler retries.
//
// The resource definitions are what tells the retry there is anything to do.
// Were they written first, an update failing after them would leave running
// instances on the old resources while every retry found nothing changed.
func TestDefinitionUpdateInterruptedIsFinishedByTheRetry(t *testing.T) {
	fixture := newDefinitionUpdateFixture(t, yamlDocument(fmt.Sprintf(webDeployment, 1, "nginx:1.26"), webService))
	updated := yamlDocument(fmt.Sprintf(webDeployment, 1, "nginx:1.27"), webService)

	fixture.api.failWrite = v0.PathKubernetesWorkloadInstances + "/"
	require.Error(t, fixture.update(updated))
	assert.True(t, *fixture.api.instances[fixture.plain].Reconciled, "the failed write did not land")

	fixture.api.failWrite = ""
	require.NoError(t, fixture.update(updated))
	assert.False(t, *fixture.api.instances[fixture.plain].Reconciled, "the retry marked the instance for update")
	assert.False(t, *fixture.api.instances[fixture.overlaid].Reconciled)
}

// TestDefinitionUpdateWithoutYAMLChangeWritesNothing covers every other write
// to a definition - each one notifies the controller too.
func TestDefinitionUpdateWithoutYAMLChangeWritesNothing(t *testing.T) {
	yaml := yamlDocument(fmt.Sprintf(webDeployment, 1, "nginx:1.26"), webService)
	fixture := newDefinitionUpdateFixture(t, yaml)

	require.NoError(t, fixture.update(yaml))
	assert.Empty(t, fixture.api.writes)
}

// TestDefinitionUpdatesThatCannotBeFollowedAreRefused covers the YAML changes
// running instances cannot follow: each is reported on the definition, and
// nothing is written.
func TestDefinitionUpdatesThatCannotBeFollowedAreRefused(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		yaml    *string
		message string
	}{
		{
			name: "moving resources into a namespace of their own",
			yaml: yamlDocument(
				`{"apiVersion":"v1","kind":"Namespace","metadata":{"name":"web"}}`,
				fmt.Sprintf(webDeployment, 1, "nginx:1.26"),
			),
			message: "changes the namespace",
		},
		{
			name:    "declaring no resources at all",
			yaml:    yamlDocument(""),
			message: "declares no resources",
		},
		{
			name:    "declaring a resource twice",
			yaml:    yamlDocument(webService, webService),
			message: "more than once",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newDefinitionUpdateFixture(t, yamlDocument(fmt.Sprintf(webDeployment, 1, "nginx:1.26"), webService))

			err := fixture.update(testCase.yaml)
			require.Error(t, err)
			assert.Contains(t, err.Error(), testCase.message)
			var withEvent *tp_errors.ErrWithEvent
			assert.True(t, errors.As(err, &withEvent), "recorded as an event on the definition")
			assert.Empty(t, fixture.api.writes)
		})
	}
}
