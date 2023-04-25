package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/errors"

	"github.com/threeport/threeport/internal/kube"
	"github.com/threeport/threeport/internal/threeport"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
)

const (
	apiToken = ""
)

// testWorkload represents a test case for this e2e test.
type testWorkload struct {
	Name             string
	ManagedNamespace bool
	Resources        []kubeResource
}

// kubeResource contains the values needed to create and retrieve resources from
// the Kubernetes API for this test.
type kubeResource struct {
	Group     string
	Version   string
	Kind      string
	Namespace string
	Name      string
	Manifest  string
}

// TestWorkloadE2E tests that workload creation and deletgion works as expected.
func TestWorkloadE2E(t *testing.T) {
	assert := assert.New(t)
	testWorkloads := testResources()

	for _, testWorkload := range *testWorkloads {
		// create workload definition
		workloadDefName := testWorkload.Name
		var workloadDefYAML string
		for _, r := range testWorkload.Resources {
			workloadDefYAML = workloadDefYAML + r.Manifest
		}
		workloadDef := v0.WorkloadDefinition{
			Definition: v0.Definition{
				Name: &workloadDefName,
			},
			YAMLDocument: &workloadDefYAML,
		}
		createdWorkloadDef, err := client.CreateWorkloadDefinition(
			&workloadDef,
			apiAddr(),
			apiToken,
		)
		assert.Nil(err, "should have no error creating workload definition")

		if assert.NotNil(createdWorkloadDef, "should have a workload definition returned") {
			assert.NotNil(createdWorkloadDef.ID, "created workload definition should contain unique ID")
			assert.NotNil(createdWorkloadDef.CreatedAt, "created workload definition should contain created timestamp")
			assert.NotNil(createdWorkloadDef.UpdatedAt, "created workload definition should contain updated timestamp")
			assert.Equal(*createdWorkloadDef.Name, workloadDefName, "created workload definition should contain the name we gave it")
			assert.Equal(*createdWorkloadDef.YAMLDocument, workloadDefYAML, "created workload definition should contain the YAML document we provided")
			assert.Equal(*createdWorkloadDef.Reconciled, false, "created workload definition should not be reconciled at creation time")
		}

		// check to make sure workload definition gets reconciled by workload
		// controller
		workloadDefChecks := 0
		workloadDefMaxChecks := 60
		workloadDefCheckDurationSeconds := 1
		reconciled := false
		var existingWorkloadDef *v0.WorkloadDefinition
		for workloadDefChecks < workloadDefMaxChecks && !reconciled {
			existingWorkloadDef, err = client.GetWorkloadDefinitionByID(
				*createdWorkloadDef.ID,
				apiAddr(),
				apiToken,
			)
			assert.Nil(err, "should have no error getting workload definition by ID")
			if *existingWorkloadDef.Reconciled {
				reconciled = true
				break
			}
			workloadDefChecks += 1
			time.Sleep(time.Duration(workloadDefCheckDurationSeconds * 1000000000))
		}
		assert.Equal(*existingWorkloadDef.Reconciled, true, fmt.Sprintf("created workload definition should be reconciled by workload controller after %d seconds", workloadDefMaxChecks*workloadDefCheckDurationSeconds))

		// check workload resource definitions
		workloadResourceDefs, err := client.GetWorkloadResourceDefinitionsByWorkloadDefinitionID(
			*createdWorkloadDef.ID,
			apiAddr(),
			apiToken,
		)
		assert.Nil(err, "should have no error getting workload resource definitions")

		if assert.NotNil(workloadResourceDefs, "should have an array of workload resource definitions returned") {
			assert.Equal(len(*workloadResourceDefs), len(testWorkload.Resources), "should get back the right number of workload resource definitions")
			for _, wrd := range *workloadResourceDefs {
				resourceFound := false
				assert.NotNil(wrd.ID, "created workload resource definition should contain unique ID")
				assert.NotNil(wrd.CreatedAt, "created workload resource definition should contain created timestamp")
				assert.NotNil(wrd.UpdatedAt, "created workload resource definition should contain updated timestamp")
				assert.Equal(wrd.WorkloadDefinitionID, createdWorkloadDef.ID, "created workload resource definition should be associated to correct workload definition")
				for _, resource := range testWorkload.Resources {
					if strings.Contains(string(*wrd.JSONDefinition), resource.Kind) {
						resourceFound = true
					}
				}
				assert.Equal(resourceFound, true, "should have workload resource definition with JSON definition for kubernetes resource")
			}
		}

		// check cluster instance
		clusterInsts, err := client.GetClusterInstances(apiAddr(), apiToken)
		assert.Nil(err, "should have no error getting workload resource definitions")
		var testClusterInst v0.ClusterInstance
		if assert.NotNil(clusterInsts, "should have an array of cluster instances returned") {
			assert.NotEqual(len(*clusterInsts), 0, "should get back at least one cluster instance")
			for _, c := range *clusterInsts {
				if *c.ThreeportControlPlaneCluster {
					testClusterInst = c
				}
			}
		}
		assert.NotNil(testClusterInst, "should have a cluster instance being used by threeport control plane")

		// create workload instance
		workloadInstName := fmt.Sprintf("%s-0", testWorkload.Name)
		workloadInst := v0.WorkloadInstance{
			Instance: v0.Instance{
				Name: &workloadInstName,
			},
			ClusterInstanceID:    testClusterInst.ID,
			WorkloadDefinitionID: createdWorkloadDef.ID,
		}
		createdWorkloadInst, err := client.CreateWorkloadInstance(
			&workloadInst,
			apiAddr(),
			apiToken,
		)
		assert.Nil(err, "should have no error creating workload instance")
		assert.NotNil(createdWorkloadInst, "should have a workload instance returned")

		// get the cluster instance from the threeport API so we can connect to it
		clusterInstance, err := client.GetClusterInstanceByID(
			*testClusterInst.ID,
			apiAddr(),
			apiToken,
		)
		assert.Nil(err, "should have no error getting cluster instance")
		assert.NotNil(clusterInstance, "should have a cluster instance returned")

		// create a client to connect to kube API
		dynamicKubeClient, mapper, err := kube.GetClient(clusterInstance, false)
		assert.Nil(err, "should have no error creating a client and REST mapper for Kubernetes cluster API")

		// for the managed namespace test, get the namespace name
		if testWorkload.ManagedNamespace {
			getNSAttempts := 0
			getNSAttemptsMax := 5
			getNSDurationSeconds := 1
			managedNSFound := false
			for getNSAttempts < getNSAttemptsMax {
				managedNamespaceNames, err := kube.GetManagedNamespaceNames(dynamicKubeClient)
				assert.Nil(err, "should have no error getting managed namespace name")
				if len(managedNamespaceNames) < 1 {
					// not found yet, check again in getNSDurationSeconds
					getNSAttempts += 1
					time.Sleep(time.Duration(getNSDurationSeconds * 1000000000))
					continue
				}
				managedNSFound = true
				for i, _ := range testWorkload.Resources {
					testWorkload.Resources[i].Namespace = managedNamespaceNames[0]
				}
				break
			}
			assert.Equal(managedNSFound, true, fmt.Sprintf("should have found managed namespace in Kubernetes after %d seconds", getNSAttemptsMax*getNSDurationSeconds))
		}

		// check kube cluster for expected resources
		allResourcesFound := false
		findAttempts := 0
		findAttemptsMax := 30
		findCheckDurationSeconds := 1
		for findAttempts < findAttemptsMax {
			resourcesFound := 0
			for _, r := range testWorkload.Resources {
				_, err := kube.GetResource(
					r.Group,
					r.Version,
					r.Kind,
					r.Namespace,
					r.Name,
					dynamicKubeClient,
					*mapper,
				)
				if err != nil {
					break
				}
				resourcesFound += 1
			}
			if resourcesFound == len(testWorkload.Resources) {
				allResourcesFound = true
				break
			}
			findAttempts += 1
			time.Sleep(time.Duration(findCheckDurationSeconds * 1000000000))
		}
		assert.Equal(allResourcesFound, true, fmt.Sprintf("should have found all resources in Kubernetes after %d seconds", findAttemptsMax*findCheckDurationSeconds))

		// attempt deleting workload definition - should fail with instance still in
		// place
		_, err = client.DeleteWorkloadDefinition(
			*createdWorkloadDef.ID,
			apiAddr(),
			apiToken,
		)
		assert.NotNil(err, "should have an error returned when trying to delete workload definition with workload instance still in place")

		// delete workload instance
		deletedWorkloadInst, err := client.DeleteWorkloadInstance(
			*createdWorkloadInst.ID,
			apiAddr(),
			apiToken,
		)
		assert.Nil(err, "should have no error deleting workload instance")

		// make sure there are zero workload instances in system
		workloadInsts, err := client.GetWorkloadInstances(
			apiAddr(),
			apiToken,
		)
		assert.Nil(err, "should have no errors geting all workload instances")
		if assert.NotNil(workloadInsts, "should have an array of workload instances returned") {
			for _, wi := range *workloadInsts {
				assert.NotEqual(wi.ID, deletedWorkloadInst.ID, "should not get back deleted workload instance when retrieving all workload instances")
			}
		}

		// check to make sure kube resources are gone
		allResourcesGone := false
		goneAttempts := 0
		goneAttemptsMax := 15
		goneCheckDurationSeconds := 1
		for goneAttempts < goneAttemptsMax {
			resourcesGone := 0
			for _, r := range testWorkload.Resources {
				resource, err := kube.GetResource(
					r.Group,
					r.Version,
					r.Kind,
					r.Namespace,
					r.Name,
					dynamicKubeClient,
					*mapper,
				)
				// if we get resource back, it's not yet gone
				if resource != nil {
					break
				}
				// if we get an error that is NOT a "not found" error we have a
				// problem - log rather than exit in case it resolves
				if err != nil && !errors.IsNotFound(err) {
					t.Log(fmt.Errorf("an error occured that was NOT a \"not found\" error: %w", err))
					break
				}
				resourcesGone += 1
			}
			if resourcesGone == len(testWorkload.Resources) {
				allResourcesGone = true
				break
			}
			goneAttempts += 1
			time.Sleep(time.Duration(goneCheckDurationSeconds * 1000000000))
		}
		assert.Equal(allResourcesGone, true, fmt.Sprintf("should have found that all resources are gone from Kubernetes after %d seconds", goneAttemptsMax*goneCheckDurationSeconds))

		// delete workload definition
		deletedWorkloadDef, err := client.DeleteWorkloadDefinition(
			*createdWorkloadDef.ID,
			apiAddr(),
			apiToken,
		)
		assert.Nil(err, "should have no error deleting workload definition")

		// make sure the workload definition is gone
		workloadDefs, err := client.GetWorkloadDefinitions(
			apiAddr(),
			apiToken,
		)
		assert.Nil(err, "should have no errors geting all workload definitions")
		if assert.NotNil(workloadDefs, "should have an array of workload definitions returned") {
			for _, wd := range *workloadDefs {
				assert.NotEqual(wd.ID, deletedWorkloadDef.ID, "should not get back deleted workload definition when retrieving all workload definitions")
			}
		}
	}
}

