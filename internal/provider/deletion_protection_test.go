package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awseks "github.com/aws/aws-sdk-go-v2/service/eks"
	ekstypes "github.com/aws/aws-sdk-go-v2/service/eks/types"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// protectRecordingMocks records whether each resource a program registers is
// protected, by its type.
type protectRecordingMocks struct {
	mu        sync.Mutex
	protected map[string][]bool
	call      func(args pulumi.MockCallArgs) (resource.PropertyMap, error)
}

func (m *protectRecordingMocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.protected == nil {
		m.protected = map[string][]bool{}
	}
	m.protected[args.TypeToken] = append(m.protected[args.TypeToken], args.RegisterRPC.GetProtect())

	return args.Name + "-id", args.Inputs, nil
}

func (m *protectRecordingMocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	if m.call != nil {
		return m.call(args)
	}

	return resource.PropertyMap{}, nil
}

// protectedTypes returns the types of the resources registered protected.
func (m *protectRecordingMocks) protectedTypes() []string {
	var types []string
	for typeToken, protections := range m.protected {
		for _, protected := range protections {
			if protected {
				types = append(types, typeToken)
			}
		}
	}

	return types
}

// TestAProtectedClusterAndItsNodesAreProtectedInPulumi covers the EKS and GKE
// programs: with DeletionProtection, the cluster and its node group or node
// pool are registered protected, so Pulumi refuses a plan that destroys or
// replaces them, and nothing else is.  Without it nothing is.
func TestAProtectedClusterAndItsNodesAreProtectedInPulumi(t *testing.T) {
	eksCall := (&eksRecordingMocks{}).Call
	for name, run := range map[string]func(protect bool) *protectRecordingMocks{
		"EKS": func(protect bool) *protectRecordingMocks {
			infra := testEksInfra(2)
			infra.DeletionProtection = protect
			mocks := &protectRecordingMocks{call: eksCall}
			require.NoError(t, pulumi.RunErr(infra.pulumiProgram(), pulumi.WithMocks("eks", "test-stack", mocks)))
			return mocks
		},
		"GKE": func(protect bool) *protectRecordingMocks {
			infra := &KubernetesRuntimeInfraGKE{
				PulumiWorkspace:        PulumiWorkspace{RuntimeInstanceName: "gke-protect-test", ProjectName: "gke"},
				ProjectID:              "test-project",
				Region:                 "us-central1",
				WorkerNodeInitialCount: 1,
				DeletionProtection:     protect,
			}
			mocks := &protectRecordingMocks{}
			require.NoError(t, pulumi.RunErr(infra.pulumiProgram(), pulumi.WithMocks("gke", "test-stack", mocks)))
			return mocks
		},
	} {
		t.Run(name, func(t *testing.T) {
			clusterAndNodes := map[string][]string{
				"EKS": {eksClusterTypeToken, eksNodeGroupTypeToken},
				"GKE": {gkeClusterTypeToken, gkeNodePoolTypeToken},
			}[name]
			assert.ElementsMatch(t, clusterAndNodes, run(true).protectedTypes())
			assert.Empty(t, run(false).protectedTypes())
		})
	}
}

// TestADestroyRemovesTheStacksProtection covers the state a destroy imports
// before it runs: every protected resource is unprotected, and everything
// else in the deployment is kept as it was.  A deployment with nothing
// protected, or empty, is left alone.
func TestADestroyRemovesTheStacksProtection(t *testing.T) {
	deployment := json.RawMessage(`{
		"manifest": {"time": "2026-11-13T00:00:00Z", "version": "v3.150.0"},
		"secrets_providers": {"type": "passphrase"},
		"resources": [
			{"urn": "urn:pulumi:stack::eks::pulumi:pulumi:Stack::eks-stack", "custom": false, "type": "pulumi:pulumi:Stack"},
			{"urn": "urn:pulumi:stack::eks::aws:eks/cluster:Cluster::c", "custom": true, "protect": true, "id": "c",
			 "inputs": {"version": "1.36", "desiredSize": 12345678901234567890}, "type": "aws:eks/cluster:Cluster"},
			{"urn": "urn:pulumi:stack::eks::aws:eks/nodeGroup:NodeGroup::n", "custom": true, "protect": true, "type": "aws:eks/nodeGroup:NodeGroup"},
			{"urn": "urn:pulumi:stack::eks::aws:ec2/vpc:Vpc::v", "custom": true, "protect": false, "type": "aws:ec2/vpc:Vpc"}
		],
		"pending_operations": [{"type": "creating"}]
	}`)

	unprotected, changed, err := unprotectDeployment(deployment)
	require.NoError(t, err)
	assert.True(t, changed)

	decode := func(raw json.RawMessage) map[string]any {
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		var contents map[string]any
		require.NoError(t, decoder.Decode(&contents))
		return contents
	}
	before, after := decode(deployment), decode(unprotected)
	beforeResources := before["resources"].([]any)
	afterResources := after["resources"].([]any)
	require.Len(t, afterResources, len(beforeResources))
	for index, resource := range afterResources {
		expected := beforeResources[index].(map[string]any)
		if protected, _ := expected["protect"].(bool); protected {
			delete(expected, "protect")
		}
		assert.Equal(t, expected, resource, "resource %d keeps everything but its protection", index)
	}
	delete(before, "resources")
	delete(after, "resources")
	assert.Equal(t, before, after, "the rest of the deployment is kept")

	unchanged := json.RawMessage(`{"resources": [{"urn": "u", "protect": false}, {"urn": "v"}]}`)
	kept, changed, err := unprotectDeployment(unchanged)
	require.NoError(t, err)
	assert.False(t, changed)
	assert.Equal(t, unchanged, kept)

	for _, empty := range []json.RawMessage{nil, json.RawMessage(`{}`), json.RawMessage(`{"resources": []}`)} {
		_, changed, err = unprotectDeployment(empty)
		require.NoError(t, err)
		assert.False(t, changed)
	}

	_, _, err = unprotectDeployment(json.RawMessage(`{"resources": "not a list"}`))
	assert.Error(t, err, "a deployment it can't read isn't written back")
}

