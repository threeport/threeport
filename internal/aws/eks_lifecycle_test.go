package aws

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/threeport/threeport/internal/provider"
	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	tpaws "github.com/threeport/threeport/pkg/aws/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestEksInfraFromApiObjects_MapsDefinitionNodeGroupFields checks the runtime
// definition's node group settings reach the infra object the Pulumi program
// provisions from.  A field dropped here provisions a cluster that is sized
// differently from what was asked for, with nothing to indicate it.
func TestEksInfraFromApiObjects_MapsDefinitionNodeGroupFields(t *testing.T) {
	instance := &v0.AwsEksKubernetesRuntimeInstance{
		Instance: v0.Instance{Name: util.Ptr("test-instance")},
		Region:   util.Ptr("us-east-1"),
	}
	definition := &v0.AwsEksKubernetesRuntimeDefinition{
		ZoneCount:                    util.Ptr(3),
		DefaultNodeGroupInstanceType: util.Ptr("t3.large"),
		DefaultNodeGroupInitialSize:  util.Ptr(2),
		DefaultNodeGroupMinimumSize:  util.Ptr(3),
		DefaultNodeGroupMaximumSize:  util.Ptr(7),
	}
	awsConfig := &aws.Config{Region: "us-east-1"}
	providerCredentials := tpaws.AwsProviderCredentials{
		AccessKeyId:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		AssumeRoleArn:   "arn:aws:iam::123456789012:role/some-role",
	}

	infra := eksInfraFromApiObjects(
		instance,
		definition,
		"123456789012",
		awsConfig,
		providerCredentials,
		nil,
	)

	assert.Equal(t, "test-instance", infra.RuntimeInstanceName)
	assert.Equal(t, "123456789012", infra.AwsAccountID)
	assert.Equal(t, "us-east-1", infra.Region)
	assert.Same(t, awsConfig, infra.AwsConfig)
	// the Pulumi provider must get the role to assume, not credentials this
	// process already assumed with
	assert.Equal(t, providerCredentials, infra.ProviderCredentials)
	assert.Equal(t, int32(3), infra.ZoneCount)
	assert.Equal(t, "t3.large", infra.DefaultNodeGroupInstanceType)
	assert.Equal(t, int32(2), infra.DefaultNodeGroupInitialNodes)
	assert.Equal(t, int32(3), infra.DefaultNodeGroupMinNodes)
	assert.Equal(t, int32(7), infra.DefaultNodeGroupMaxNodes)
}

// TestEksInfraSatisfiesLifecycleInterfaces checks the infra object plugs into
// the shared lifecycle engine the way GKE and OKE do.  The engine type-asserts
// for the optional interfaces, so a missing one is skipped silently rather
// than reported.
func TestEksInfraSatisfiesLifecycleInterfaces(t *testing.T) {
	var infra any = &provider.KubernetesRuntimeInfraEKS{}

	_, isInfraProvider := infra.(provider.InfraProvider)
	assert.True(t, isInfraProvider, "must implement InfraProvider")

	// without this the engine cannot stream state to the API while the stack
	// is still being provisioned, so an interrupted create cannot resume
	_, isStreamable := infra.(provider.StreamableProvider)
	assert.True(t, isStreamable, "must implement StreamableProvider")

	// without this the stack is not refreshed before a destroy, leaving stale
	// pending operations in the way
	_, isRefreshable := infra.(provider.RefreshableProvider)
	assert.True(t, isRefreshable, "must implement RefreshableProvider")
}

// TestEksLifecycleSatisfiesInfraLifecycleProvider checks the adapter
// implements every method the shared create and delete state machines call.
func TestEksLifecycleSatisfiesInfraLifecycleProvider(t *testing.T) {
	var lifecycle any = &eksLifecycle{}

	_, ok := lifecycle.(provider.InfraLifecycleProvider)
	require.True(t, ok, "must implement InfraLifecycleProvider")
}