// apiAddr returns the address of a local instance of threeport API.
func apiAddr() string {
	return fmt.Sprintf(
		"%s://%s:%s",
		threeport.ThreeportLocalAPIProtocol,
		threeport.ThreeportLocalAPIEndpoint,
		threeport.ThreeportLocalAPIPort,
	)
}

// testResources returns the test workloads for this test.
// func testResources() *[]kubeResource {
func testResources() *[]testWorkload {
	tests := []testWorkload{
		{
			Name:             "unmanaged-namespace-workload",
			ManagedNamespace: false,
			Resources: []kubeResource{
				{
					Group:     "",
					Version:   "v1",
					Kind:      "Namespace",
					Namespace: "",
					Name:      "go-web3-sample-app-0",
					Manifest:  workloadDefNamespace,
				},
				{
					Group:     "",
					Version:   "v1",
					Kind:      "ConfigMap",
					Namespace: "go-web3-sample-app-0",
					Name:      "go-web3-sample-app-config",
					Manifest:  workloadDefConfigMap,
				},
				{
					Group:     "apps",
					Version:   "v1",
					Kind:      "Deployment",
					Namespace: "go-web3-sample-app-0",
					Name:      "go-web3-sample-app",
					Manifest:  workloadDefDeployment,
				},
				{
					Group:     "",
					Version:   "v1",
					Kind:      "Service",
					Namespace: "go-web3-sample-app-0",
					Name:      "go-web3-sample-app",
					Manifest:  workloadDefService,
				},
			},
		},
		{
			Name:             "managed-namespace-workload",
			ManagedNamespace: true,
			Resources: []kubeResource{
				{
					Group:    "",
					Version:  "v1",
					Kind:     "ConfigMap",
					Name:     "go-web3-sample-app-config",
					Manifest: workloadDefConfigMapMinusNamespace,
				},
				{
					Group:    "apps",
					Version:  "v1",
					Kind:     "Deployment",
					Name:     "go-web3-sample-app",
					Manifest: workloadDefDeploymentMinusNamespace,
				},
				{
					Group:    "",
					Version:  "v1",
					Kind:     "Service",
					Name:     "go-web3-sample-app",
					Manifest: workloadDefServiceMinusNamespace,
				},
			},
		},
	}

	return &tests
}