// fakeEksDeletionProtection is an EKS API whose cluster goes through the
// statuses given, one per describe, and whose update goes through its own.
type fakeEksDeletionProtection struct {
	clusterStatuses []ekstypes.ClusterStatus
	protected       bool
	describeErr     error
	updateStatuses  []ekstypes.UpdateStatus
	updateErr       error

	describes int
	updates   []bool
	polls     int
}

func (f *fakeEksDeletionProtection) DescribeCluster(context.Context, *awseks.DescribeClusterInput, ...func(*awseks.Options)) (*awseks.DescribeClusterOutput, error) {
	if f.describeErr != nil {
		return nil, f.describeErr
	}
	status := f.clusterStatuses[min(f.describes, len(f.clusterStatuses)-1)]
	f.describes++

	return &awseks.DescribeClusterOutput{Cluster: &ekstypes.Cluster{
		Status:             status,
		DeletionProtection: aws.Bool(f.protected),
	}}, nil
}

func (f *fakeEksDeletionProtection) UpdateClusterConfig(_ context.Context, input *awseks.UpdateClusterConfigInput, _ ...func(*awseks.Options)) (*awseks.UpdateClusterConfigOutput, error) {
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	f.updates = append(f.updates, aws.ToBool(input.DeletionProtection))

	return &awseks.UpdateClusterConfigOutput{Update: &ekstypes.Update{Id: aws.String("update-1")}}, nil
}

func (f *fakeEksDeletionProtection) DescribeUpdate(context.Context, *awseks.DescribeUpdateInput, ...func(*awseks.Options)) (*awseks.DescribeUpdateOutput, error) {
	status := f.updateStatuses[min(f.polls, len(f.updateStatuses)-1)]
	f.polls++

	return &awseks.DescribeUpdateOutput{Update: &ekstypes.Update{
		Status: status,
		Errors: []ekstypes.ErrorDetail{{ErrorCode: ekstypes.ErrorCodeAccessDenied, ErrorMessage: aws.String("not allowed")}},
	}}, nil
}

