package secret

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"

	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// fakeAPI serves the secret definition's current state and records every path
// requested of it, so a test can tell whether the create went on to push.
type fakeAPI struct {
	mu       sync.Mutex
	requests []string
}

func (f *fakeAPI) paths() []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]string{}, f.requests...)
}

// newFakeAPI returns an API server that answers a get of secret definition 7
// with status and, when status is 200, the definition. The AWS provider the
// push looks up next is not found, which ends the push.
func newFakeAPI(t *testing.T, status int, current *v0.SecretDefinition) (*fakeAPI, *controller.Reconciler) {
	t.Helper()

	api := &fakeAPI{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		api.mu.Lock()
		api.requests = append(api.requests, r.URL.Path)
		api.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == fmt.Sprintf("%s/7", v0.PathSecretDefinitions) && status == http.StatusOK {
			object, err := json.Marshal(current)
			require.NoError(t, err)
			require.NoError(t, json.NewEncoder(w).Encode(map[string]interface{}{
				"Data": []json.RawMessage{object},
			}))
			return
		}

		w.WriteHeader(http.StatusNotFound)
		require.NoError(t, json.NewEncoder(w).Encode(apiserver_lib.Response{
			Status: apiserver_lib.Status{Code: http.StatusNotFound, Error: "not found"},
		}))
	}))
	t.Cleanup(server.Close)

	return api, &controller.Reconciler{APIServer: strings.TrimPrefix(server.URL, "http://"), APIClient: server.Client()}
}

// notificationSnapshot is the definition as the create notification carried it,
// with the data the API never stores.
func notificationSnapshot() *v0.SecretDefinition {
	data := datatypes.JSON(`{"key":"encrypted"}`)

	return &v0.SecretDefinition{
		Common:        v0.Common{ID: util.Ptr(uint(7))},
		Definition:    v0.Definition{Name: util.Ptr("db-password")},
		AwsProviderID: util.Ptr(uint(3)),
		Data:          &data,
	}
}

func TestSecretDefinitionCreatedSkipsADefinitionThatIsGone(t *testing.T) {
	api, r := newFakeAPI(t, http.StatusNotFound, nil)

	log := logr.Discard()
	requeueDelay, err := v0SecretDefinitionCreated(r, notificationSnapshot(), &log)

	require.NoError(t, err)
	assert.Zero(t, requeueDelay)
	assert.Equal(t, []string{"/v0/secret-definitions/7"}, api.paths(),
		"a definition that is gone is not pushed, so its provider is never looked up")
}

func TestSecretDefinitionCreatedSkipsADefinitionScheduledForDeletion(t *testing.T) {
	// the notification's snapshot predates the delete, so it is not scheduled
	snapshot := notificationSnapshot()
	require.Nil(t, snapshot.ScheduledForDeletion())

	current := notificationSnapshot()
	current.DeletionScheduled = util.Ptr(time.Now())
	api, r := newFakeAPI(t, http.StatusOK, current)

	log := logr.Discard()
	_, err := v0SecretDefinitionCreated(r, snapshot, &log)

	require.NoError(t, err)
	assert.Equal(t, []string{"/v0/secret-definitions/7"}, api.paths(),
		"a definition scheduled for deletion since the notification is not pushed")
}

func TestSecretDefinitionCreatedPushesALiveDefinitionFromTheNotification(t *testing.T) {
	// the API returns the definition without its data, as it does not store it
	current := notificationSnapshot()
	current.Data = nil
	api, r := newFakeAPI(t, http.StatusOK, current)

	log := logr.Discard()
	_, err := v0SecretDefinitionCreated(r, notificationSnapshot(), &log)

	// the push goes on to look up the provider, which the fake API does not have
	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to push secret")
	assert.Equal(t, []string{"/v0/secret-definitions/7", "/v0/aws-providers/3"}, api.paths(),
		"a live definition is pushed")
}