const workloadDefNamespace = `---
apiVersion: v1
kind: Namespace
metadata:
  name: go-web3-sample-app-0
`

const workloadDefConfigMap = `---
apiVersion: v1
kind: ConfigMap
metadata:
  name: go-web3-sample-app-config
  namespace: go-web3-sample-app-0
data:
  RPCENDPOINT: http://forward-proxy.forward-proxy-system.svc.cluster.local
`

const workloadDefDeployment = `---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: go-web3-sample-app
  namespace: go-web3-sample-app-0
spec:
  selector:
    matchLabels:
      app: web3-sample-app
  template:
    metadata:
      labels:
        app: web3-sample-app
    spec:
      containers:
        - name: web3-sample-app
          image: ghcr.io/qleet/go-web3-sample-app:v0.0.4
          env:
            - name: PORT
              value: '8080'
            - name: RPCENDPOINT
              valueFrom:
                configMapKeyRef:
                  name: go-web3-sample-app-config
                  key: RPCENDPOINT
          ports:
            - containerPort: 8080
      restartPolicy: Always
`

const workloadDefService = `---
apiVersion: v1
kind: Service
metadata:
  name: go-web3-sample-app
  namespace: go-web3-sample-app-0
spec:
  ports:
    - port: 8080
      targetPort: 8080
  type: ClusterIP
  selector:
    app: web3-sample-app
`

const workloadDefConfigMapMinusNamespace = `---
apiVersion: v1
kind: ConfigMap
metadata:
  name: go-web3-sample-app-config
data:
  RPCENDPOINT: http://forward-proxy.forward-proxy-system.svc.cluster.local
`

const workloadDefDeploymentMinusNamespace = `---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: go-web3-sample-app
spec:
  selector:
    matchLabels:
      app: web3-sample-app
  template:
    metadata:
      labels:
        app: web3-sample-app
    spec:
      containers:
        - name: web3-sample-app
          image: ghcr.io/qleet/go-web3-sample-app:v0.0.4
          env:
            - name: PORT
              value: '8080'
            - name: RPCENDPOINT
              valueFrom:
                configMapKeyRef:
                  name: go-web3-sample-app-config
                  key: RPCENDPOINT
          ports:
            - containerPort: 8080
      restartPolicy: Always
`

const workloadDefServiceMinusNamespace = `---
apiVersion: v1
kind: Service
metadata:
  name: go-web3-sample-app
  namespace: not-used
spec:
  ports:
    - port: 8080
      targetPort: 8080
  type: ClusterIP
  selector:
    app: web3-sample-app
`
