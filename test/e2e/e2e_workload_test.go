package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gorm.io/datatypes"
	kubeerrors "k8s.io/apimachinery/pkg/api/errors"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	v1 "github.com/threeport/threeport/pkg/api/v1"
	cli "github.com/threeport/threeport/pkg/cli/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	client_v1 "github.com/threeport/threeport/pkg/client/v1"
	config "github.com/threeport/threeport/pkg/config/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
	threeport "github.com/threeport/threeport/pkg/threeport-installer/v0"
	"github.com/threeport/threeport/pkg/threeport-installer/v0/tptdev"
	util "github.com/threeport/threeport/pkg/util/v0"
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
		t.Log(fmt.Sprintf(
			"testing workload: %s\n", testWorkload.Name,
		))

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

		// create a duplicate workload definition
		duplicateWorkload := v0.WorkloadDefinition{
			Definition: v0.Definition{
				Name: &workloadDefName,
			},
			YAMLDocument: util.StringPtr(""),
		}

		// determine if the API is serving HTTPS or HTTP
		var authEnabled bool
		var respCodeErr error
		var resp *http.Response
		// check HTTPS
		if err := util.Retry(10, 1, func() error {
			var err error
			resp, err = http.Get(fmt.Sprintf("http://localhost:%d/version", 443))
			// if we get an error, check HTTP
			if err != nil {
				resp, err = http.Get(fmt.Sprintf("http://localhost:%d/version", 80))
				if err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			assert.Nil(err, "should not get an error when calling threeport API version endpoint")
		}
		switch resp.StatusCode {
		case 400:
			authEnabled = true
			t.Log("auth enabled")
		case 200:
			authEnabled = false
			t.Log("auth not enabled")
		default:
			respCodeErr = fmt.Errorf("unexpected response from API (not 200 or 400): %d", resp.StatusCode)
		}
		assert.Nil(respCodeErr, "should not get an get an unexpected response from API version endpoint")

		threeportAPIEndpoint := threeport.GetLocalThreeportAPIEndpoint(authEnabled)

		// initialize config so we can pull credentials from it
		cli.InitConfig("")

		// get threeport config and configure http client for calls to threeport API
		threeportConfig, _, err := config.GetThreeportConfig("")
		assert.Nil(err, "should have no error getting threeport config")
		apiClient, err := threeportConfig.GetHTTPClient(tptdev.DefaultInstanceName)
		assert.Nil(err, "should have no error creating http client")

		// configure domain name definition object
		domainNameDefinition := &v0.DomainNameDefinition{
			Definition: v0.Definition{
				Name: util.StringPtr("domainNameDefinition"),
			},
			Domain:     util.StringPtr("test.threeport.io"),
			Zone:       util.StringPtr("testZone"),
			AdminEmail: util.StringPtr("no-reply@threeport.io"),
		}

		// create domain name definition
		createdDomainNameDefinition, err := client.CreateDomainNameDefinition(
			apiClient,
			threeportAPIEndpoint,
			domainNameDefinition,
		)
		assert.Nil(err, "should have no error creating domain name definition")

		// configure gateway definition object
		gatewayDefinition := &v0.GatewayDefinition{
			Definition: v0.Definition{
				Name: util.StringPtr("gateway-definition"),
			},
			DomainNameDefinitionID: createdDomainNameDefinition.ID,
			HttpPorts: []*v0.GatewayHttpPort{
				{
					Port:       util.IntPtr(80),
					TLSEnabled: util.BoolPtr(false),
				},
				{
					Port:       util.IntPtr(443),
					TLSEnabled: util.BoolPtr(true),
				},
			},
			TcpPorts: []*v0.GatewayTcpPort{
				{
					Port:       util.IntPtr(22),
					TLSEnabled: util.BoolPtr(false),
				},
			},
		}

		// create gateway definition
		_, err = client.CreateGatewayDefinition(
			apiClient,
			threeportAPIEndpoint,
			gatewayDefinition,
		)
		assert.Nil(err, "should have no error creating gateway definition")

		// update gateway definition
		gatewayDefinition.HttpPorts = []*v0.GatewayHttpPort{
			{
				Port: util.IntPtr(443),
			},
		}
		_, err = client.UpdateGatewayDefinition(
			apiClient,
			threeportAPIEndpoint,
			gatewayDefinition,
		)
		assert.Nil(err, "should have no error updating gateway definition")

		// create secret data
		secretData := map[string]string{
			"username": "admin",
			"password": "password",
		}
		jsonData, err := json.Marshal(secretData)
		assert.Nil(err, "should have no error marshalling secret data")

		// create secret definition
		createdSecretDefinition, err := client.CreateSecretDefinition(
			apiClient,
			threeportAPIEndpoint,
			&v0.SecretDefinition{
				Definition: v0.Definition{
					Name: util.StringPtr("secret-definition"),
				},
				Data: util.Ptr(datatypes.JSON(jsonData)),
			},
		)
		assert.Nil(err, "should have no error creating secret definition")

		// create test workload definition
		createdWorkloadDef, err := client.CreateWorkloadDefinition(
			apiClient,
			threeportAPIEndpoint,
			&workloadDef,
		)
		assert.Nil(err, "should have no error creating workload definition")

		// ensure duplicate workload name throws error
		_, err = client.CreateWorkloadDefinition(
			apiClient,
			threeportAPIEndpoint,
			&duplicateWorkload,
		)
		assert.NotNil(err, "duplicate workload definition should throw error")

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
		workloadDefMaxChecks := 600
		workloadDefCheckDurationSeconds := 1
		reconciled := false
		var existingWorkloadDef *v0.WorkloadDefinition
		for workloadDefChecks < workloadDefMaxChecks && !reconciled {
			existingWorkloadDef, err = client.GetWorkloadDefinitionByID(
				apiClient,
				threeportAPIEndpoint,
				*createdWorkloadDef.ID,
			)
			assert.Nil(err, "should have no error getting workload definition by ID")
			if *existingWorkloadDef.Reconciled {
				reconciled = true
				break
			}
			workloadDefChecks += 1
			time.Sleep(time.Second * time.Duration(workloadDefCheckDurationSeconds))
		}
		assert.Equal(*existingWorkloadDef.Reconciled, true, fmt.Sprintf("created workload definition should be reconciled by workload controller after %d seconds", workloadDefMaxChecks*workloadDefCheckDurationSeconds))

		// check workload resource definitions
		workloadResourceDefs, err := client.GetWorkloadResourceDefinitionsByWorkloadDefinitionID(
			apiClient,
			threeportAPIEndpoint,
			*createdWorkloadDef.ID,
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

		// check kubernetes runtime instance
		kubernetesRuntimeInsts, err := client.GetKubernetesRuntimeInstances(apiClient, threeportAPIEndpoint)
		assert.Nil(err, "should have no error getting workload resource definitions")
		var testKubernetesRuntimeInst v0.KubernetesRuntimeInstance
		if assert.NotNil(kubernetesRuntimeInsts, "should have an array of kubernetes runtime instances returned") {
			assert.NotEqual(len(*kubernetesRuntimeInsts), 0, "should get back at least one kubernetes runtime instance")
			for _, c := range *kubernetesRuntimeInsts {
				if *c.ThreeportControlPlaneHost {
					testKubernetesRuntimeInst = c
				}
			}
		}
		assert.NotNil(testKubernetesRuntimeInst, "should have a kubernetes runtime instance being used by threeport control plane")

		// create workload instance
		workloadInstName := fmt.Sprintf("%s-0", testWorkload.Name)
		workloadInst := v1.WorkloadInstance{
			Instance: v0.Instance{
				Name: &workloadInstName,
			},
			KubernetesRuntimeInstanceID: testKubernetesRuntimeInst.ID,
			WorkloadDefinitionID:        createdWorkloadDef.ID,
		}
		createdWorkloadInst, err := client_v1.CreateWorkloadInstance(
			apiClient,
			threeportAPIEndpoint,
			&workloadInst,
		)
		assert.Nil(err, "should have no error creating workload instance")
		assert.NotNil(createdWorkloadInst, "should have a workload instance returned")

		// create a duplicate workload instance
		duplicateWorkloadInst := v1.WorkloadInstance{
			Instance: v0.Instance{
				Name: &workloadInstName,
			},
			KubernetesRuntimeInstanceID: testKubernetesRuntimeInst.ID,
			WorkloadDefinitionID:        createdWorkloadDef.ID,
		}

		_, err = client_v1.CreateWorkloadInstance(
			apiClient,
			threeportAPIEndpoint,
			&duplicateWorkloadInst,
		)
		assert.NotNil(err, "duplicate workload instance should throw error")

		// create secret instance
		// _, err = client.CreateSecretInstance(
		// 	apiClient,
		// 	threeportAPIEndpoint,
		// 	&v0.SecretInstance{
		// 		Instance: v0.Instance{
		// 			Name: util.Ptr("secret-instance"),
		// 		},
		// 		SecretDefinitionID:          createdSecretDefinition.ID,
		// 		WorkloadInstanceID:          createdWorkloadInst.ID,
		// 		KubernetesRuntimeInstanceID: testKubernetesRuntimeInst.ID,
		// 	},
		// )
		// assert.Nil(err, "should have no error creating secret instance")

		// configure domain name instance
		domainNameInstance := &v0.DomainNameInstance{
			Instance: v0.Instance{
				Name: &workloadInstName,
			},
			DomainNameDefinitionID:      domainNameDefinition.ID,
			WorkloadInstanceID:          createdWorkloadInst.ID,
			KubernetesRuntimeInstanceID: testKubernetesRuntimeInst.ID,
		}

		// create domain name instance
		_, err = client.CreateDomainNameInstance(
			apiClient,
			threeportAPIEndpoint,
			domainNameInstance,
		)
		assert.Nil(err, "should have no error creating domain name instance")

		// create a gateway instance
		gatewayInstance := &v0.GatewayInstance{
			Instance: v0.Instance{
				Name: util.StringPtr("gatewayInstance"),
			},
			KubernetesRuntimeInstanceID: testKubernetesRuntimeInst.ID,
			GatewayDefinitionID:         gatewayDefinition.ID,
			WorkloadInstanceID:          createdWorkloadInst.ID,
		}
		_, err = client.CreateGatewayInstance(
			apiClient,
			threeportAPIEndpoint,
			gatewayInstance,
		)
		assert.Nil(err, "should have no error creating gateway instance")

		// get the kubernetes runtime instance from the threeport API so we can connect to it
		kubernetesRuntimeInstance, err := client.GetKubernetesRuntimeInstanceByID(
			apiClient,
			threeportAPIEndpoint,
			*testKubernetesRuntimeInst.ID,
		)
		assert.Nil(err, "should have no error getting kubernetes runtime instance")
		assert.NotNil(kubernetesRuntimeInstance, "should have a kubernetes runtime instance returned")

		encryptionKey, err := threeportConfig.GetEncryptionKey(tptdev.DefaultInstanceName)
		assert.Nil(err, "should have no error getting encryption key")

		// create a client to connect to kube API
		dynamicKubeClient, mapper, err := kube.GetClient(
			kubernetesRuntimeInstance,
			false,
			apiClient,
			threeportAPIEndpoint,
			encryptionKey,
		)
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
					time.Sleep(time.Second * time.Duration(getNSDurationSeconds))
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
		findAttemptsMax := 60
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
			time.Sleep(time.Second * time.Duration(findCheckDurationSeconds))
		}
		assert.Equal(allResourcesFound, true, fmt.Sprintf("should have found all resources in Kubernetes after %d seconds", findAttemptsMax*findCheckDurationSeconds))

		// check threeport API for expected WorkloadEvents
		startedEventFound := false
		eventAttempts := 0
		eventAttemptsMax := 300
		eventCheckDurationSeconds := 1
		for eventAttempts < eventAttemptsMax {
			workloadEvents, err := client.GetWorkloadEventsByQueryString(
				apiClient,
				threeportAPIEndpoint,
				fmt.Sprintf("workloadinstanceid=%d", *createdWorkloadInst.ID),
			)
			assert.Nil(err, "should have no error returned when trying to retrieve workload events for workload instance")
			for _, event := range *workloadEvents {
				if *event.Type == "Normal" && *event.Reason == "Started" {
					startedEventFound = true
					break
				}
			}
			if startedEventFound {
				break
			}
			eventAttempts += 1
			time.Sleep(time.Second * time.Duration(eventCheckDurationSeconds))
		}
		assert.Equal(startedEventFound, true, fmt.Sprintf("should have found all container started events in Kubernetes after %d seconds", eventAttemptsMax*eventCheckDurationSeconds))

		// attempt deleting workload definition - should fail with instance still in
		// place
		_, err = client.DeleteWorkloadDefinition(
			apiClient,
			threeportAPIEndpoint,
			*createdWorkloadDef.ID,
		)
		assert.NotNil(err, "should have an error returned when trying to delete workload definition with workload instance still in place")

		// delete workload instance
		deletedWorkloadInst, err := client.DeleteWorkloadInstance(
			apiClient,
			threeportAPIEndpoint,
			*createdWorkloadInst.ID,
		)
		assert.Nil(err, "should have no error deleting workload instance")

		// wait for workload deletion to be reconciled
		deletedCheckAttempts := 0
		deletedCheckAttemptsMax := 90
		deletedCheckDurationSeconds := 1
		workloadInstanceDeleted := false
		for deletedCheckAttempts < deletedCheckAttemptsMax {
			_, err := client.GetWorkloadInstanceByID(apiClient, threeportAPIEndpoint, *createdWorkloadInst.ID)
			if err != nil {
				if errors.Is(err, client.ErrObjectNotFound) {
					workloadInstanceDeleted = true
					break
				}
			}
			// no error means workload instance was found - hasn't yet been deleted
			deletedCheckAttempts += 1
			time.Sleep(time.Second * time.Duration(deletedCheckDurationSeconds))
		}
		assert.True(workloadInstanceDeleted, fmt.Sprintf("should have found that workload instance was deleted after %d seconds", deletedCheckAttemptsMax*deletedCheckDurationSeconds))

		// make sure there are zero workload instances in system
		workloadInsts, err := client.GetWorkloadInstances(
			apiClient,
			threeportAPIEndpoint,
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
		goneAttemptsMax := 30
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
				if err != nil && !kubeerrors.IsNotFound(err) {
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
			time.Sleep(time.Second * time.Duration(goneCheckDurationSeconds))
		}
		assert.Equal(allResourcesGone, true, fmt.Sprintf("should have found that all resources are gone from Kubernetes after %d seconds", goneAttemptsMax*goneCheckDurationSeconds))

		// delete gateway definition
		deletedAttempts := 0
		deletedAttemptsMax := 10
		deletedCheckDurationSeconds = 1
		for deletedAttempts < deletedAttemptsMax {
			_, err = client.DeleteGatewayDefinition(
				apiClient,
				threeportAPIEndpoint,
				*gatewayDefinition.ID,
			)

			// workload controller may not have deleted the gateway
			// instance yet. If so, wait and try again
			if err != nil {
				deletedAttempts += 1
				time.Sleep(time.Second * time.Duration(deletedCheckDurationSeconds))
				continue
			}
			break
		}
		assert.Nil(err, "should have no error deleting gateway definition")

		// delete domain name definition
		deletedAttempts = 0
		for deletedAttempts < deletedAttemptsMax {
			_, err = client.DeleteDomainNameDefinition(
				apiClient,
				threeportAPIEndpoint,
				*domainNameDefinition.ID,
			)

			// workload controller may not have deleted the gateway
			// instance yet. If so, wait and try again
			if err != nil {
				deletedAttempts += 1
				time.Sleep(time.Duration(deletedCheckDurationSeconds * 1000000000))
				continue
			}
			break
		}
		assert.Nil(err, "should have no error deleting domain name definition")

		// delete secret definition
		_, err = client.DeleteSecretDefinition(
			apiClient,
			threeportAPIEndpoint,
			*createdSecretDefinition.ID,
		)
		assert.Nil(err, "should have no error deleting secret definition")

		// delete workload definition
		deletedWorkloadDef, err := client.DeleteWorkloadDefinition(
			apiClient,
			threeportAPIEndpoint,
			*createdWorkloadDef.ID,
		)
		assert.Nil(err, "should have no error deleting workload definition")

		// give the API server time to process deletion
		time.Sleep(time.Second * 3)

		// make sure the workload definition is gone
		workloadDefs, err := client.GetWorkloadDefinitions(
			apiClient,
			threeportAPIEndpoint,
		)
		assert.Nil(err, "should have no errors geting all workload definitions")
		if assert.NotNil(workloadDefs, "should have an array of workload definitions returned") {
			for _, wd := range *workloadDefs {
				assert.NotEqual(wd.ID, deletedWorkloadDef.ID, "should not get back deleted workload definition when retrieving all workload definitions")
			}
		}
	}
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
