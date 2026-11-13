package aws

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/threeport/threeport/internal/provider"
	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// the EKS adapter has its clusters upgraded
var _ provider.UpgradeLifecycleProvider = (*eksLifecycle)(nil)

// recordedUpdate is an update the API was sent, and the path it was sent to.
type recordedUpdate struct {
	path   string
	fields map[string]any
}

// TestTheEksInstanceRecordsItsUpgrade covers the upgrade state the EKS
// adapter reads from and writes to the API, and the autoscaler following an
// upgrade: the kubernetes runtime instance is marked unreconciled, so its
// controller installs the upgraded version's release.
func TestTheEksInstanceRecordsItsUpgrade(t *testing.T) {
	instance := v0.AwsEksKubernetesRuntimeInstance{
		Common:                              v0.Common{ID: util.Ptr(uint(7))},
		AwsEksKubernetesRuntimeDefinitionID: util.Ptr(uint(3)),
		KubernetesRuntimeInstanceID:         util.Ptr(uint(11)),
		KubernetesVersion:                   util.Ptr("1.35"),
		UpgradeRefused:                      util.Ptr(true),
		UpgradeStatus:                       util.Ptr("refused"),
	}
	definition := v0.AwsEksKubernetesRuntimeDefinition{
		Common:            v0.Common{ID: util.Ptr(uint(3))},
		KubernetesVersion: util.Ptr("1.36"),
	}

	var mu sync.Mutex
	var updates []recordedUpdate
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var object any = instance
		if strings.Contains(r.URL.Path, "definitions") {
			object = definition
		}
		if r.Method == http.MethodPatch {
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			var fields map[string]any
			require.NoError(t, json.Unmarshal(body, &fields))
			mu.Lock()
			updates = append(updates, recordedUpdate{path: r.URL.Path, fields: fields})
			mu.Unlock()
		}
		body, err := json.Marshal(apiserver_lib.Response{
			Status: apiserver_lib.Status{Code: http.StatusOK, Message: "OK"},
			Data:   []apiserver_lib.Object{object},
		})
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer server.Close()
	lifecycle := &eksLifecycle{
		r: &controller.Reconciler{
			APIClient: &http.Client{},
			APIServer: strings.TrimPrefix(server.URL, "http://"),
		},
		instanceID: 7,
	}

	snapshot, err := lifecycle.GetUpgrade()
	require.NoError(t, err)
	assert.Equal(t, "1.35", snapshot.RecordedVersion)
	assert.True(t, snapshot.Refused)
	assert.False(t, snapshot.Upgrading, "unset is not upgrading")
	target, err := lifecycle.GetUpgradeTarget()
	require.NoError(t, err)
	assert.Equal(t, "1.36", target)

	require.NoError(t, lifecycle.AckUpgrade("upgrading"))
	require.NoError(t, lifecycle.FinishUpgrade(provider.UpgradeOutcome{Failed: true, Status: "failed"}))
	require.NoError(t, lifecycle.OnUpgraded(nil))

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, updates, 3)
	assert.Contains(t, updates[0].path, "aws-eks-kubernetes-runtime-instances/7")
	assert.Equal(t, true, updates[0].fields["Upgrading"])
	assert.Equal(t, false, updates[0].fields["UpgradeRefused"], "a new upgrade clears a refusal")
	assert.Equal(t, true, updates[1].fields["UpgradeFailed"])
	assert.Equal(t, false, updates[1].fields["Upgrading"])
	assert.Contains(t, updates[2].path, "/kubernetes-runtime-instances/11")
	assert.Equal(t, false, updates[2].fields["Reconciled"])
}

// TestAnEksUpdateReachesTheUpgradeHandler covers the EKS update reconciler:
// it runs the shared update handler, which upgrades clusters, rather than
// returning without looking.  An instance whose creation isn't confirmed is
// held for the create, which only that handler does.
func TestAnEksUpdateReachesTheUpgradeHandler(t *testing.T) {
	instance := v0.AwsEksKubernetesRuntimeInstance{
		Common:   v0.Common{ID: util.Ptr(uint(7))},
		Instance: v0.Instance{Name: util.Ptr("eks")},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := json.Marshal(apiserver_lib.Response{
			Status: apiserver_lib.Status{Code: http.StatusOK, Message: "OK"},
			Data:   []apiserver_lib.Object{instance},
		})
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer server.Close()
	r := &controller.Reconciler{APIClient: &http.Client{}, APIServer: strings.TrimPrefix(server.URL, "http://")}
	log := logr.Discard()

	requeue, err := v0AwsEksKubernetesRuntimeInstanceUpdated(r, &instance, &log)
	require.NoError(t, err)
	assert.Equal(t, int64(120), requeue)
}