// TestEksInfraFromApiObjects_PulumiProjectSetAtConstruction covers the state
// file path the crash-recovery watcher is built from.
//
// executeInfraCreate starts the fsnotify watcher that streams partial state to
// the API before it calls DeployInfra, and GetStateFilePath builds the watched
// path from ProjectName. Filling that in later - from CreateInfra, as it used
// to be - leaves the watcher reading an empty name, so it watches one directory
// above where Pulumi writes and never fires. A controller that then crashes
// mid-provision has persisted nothing, and the retry starts over with the first
// attempt's resources still in the account.
func TestEksInfraFromApiObjects_PulumiProjectSetAtConstruction(t *testing.T) {
	infra := eksInfraFromApiObjects(
		&v0.AwsEksKubernetesRuntimeInstance{
			Instance: v0.Instance{Name: util.Ptr("test-instance")},
			Region:   util.Ptr("us-east-1"),
		},
		&v0.AwsEksKubernetesRuntimeDefinition{
			ZoneCount:                    util.Ptr(2),
			DefaultNodeGroupInstanceType: util.Ptr("t3.medium"),
			DefaultNodeGroupInitialSize:  util.Ptr(3),
			DefaultNodeGroupMinimumSize:  util.Ptr(3),
			DefaultNodeGroupMaximumSize:  util.Ptr(5),
		},
		"123456789012",
		&aws.Config{Region: "us-east-1"},
		tpaws.AwsProviderCredentials{},
		nil,
	)

	require.NotEmpty(
		t, infra.ProjectName,
		"the project name has to be set before the lifecycle starts watching for state",
	)

	statePath, err := infra.GetStateFilePath()
	require.NoError(t, err)
	assert.Contains(
		t, statePath, filepath.Join("stacks", infra.ProjectName, "test-instance.json"),
		"the watched path must name the project directory Pulumi writes into",
	)

	// the region reaches the stack config at construction too, for the same
	// reason: nothing may write to the infra once the watcher is reading it
	assert.Equal(t, "us-east-1", infra.StackConfigs["aws:region"])
}

// TestBuildInfraIsCachedForTheAttempt covers the short circuit that keeps
// IsCreateComplete from rebuilding everything on every poll.
//
// HandleInfraCreate polls roughly every two minutes for the ten to twenty a
// create takes, and each rebuild cost four Threeport round trips, a fresh
// credential cache and - whenever a role ARN is configured, which is the normal
// EKS case - a new sts:AssumeRole that discarded the previous hour-long
// session. The reconciler here is nil on purpose: a cache miss would have to
// reach for it, so returning cleanly is the whole assertion.
func TestBuildInfraIsCachedForTheAttempt(t *testing.T) {
	cached := &provider.KubernetesRuntimeInfraEKS{
		PulumiWorkspace: provider.PulumiWorkspace{RuntimeInstanceName: "already-built"},
	}
	lifecycle := &eksLifecycle{infra: cached}

	for _, call := range []string{"first", "second"} {
		var built provider.InfraProvider
		var err error
		require.NotPanics(
			t, func() { built, err = lifecycle.BuildInfra() },
			call+" call must not reach the API",
		)
		require.NoError(t, err)
		assert.Same(t, cached, built, call+" call must hand back the cached infra")
	}

	// the same short circuit guards the path IsCreateComplete takes, which
	// passes the instance it already fetched rather than fetching it again
	fromInstance, err := lifecycle.buildInfraFrom(nil)
	require.NoError(t, err)
	assert.Same(t, cached, fromInstance)
}

// TestEksLifecycle_GetReconciliation_AbandonInfrastructure covers the operator
// override reaching the shared delete state machine. A snapshot that drops it
// leaves a held delete refused no matter what the operator sets.
func TestEksLifecycle_GetReconciliation_AbandonInfrastructure(t *testing.T) {
	cases := []struct {
		name    string
		abandon *bool
		want    bool
	}{
		{name: "unset is not abandoned", abandon: nil, want: false},
		{name: "false is not abandoned", abandon: util.Ptr(false), want: false},
		{name: "true is abandoned", abandon: util.Ptr(true), want: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			instanceID := uint(7)
			instance := v0.AwsEksKubernetesRuntimeInstance{
				Common:                v0.Common{ID: &instanceID},
				AbandonInfrastructure: tc.abandon,
			}

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := json.Marshal(apiserver_lib.Response{
					Status: apiserver_lib.Status{Code: http.StatusOK, Message: "OK"},
					Data:   []apiserver_lib.Object{instance},
				})
				require.NoError(t, err)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(body)
			}))
			defer srv.Close()

			lifecycle := &eksLifecycle{
				r: &controller.Reconciler{
					APIClient: &http.Client{},
					APIServer: strings.TrimPrefix(srv.URL, "http://"),
				},
				instanceID: instanceID,
			}

			snap, err := lifecycle.GetReconciliation()
			require.NoError(t, err)
			assert.Equal(t, tc.want, snap.AbandonInfrastructure)
		})
	}
}
