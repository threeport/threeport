package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	kubeerrors "k8s.io/apimachinery/pkg/api/errors"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	cli "github.com/threeport/threeport/pkg/cli/v0"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	encryption "github.com/threeport/threeport/pkg/encryption/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// retryDelete keeps invoking deleteFn until it succeeds, returns
// ErrObjectNotFound (already gone), or the 60s budget runs out. Test
// defers use it to ride out async AOR cleanup that runs after instance
// deletes — without the wait, definition deletes race the cleanup and
// fail with "still referenced", silently leaving orphans for the next
// run.
func retryDelete(t *testing.T, target string, deleteFn func() error) {
	// report failures at the deferred callsite instead of this helper.
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		err := deleteFn()
		if err == nil || errors.Is(err, client_lib.ErrObjectNotFound) {
			return
		}
		lastErr = err
		time.Sleep(time.Second)
	}
	t.Logf("cleanup: failed to delete %s within 60s: %v", target, lastErr)
}

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

// TestWorkloadIntegration tests that workload creation and deletgion works as expected
// when performed using the Threeport client library.
func TestWorkloadIntegration(t *testing.T) {
	assert := assert.New(t)
	testWorkloads := testResources()

	for _, testWorkload := range *testWorkloads {
		t.Logf("testing workload: %s\n", testWorkload.Name)

		// create workload definition
		workloadDefName := testWorkload.Name
		var workloadDefYAML string
		for _, r := range testWorkload.Resources {
			workloadDefYAML = workloadDefYAML + r.Manifest
		}
		workloadDef := v0.KubernetesWorkloadDefinition{
			Definition: v0.Definition{
				Name: &workloadDefName,
			},
			YAMLDocument: &workloadDefYAML,
		}

		// create a duplicate workload definition
		duplicateWorkload := v0.KubernetesWorkloadDefinition{
			Definition: v0.Definition{
				Name: &workloadDefName,
			},
			YAMLDocument: util.Ptr(""),
		}

		// initialize config so we can pull credentials from it
		cli.InitConfig(nil, "")

		// get threeport config and configure http client for calls to threeport API
		threeportConfig, _, err := cli.GetThreeportConfig("")
		require.Nil(t, err, "should have no error getting threeport config")
		apiClient, err := threeportConfig.GetHTTPClient(threeportConfig.CurrentControlPlane)
		require.Nil(t, err, "should have no error creating http client")

		// get Threeport API endpoint
		controlPlaneConfig, err := threeportConfig.GetControlPlaneConfig(threeportConfig.CurrentControlPlane)
		require.Nil(t, err, "should not get an error looking up Threeport API endpoint")
		threeportAPIEndpoint := controlPlaneConfig.APIServer

		// configure domain name definition object
		domainNameDefinition := &v0.DomainNameDefinition{
			Definition: v0.Definition{
				Name: util.Ptr("domainNameDefinition"),
			},
			Domain:     util.Ptr("test.threeport.io"),
			Zone:       util.Ptr("testZone"),
			AdminEmail: util.Ptr("no-reply@threeport.io"),
		}

		// create domain name definition
		createdDomainNameDefinition, err := client.CreateDomainNameDefinition(
			apiClient,
			threeportAPIEndpoint,
			domainNameDefinition,
		)
		require.Nil(t, err, "should have no error creating domain name definition")
		defer retryDelete(t, "domain name definition", func() error {
			_, err := client.DeleteDomainNameDefinition(apiClient, threeportAPIEndpoint, *createdDomainNameDefinition.ID)
			return err
		})

		// configure gateway definition object
		gatewayDefinition := &v0.GatewayDefinition{
			Definition: v0.Definition{
				Name: util.Ptr("gateway-definition"),
			},
			DomainNameDefinitionID: createdDomainNameDefinition.ID,
			HttpPorts: []*v0.GatewayHttpPort{
				{
					Port:       util.Ptr(80),
					TLSEnabled: util.Ptr(false),
				},
				{
					Port:       util.Ptr(443),
					TLSEnabled: util.Ptr(true),
				},
			},
			TcpPorts: []*v0.GatewayTcpPort{
				{
					Port:       util.Ptr(22),
					TLSEnabled: util.Ptr(false),
				},
			},
		}

		// create gateway definition
		createdGatewayDef, err := client.CreateGatewayDefinition(
			apiClient,
			threeportAPIEndpoint,
			gatewayDefinition,
		)
		require.Nil(t, err, "should have no error creating gateway definition")
		defer retryDelete(t, "gateway definition", func() error {
			_, err := client.DeleteGatewayDefinition(apiClient, threeportAPIEndpoint, *createdGatewayDef.ID)
			return err
		})

		// wait for the gateway controller to finish reconciling the new
		// definition before issuing the update. without this, the
		// controller's reconcile transaction (which sets Reconciled=true
		// on the row) races with the test's UPDATE transaction and
		// CockroachDB's serializable isolation rejects one of them with
		// a write-too-old error.
		err = util.Retry(30, 1, func() error {
			def, err := client.GetGatewayDefinitionByID(apiClient, threeportAPIEndpoint, *createdGatewayDef.ID)
			if err != nil {
				return err
			}
			if def.Reconciled == nil || !*def.Reconciled {
				return fmt.Errorf("gateway definition not yet reconciled")
			}
			return nil
		})
		assert.Nil(err, "gateway definition should be reconciled before update")

		// update gateway definition
		gatewayDefinition.HttpPorts = []*v0.GatewayHttpPort{
			{
				Port: util.Ptr(443),
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
					Name: util.Ptr("secret-definition"),
				},
				Data: util.Ptr(datatypes.JSON(jsonData)),
			},
		)
		require.Nil(t, err, "should have no error creating secret definition")
		defer retryDelete(t, "secret definition", func() error {
			_, err := client.DeleteSecretDefinition(apiClient, threeportAPIEndpoint, *createdSecretDefinition.ID)
			return err
		})

		// create test workload definition
		createdWorkloadDef, err := client.CreateKubernetesWorkloadDefinition(
			apiClient,
			threeportAPIEndpoint,
			&workloadDef,
		)
		require.Nil(t, err, "should have no error creating kubernetes workload definition")
		defer retryDelete(t, "kubernetes workload definition", func() error {
			_, err := client.DeleteKubernetesWorkloadDefinition(apiClient, threeportAPIEndpoint, *createdWorkloadDef.ID)
			return err
		})

		// ensure duplicate workload name throws error
		_, err = client.CreateKubernetesWorkloadDefinition(
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

		// wait for the workload controller to finish reconciling the new
		// definition. workload reconciliation takes longer than gateway
		// because the controller actually deploys resources to the
		// runtime, hence the 600s cap vs the shorter waits elsewhere.
		err = util.Retry(600, 1, func() error {
			def, err := client.GetKubernetesWorkloadDefinitionByID(apiClient, threeportAPIEndpoint, *createdWorkloadDef.ID)
			if err != nil {
				return err
			}
			if def.Reconciled == nil || !*def.Reconciled {
				return fmt.Errorf("workload definition not yet reconciled")
			}
			return nil
		})
		assert.Nil(err, "workload definition should be reconciled by workload controller")

		// check kubernetes workload resource definitions
		workloadResourceDefs, err := client.GetKubernetesWorkloadResourceDefinitionsByID(
			apiClient,
			threeportAPIEndpoint,
			*createdWorkloadDef.ID,
		)
		assert.Nil(err, "should have no error getting kubernetes workload resource definitions")

		if assert.NotNil(workloadResourceDefs, "should have an array of kubernetes workload resource definitions returned") {
			assert.Equal(len(*workloadResourceDefs), len(testWorkload.Resources), "should get back the right number of workload resource definitions")
			for _, wrd := range *workloadResourceDefs {
				resourceFound := false
				assert.NotNil(wrd.ID, "created workload resource definition should contain unique ID")
				assert.NotNil(wrd.CreatedAt, "created workload resource definition should contain created timestamp")
				assert.NotNil(wrd.UpdatedAt, "created workload resource definition should contain updated timestamp")
				assert.Equal(wrd.KubernetesWorkloadDefinitionID, createdWorkloadDef.ID, "created workload resource definition should be associated to correct workload definition")
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
		workloadInst := v0.KubernetesWorkloadInstance{
			Instance: v0.Instance{
				Name: &workloadInstName,
			},
			KubernetesRuntimeInstanceID:    testKubernetesRuntimeInst.ID,
			KubernetesWorkloadDefinitionID: createdWorkloadDef.ID,
		}
		createdWorkloadInst, err := client.CreateKubernetesWorkloadInstance(
			apiClient,
			threeportAPIEndpoint,
			&workloadInst,
		)
		require.Nil(t, err, "should have no error creating kubernetes workload instance")
		assert.NotNil(createdWorkloadInst, "should have a kubernetes workload instance returned")
		defer retryDelete(t, "kubernetes workload instance", func() error {
			_, err := client.DeleteKubernetesWorkloadInstance(apiClient, threeportAPIEndpoint, *createdWorkloadInst.ID)
			return err
		})

		// create a duplicate workload instance
		duplicateWorkloadInst := v0.KubernetesWorkloadInstance{
			Instance: v0.Instance{
				Name: &workloadInstName,
			},
			KubernetesRuntimeInstanceID:    testKubernetesRuntimeInst.ID,
			KubernetesWorkloadDefinitionID: createdWorkloadDef.ID,
		}

		_, err = client.CreateKubernetesWorkloadInstance(
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
		// 		KubernetesWorkloadInstanceID: createdWorkloadInst.ID,
		// 		KubernetesRuntimeInstanceID: testKubernetesRuntimeInst.ID,
		// 	},
		// )
		// assert.Nil(err, "should have no error creating secret instance")

		// configure domain name instance
		domainNameInstance := &v0.DomainNameInstance{
			Instance: v0.Instance{
				Name: &workloadInstName,
			},
			DomainNameDefinitionID:       createdDomainNameDefinition.ID,
			KubernetesWorkloadInstanceID: createdWorkloadInst.ID,
			KubernetesRuntimeInstanceID:  testKubernetesRuntimeInst.ID,
		}

		// create domain name instance
		createdDomainNameInstance, err := client.CreateDomainNameInstance(
			apiClient,
			threeportAPIEndpoint,
			domainNameInstance,
		)
		require.Nil(t, err, "should have no error creating domain name instance")
		defer retryDelete(t, "domain name instance", func() error {
			_, err := client.DeleteDomainNameInstance(apiClient, threeportAPIEndpoint, *createdDomainNameInstance.ID)
			return err
		})

		// create a gateway instance
		gatewayInstance := &v0.GatewayInstance{
			Instance: v0.Instance{
				Name: util.Ptr("gatewayInstance"),
			},
			KubernetesRuntimeInstanceID:  testKubernetesRuntimeInst.ID,
			GatewayDefinitionID:          createdGatewayDef.ID,
			KubernetesWorkloadInstanceID: createdWorkloadInst.ID,
		}
		createdGatewayInstance, err := client.CreateGatewayInstance(
			apiClient,
			threeportAPIEndpoint,
			gatewayInstance,
		)
		require.Nil(t, err, "should have no error creating gateway instance")
		defer retryDelete(t, "gateway instance", func() error {
			_, err := client.DeleteGatewayInstance(apiClient, threeportAPIEndpoint, *createdGatewayInstance.ID)
			return err
		})

		// get the kubernetes runtime instance from the threeport API so we can connect to it
		kubernetesRuntimeInstance, err := client.GetKubernetesRuntimeInstanceByID(
			apiClient,
			threeportAPIEndpoint,
			*testKubernetesRuntimeInst.ID,
		)
		assert.Nil(err, "should have no error getting kubernetes runtime instance")
		assert.NotNil(kubernetesRuntimeInstance, "should have a kubernetes runtime instance returned")

		encryptionKey, err := threeportConfig.GetThreeportEncryptionKey(threeportConfig.CurrentControlPlane)
		require.Nil(t, err, "should have no error getting encryption key")

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

		// check threeport API for expected Events on this KubernetesWorkloadInstance
		startedEventFound := false
		eventAttempts := 0
		eventAttemptsMax := 300
		eventCheckDurationSeconds := 1
		for eventAttempts < eventAttemptsMax {
			events, err := client.GetEventsJoinAttachedObjectReferenceByQueryString(
				apiClient,
				threeportAPIEndpoint,
				fmt.Sprintf(
					"objectid=%d&objecttypename=KubernetesWorkloadInstance&objectnamespace=threeport.io&objectversion=v0",
					*createdWorkloadInst.ID,
				),
			)
			assert.Nil(err, "should have no error returned when trying to retrieve events for kubernetes workload instance")
			for _, evt := range *events {
				if *evt.Type == "Normal" && *evt.Reason == "Started" {
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

		// relationship-tag FK transitions on KubernetesWorkloadInstance.
		// KubernetesWorkloadDefinitionID is tagged `relationship:"requires"` and
		// `validate:"required"`. once set at create, the API must reject
		// any further state change (clear or reassign). a second
		// workload definition is created here purely as the "other
		// value" target for the change-rejection assertion.
		secondWorkloadDefName := fmt.Sprintf("%s-relationship-target", workloadDefName)
		secondWorkloadDef := v0.KubernetesWorkloadDefinition{
			Definition: v0.Definition{
				Name: &secondWorkloadDefName,
			},
			YAMLDocument: &workloadDefYAML,
		}
		createdSecondWorkloadDef, err := client.CreateKubernetesWorkloadDefinition(
			apiClient,
			threeportAPIEndpoint,
			&secondWorkloadDef,
		)
		require.Nil(t, err, "should have no error creating second kubernetes workload definition")
		defer retryDelete(t, "second kubernetes workload definition", func() error {
			_, err := client.DeleteKubernetesWorkloadDefinition(apiClient, threeportAPIEndpoint, *createdSecondWorkloadDef.ID)
			return err
		})

		// value to nil: clearing a requires-tagged FK should be rejected.
		// the payload sets KubernetesWorkloadDefinitionID to nil while preserving
		// the row identity via ID.
		clearWorkloadDefIDPayload := v0.KubernetesWorkloadInstance{
			Common:                         v0.Common{ID: createdWorkloadInst.ID},
			KubernetesWorkloadDefinitionID: nil,
		}
		_, err = client.UpdateKubernetesWorkloadInstance(apiClient, threeportAPIEndpoint, &clearWorkloadDefIDPayload)
		assert.NotNil(err, "should reject clearing a requires-tagged FK (KubernetesWorkloadDefinitionID nil)")

		// value to other: reassigning a requires-tagged FK to a
		// different valid target should be rejected.
		changeWorkloadDefIDPayload := v0.KubernetesWorkloadInstance{
			Common:                         v0.Common{ID: createdWorkloadInst.ID},
			KubernetesWorkloadDefinitionID: createdSecondWorkloadDef.ID,
		}
		_, err = client.UpdateKubernetesWorkloadInstance(apiClient, threeportAPIEndpoint, &changeWorkloadDefIDPayload)
		assert.NotNil(err, "should reject reassigning a requires-tagged FK (KubernetesWorkloadDefinitionID to a different value)")

		// value to nil on KubernetesRuntimeInstanceID, also
		// `relationship:"requires"`. tests the same rule on a different
		// FK on the same row.
		clearRuntimeIDPayload := v0.KubernetesWorkloadInstance{
			Common:                      v0.Common{ID: createdWorkloadInst.ID},
			KubernetesRuntimeInstanceID: nil,
		}
		_, err = client.UpdateKubernetesWorkloadInstance(apiClient, threeportAPIEndpoint, &clearRuntimeIDPayload)
		assert.NotNil(err, "should reject clearing a requires-tagged FK (KubernetesRuntimeInstanceID nil)")

		// encrypted-field round-trip on KubernetesRuntimeInstance.
		// ConnectionToken is tagged `encrypt:"true"`. The API encrypts
		// on write but doesn't decrypt on read, so we decrypt the GET
		// response with the shared key and compare to what was written.
		//
		// Uses a standalone runtime rather than the control-plane host
		// runtime. In provider-managed environments the host runtime
		// is paired with a provider-side instance via a `marries` AOR,
		// and the 1-to-1 ownership guard rejects external updates.
		// A standalone runtime that nothing pairs with is safe to
		// mutate.
		encDefName := fmt.Sprintf("encrypted-field-test-runtime-def-%s", testWorkload.Name)
		encInstName := fmt.Sprintf("encrypted-field-test-runtime-%s", testWorkload.Name)

		encDef, err := client.CreateKubernetesRuntimeDefinition(
			apiClient,
			threeportAPIEndpoint,
			&v0.KubernetesRuntimeDefinition{
				Definition:    v0.Definition{Name: util.Ptr(encDefName)},
				InfraProvider: util.Ptr("kind"),
			},
		)
		require.Nil(t, err, "should have no error creating runtime definition for encrypted-field test")
		defer retryDelete(t, "encrypted-field test runtime definition", func() error {
			_, err := client.DeleteKubernetesRuntimeDefinition(apiClient, threeportAPIEndpoint, *encDef.ID)
			return err
		})

		// Reconciled is pre-set so the kubernetes-runtime-controller
		// skips this fixture instead of trying to provision a real
		// kind cluster on top of the test host. The encrypted-field
		// round-trip only needs a row in the database that nothing
		// else owns or writes to.
		encInst, err := client.CreateKubernetesRuntimeInstance(
			apiClient,
			threeportAPIEndpoint,
			&v0.KubernetesRuntimeInstance{
				Instance:                      v0.Instance{Name: util.Ptr(encInstName)},
				Reconciliation:                v0.Reconciliation{Reconciled: util.Ptr(true)},
				Location:                      util.Ptr("Local"),
				KubernetesRuntimeDefinitionID: encDef.ID,
			},
		)
		require.Nil(t, err, "should have no error creating runtime instance for encrypted-field test")
		defer retryDelete(t, "encrypted-field test runtime instance", func() error {
			_, err := client.DeleteKubernetesRuntimeInstance(apiClient, threeportAPIEndpoint, *encInst.ID)
			return err
		})

		setTokenPayload := v0.KubernetesRuntimeInstance{
			Common:          v0.Common{ID: encInst.ID},
			ConnectionToken: util.Ptr("encrypted-field-test-value-1"),
		}
		_, err = client.UpdateKubernetesRuntimeInstance(apiClient, threeportAPIEndpoint, &setTokenPayload)
		assert.Nil(err, "should have no error setting encrypted ConnectionToken")

		// fetch back, expect ciphertext, decrypt with the shared key,
		// and verify it equals what we wrote
		readBackKRI, err := client.GetKubernetesRuntimeInstanceByID(apiClient, threeportAPIEndpoint, *encInst.ID)
		require.Nil(t, err, "should have no error reading back KRI after setting ConnectionToken")
		require.NotNil(t, readBackKRI.ConnectionToken, "ConnectionToken should be non-nil on read after set")
		decryptedFirst, err := encryption.Decrypt(encryptionKey, *readBackKRI.ConnectionToken)
		require.Nil(t, err, "should have no error decrypting ConnectionToken after first update")
		assert.Equal("encrypted-field-test-value-1", decryptedFirst, "decrypted ConnectionToken should equal the value we wrote")

		// value to other: change the encrypted field again, decrypt,
		// and verify the new value round-trips
		changeTokenPayload := v0.KubernetesRuntimeInstance{
			Common:          v0.Common{ID: encInst.ID},
			ConnectionToken: util.Ptr("encrypted-field-test-value-2"),
		}
		_, err = client.UpdateKubernetesRuntimeInstance(apiClient, threeportAPIEndpoint, &changeTokenPayload)
		assert.Nil(err, "should have no error updating encrypted ConnectionToken")
		readBackKRI, err = client.GetKubernetesRuntimeInstanceByID(apiClient, threeportAPIEndpoint, *encInst.ID)
		require.Nil(t, err, "should have no error reading back KRI after updating ConnectionToken")
		require.NotNil(t, readBackKRI.ConnectionToken, "ConnectionToken should be non-nil on read after update")
		decryptedSecond, err := encryption.Decrypt(encryptionKey, *readBackKRI.ConnectionToken)
		require.Nil(t, err, "should have no error decrypting ConnectionToken after second update")
		assert.Equal("encrypted-field-test-value-2", decryptedSecond, "decrypted ConnectionToken should equal the updated value")

		// AOR delete-guards: callers must tear down in reverse order.
		// before any cleanup, assert each definition delete is rejected
		// while its instance still references it.
		_, err = client.DeleteKubernetesWorkloadDefinition(
			apiClient,
			threeportAPIEndpoint,
			*createdWorkloadDef.ID,
		)
		assert.NotNil(err, "should fail to delete workload definition while workload instance still references it")

		_, err = client.DeleteGatewayDefinition(
			apiClient,
			threeportAPIEndpoint,
			*createdGatewayDef.ID,
		)
		assert.NotNil(err, "should fail to delete gateway definition while gateway instance still references it")

		_, err = client.DeleteDomainNameDefinition(
			apiClient,
			threeportAPIEndpoint,
			*createdDomainNameDefinition.ID,
		)
		assert.NotNil(err, "should fail to delete domain name definition while domain name instance still references it")

		// reverse-order tear-down: delete the gateway and domain name
		// instances first so the workload instance has no incoming refs
		// when its hard-delete runs in the reconciler
		_, err = client.DeleteGatewayInstance(
			apiClient,
			threeportAPIEndpoint,
			*createdGatewayInstance.ID,
		)
		assert.Nil(err, "should have no error deleting gateway instance")

		gatewayInstanceDeleted := false
		gatewayInstanceCheckMax := 90
		for i := 0; i < gatewayInstanceCheckMax; i++ {
			_, err := client.GetGatewayInstanceByID(apiClient, threeportAPIEndpoint, *createdGatewayInstance.ID)
			if errors.Is(err, client_lib.ErrObjectNotFound) {
				gatewayInstanceDeleted = true
				break
			}
			time.Sleep(time.Second)
		}
		assert.True(gatewayInstanceDeleted, fmt.Sprintf("gateway instance should be gone within %d seconds", gatewayInstanceCheckMax))

		_, err = client.DeleteDomainNameInstance(
			apiClient,
			threeportAPIEndpoint,
			*createdDomainNameInstance.ID,
		)
		assert.Nil(err, "should have no error deleting domain name instance")

		domainNameInstanceDeleted := false
		domainNameInstanceCheckMax := 90
		for i := 0; i < domainNameInstanceCheckMax; i++ {
			_, err := client.GetDomainNameInstanceByID(apiClient, threeportAPIEndpoint, *createdDomainNameInstance.ID)
			if errors.Is(err, client_lib.ErrObjectNotFound) {
				domainNameInstanceDeleted = true
				break
			}
			time.Sleep(time.Second)
		}
		assert.True(domainNameInstanceDeleted, fmt.Sprintf("domain name instance should be gone within %d seconds", domainNameInstanceCheckMax))

		// now delete the workload instance — its only remaining incoming
		// refs are from the gateway/domain name instances we just removed
		deletedWorkloadInst, err := client.DeleteKubernetesWorkloadInstance(
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
			_, err := client.GetKubernetesWorkloadInstanceByID(apiClient, threeportAPIEndpoint, *createdWorkloadInst.ID)
			if err != nil {
				if errors.Is(err, client_lib.ErrObjectNotFound) {
					workloadInstanceDeleted = true
					break
				}
			}
			// no error means workload instance was found - hasn't yet been deleted
			deletedCheckAttempts += 1
			time.Sleep(time.Second * time.Duration(deletedCheckDurationSeconds))
		}
		assert.True(workloadInstanceDeleted, fmt.Sprintf("should have found that workload instance was deleted after %d seconds", deletedCheckAttemptsMax*deletedCheckDurationSeconds))

		// make sure there are zero kubernetes workload instances in system
		workloadInsts, err := client.GetKubernetesWorkloadInstances(
			apiClient,
			threeportAPIEndpoint,
		)
		assert.Nil(err, "should have no errors geting all kubernetes workload instances")
		if assert.NotNil(workloadInsts, "should have an array of kubernetes workload instances returned") {
			for _, wi := range *workloadInsts {
				assert.NotEqual(wi.ID, deletedWorkloadInst.ID, "should not get back deleted kubernetes workload instance when retrieving all kubernetes workload instances")
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

		// definitions can now be deleted in any order — none of their
		// instances remain to block them
		_, err = client.DeleteGatewayDefinition(
			apiClient,
			threeportAPIEndpoint,
			*createdGatewayDef.ID,
		)
		assert.Nil(err, "should have no error deleting gateway definition")

		// wait for gateway def deletion reconciliation to complete
		reconcileAttempts := 0
		reconcileAttemptsMax := 20
		reconcileCheckDurationSeconds := 1
		gatewayDefDeleted := false
		for reconcileAttempts < reconcileAttemptsMax {
			gatewayDefs, err := client.GetGatewayDefinitions(
				apiClient,
				threeportAPIEndpoint,
			)
			assert.Nil(err, "should get no error list gateway definitions")

			if len(*gatewayDefs) > 0 {
				reconcileAttempts++
				time.Sleep(time.Second * time.Duration(reconcileCheckDurationSeconds))
				continue
			}
			gatewayDefDeleted = true
			break
		}
		assert.True(gatewayDefDeleted, "gateway definition deletion reconciliation should be complete")

		_, err = client.DeleteDomainNameDefinition(
			apiClient,
			threeportAPIEndpoint,
			*createdDomainNameDefinition.ID,
		)
		assert.Nil(err, "should have no error deleting domain name definition")

		// delete secret definition
		if createdSecretDefinition != nil && createdSecretDefinition.ID != nil {
			_, err = client.DeleteSecretDefinition(
				apiClient,
				threeportAPIEndpoint,
				*createdSecretDefinition.ID,
			)
			assert.Nil(err, "should have no error deleting secret definition")

			// wait for secret-controller to finish reconciling the delete
			// and remove the row, otherwise the next test iteration trips
			// the unique-name constraint when re-creating with the same name
			secretDefDeleted := false
			for i := 0; i < 30; i++ {
				_, err := client.GetSecretDefinitionByID(apiClient, threeportAPIEndpoint, *createdSecretDefinition.ID)
				if errors.Is(err, client_lib.ErrObjectNotFound) {
					secretDefDeleted = true
					break
				}
				time.Sleep(time.Second)
			}
			assert.True(secretDefDeleted, "secret definition should be gone within 30 seconds")
		}

		// delete the second workload def created earlier for the
		// fk-transition tests. no instance references it, so the delete
		// goes through immediately.
		_, err = client.DeleteKubernetesWorkloadDefinition(
			apiClient,
			threeportAPIEndpoint,
			*createdSecondWorkloadDef.ID,
		)
		assert.Nil(err, "should have no error deleting second workload definition")

		// delete workload definition
		deletedWorkloadDef, err := client.DeleteKubernetesWorkloadDefinition(
			apiClient,
			threeportAPIEndpoint,
			*createdWorkloadDef.ID,
		)
		assert.Nil(err, "should have no error deleting workload definition")

		// make sure the workload definition is gone
		if err := util.Retry(10, 3, func() error {
			workloadDefs, err := client.GetKubernetesWorkloadDefinitions(
				apiClient,
				threeportAPIEndpoint,
			)
			if err != nil {
				return fmt.Errorf("failed to get workload definitions: %w", err)
			}
			for _, wd := range *workloadDefs {
				if wd.ID == deletedWorkloadDef.ID {
					return fmt.Errorf("deleted workload definition with ID %d still returned from Threeport API", wd.ID)
				}
			}

			return nil
		}); err != nil {
			assert.Nil(err, "should not get back deleted workload definition when retrieving all workload definitions")
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