// TestEksDeletionProtectionIsSetOnAnActiveCluster covers turning the EKS
// cluster's own deletion protection on and off: a cluster that already has it
// as asked, or doesn't exist, is left alone; one still settling is waited
// for; the update is followed until it finishes, and one EKS fails is an
// error.
func TestEksDeletionProtectionIsSetOnAnActiveCluster(t *testing.T) {
	interval := eksDeletionProtectionPollInterval
	eksDeletionProtectionPollInterval = time.Millisecond
	t.Cleanup(func() { eksDeletionProtectionPollInterval = interval })
	ctx := context.Background()
	active := []ekstypes.ClusterStatus{ekstypes.ClusterStatusActive}

	// already as asked
	fake := &fakeEksDeletionProtection{clusterStatuses: active, protected: true}
	require.NoError(t, setEksDeletionProtection(ctx, fake, "c", true))
	assert.Empty(t, fake.updates)
	fake = &fakeEksDeletionProtection{clusterStatuses: active}
	require.NoError(t, setEksDeletionProtection(ctx, fake, "c", false))
	assert.Empty(t, fake.updates)

	// no cluster to protect or unprotect
	fake = &fakeEksDeletionProtection{describeErr: &ekstypes.ResourceNotFoundException{Message: aws.String("gone")}}
	require.NoError(t, setEksDeletionProtection(ctx, fake, "c", false))
	fake = &fakeEksDeletionProtection{describeErr: errors.New("throttled")}
	assert.ErrorContains(t, setEksDeletionProtection(ctx, fake, "c", false), "throttled")

	// a cluster still updating is waited for, then the update is followed
	fake = &fakeEksDeletionProtection{
		clusterStatuses: []ekstypes.ClusterStatus{ekstypes.ClusterStatusUpdating, ekstypes.ClusterStatusActive},
		updateStatuses:  []ekstypes.UpdateStatus{ekstypes.UpdateStatusInProgress, ekstypes.UpdateStatusSuccessful},
	}
	require.NoError(t, setEksDeletionProtection(ctx, fake, "c", true))
	assert.Equal(t, []bool{true}, fake.updates)
	assert.Equal(t, 2, fake.describes)
	assert.Equal(t, 2, fake.polls, "the update is followed until it finishes")

	fake = &fakeEksDeletionProtection{
		clusterStatuses: active,
		protected:       true,
		updateStatuses:  []ekstypes.UpdateStatus{ekstypes.UpdateStatusSuccessful},
	}
	require.NoError(t, setEksDeletionProtection(ctx, fake, "c", false))
	assert.Equal(t, []bool{false}, fake.updates)

	// a cluster that won't settle is asked anyway, for EKS to say why
	fake = &fakeEksDeletionProtection{
		clusterStatuses: []ekstypes.ClusterStatus{ekstypes.ClusterStatusFailed},
		updateErr:       errors.New("cluster is FAILED"),
	}
	assert.ErrorContains(t, setEksDeletionProtection(ctx, fake, "c", true), "cluster is FAILED")

	// an update EKS fails
	fake = &fakeEksDeletionProtection{
		clusterStatuses: active,
		updateStatuses:  []ekstypes.UpdateStatus{ekstypes.UpdateStatusFailed},
	}
	assert.ErrorContains(t, setEksDeletionProtection(ctx, fake, "c", true), "not allowed")

	// a cluster that never settles times out
	timeoutCtx, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	fake = &fakeEksDeletionProtection{clusterStatuses: []ekstypes.ClusterStatus{ekstypes.ClusterStatusCreating}}
	assert.ErrorContains(t, setEksDeletionProtection(timeoutCtx, fake, "c", true), "timed out")
	assert.Empty(t, fake.updates)
}

// guardedLifecycle is a fake lifecycle whose cluster is protected while
// refusal returns an error.
type guardedLifecycle struct {
	*fakeLifecycle
	refusal error
}

func (g *guardedLifecycle) DeletionRefusal() error { return g.refusal }

// TestADeleteWaitsWhileTheClusterIsProtected covers a delete scheduled before
// the cluster's definition was protected: it is held, with a Warning event
// saying how to turn the protection off, and nothing is destroyed.  A failure
// to read the protection is an error, and once the protection is off the
// destroy runs.
func TestADeleteWaitsWhileTheClusterIsProtected(t *testing.T) {
	snapshot := func() *ReconciliationSnapshot {
		return &ReconciliationSnapshot{
			DeletionScheduled: util.Ptr(deleteTestBase.Add(-time.Minute)),
			CreationConfirmed: util.Ptr(deleteTestBase.Add(-time.Hour)),
			ResourceInventory: validStackState(),
		}
	}
	protected := &api_v0.DeletionProtectedError{DefinitionType: "aws-eks-kubernetes-runtime-definition", DefinitionName: "prod"}

	held := &guardedLifecycle{fakeLifecycle: newFakeLifecycle(snapshot()), refusal: protected}
	requeue, err := HandleInfraDelete(held, newTestLogger())
	assertHeld(t, held.fakeLifecycle, requeue, err, protected)
	assert.Equal(t, 0, held.callCount("AckDeletion"))
	assert.Equal(t, 0, held.callCount("BuildInfra"))

	unreadable := &guardedLifecycle{fakeLifecycle: newFakeLifecycle(snapshot()), refusal: errors.New("api down")}
	_, err = HandleInfraDelete(unreadable, newTestLogger())
	assert.ErrorContains(t, err, "api down")
	assert.Equal(t, 0, unreadable.callCount("AckDeletion"))

	cfg := testLifecycleConfig()
	restoreCfg := setLifecycleConfig(cfg)
	t.Cleanup(restoreCfg)
	fi := newFakeInfra()
	fi.setDestroy(infraBlock, nil)
	unprotected := &guardedLifecycle{fakeLifecycle: newFakeLifecycle(snapshot())}
	unprotected.setInfra(fi)
	_, err = HandleInfraDelete(unprotected, newTestLogger())
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		return fi.destroyCallCount() == 1
	}, 5*time.Second, 5*time.Millisecond, "destroy goroutine never launched")
	fi.releaseDestroy()
	drainDeleteOps(t)
}
