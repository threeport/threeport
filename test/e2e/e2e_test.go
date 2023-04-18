package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/threeport/threeport/internal/threeport"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
)

const (
	apiToken = ""
)

// apiAddr returns the address of a local instance of threeport API.
func apiAddr() string {
	return fmt.Sprintf(
		"%s://%s",
		threeport.ThreeportLocalAPIProtocol,
		threeport.ThreeportLocalAPIEndpoint,
	)
}

// TestWorkload tests that workload creation works as expected.
func TestWorkload(t *testing.T) {
	assert := assert.New(t)

	// create workload definition
	workloadDefName := "test-workload"
	workloadDefYAML := "apiVersion: v1\nkind: Namespace\nmetadata:\n  name: go-web3-sample-app\n---\napiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: go-web3-sample-app-config\n  namespace: go-web3-sample-app\ndata:\n  RPCENDPOINT: http://forward-proxy.forward-proxy-system.svc.cluster.local\n---\napiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: go-web3-sample-app\n  namespace: go-web3-sample-app\nspec:\n  selector:\n    matchLabels:\n      app: web3-sample-app\n  template:\n    metadata:\n      labels:\n        app: web3-sample-app\n    spec:\n      containers:\n        - name: web3-sample-app\n          image: ghcr.io/qleet/go-web3-sample-app:v0.0.4\n          env:\n            - name: PORT\n              value: '8080'\n            - name: RPCENDPOINT\n              valueFrom:\n                configMapKeyRef:\n                  name: go-web3-sample-app-config\n                  key: RPCENDPOINT\n          ports:\n            - containerPort: 8080\n      restartPolicy: Always\n---\napiVersion: v1\nkind: Service\nmetadata:\n  name: go-web3-sample-app\n  namespace: go-web3-sample-app\nspec:\n  ports:\n    - port: 8080\n      targetPort: 8080\n  type: ClusterIP\n  selector:\n    app: web3-sample-app\n\n"
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

	// wait for workload reconciler to reconile workload defintion
	time.Sleep(time.Second * 3)

	// check workload resource definitions
	workloadResourceDefs, err := client.GetWorkloadResourceDefinitionsByWorkloadDefinitionID(
		*createdWorkloadDef.ID,
		apiAddr(),
		apiToken,
	)
	assert.Nil(err, "should have no error getting workload resource definitions")

	if assert.NotNil(workloadResourceDefs, "should have an array of workload resource definitions returned") {
		assert.Equal(len(*workloadResourceDefs), 4, "should get back 4 workload resource definitions")
		namespaceFound := false
		configmapFound := false
		deploymentFound := false
		serviceFound := false
		for _, wrd := range *workloadResourceDefs {
			assert.NotNil(wrd.ID, "created workload resource definition should contain unique ID")
			assert.NotNil(wrd.CreatedAt, "created workload resource definition should contain created timestamp")
			assert.NotNil(wrd.UpdatedAt, "created workload resource definition should contain updated timestamp")
			assert.Equal(wrd.WorkloadDefinitionID, createdWorkloadDef.ID, "created workload resource definition should be associated to correct workload definition")
			switch {
			case strings.Contains(string(*wrd.JSONDefinition), "Namespace"):
				namespaceFound = true
			case strings.Contains(string(*wrd.JSONDefinition), "ConfigMap"):
				configmapFound = true
			case strings.Contains(string(*wrd.JSONDefinition), "Deployment"):
				deploymentFound = true
			case strings.Contains(string(*wrd.JSONDefinition), "Service"):
				serviceFound = true
			}
		}
		assert.Equal(namespaceFound, true, "one of the workload resource definitions should contain a JSON definition for a namespace")
		assert.Equal(configmapFound, true, "one of the workload resource definitions should contain a JSON definition for a configmpa")
		assert.Equal(deploymentFound, true, "one of the workload resource definitions should contain a JSON definition for a deployment")
		assert.Equal(serviceFound, true, "one of the workload resource definitions should contain a JSON definition for a service")
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
	workloadInstName := "test-workload-0"
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

	// TODO: check kube cluster for expected resources

	// TODO: delete workload instance and definition then check results
}
