package v0

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

func TestSyncServiceAccountEnabled(t *testing.T) {
	yes, no := util.Ptr(true), util.Ptr(false)

	tests := []struct {
		name     string
		settings []*bool
		want     bool
	}{
		{"nothing set is synced", nil, true},
		{"unset values are synced", []*bool{nil, nil}, true},
		{"true is synced", []*bool{yes}, true},
		{"false is not synced", []*bool{no}, false},
		{"false on the stored provider holds when the config is unset", []*bool{nil, no}, false},
		{"false on the stored provider holds against a true in the config", []*bool{yes, no}, false},
		{"false in the config holds against a stored true", []*bool{no, yes}, false},
		{"false holds when the stored provider predates the field", []*bool{no, nil}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, syncServiceAccountEnabled(tt.settings...))
		})
	}
}

// fakeGcpProviderApi serves one GcpProvider and records the last body it was
// sent. It answers a lookup with the provider, and a create, replace or delete
// with the object it was given.
type fakeGcpProviderApi struct {
	*httptest.Server

	mu      sync.Mutex
	posted  *api_v0.GcpProvider
	put     *api_v0.GcpProvider
	deleted bool
}

func newFakeGcpProviderApi(t *testing.T, existing *api_v0.GcpProvider) *fakeGcpProviderApi {
	t.Helper()

	f := &fakeGcpProviderApi{}
	reply := func(w http.ResponseWriter, code int, objects ...apiserver_lib.Object) {
		body, err := json.Marshal(apiserver_lib.Response{
			Status: apiserver_lib.Status{Code: code, Message: http.StatusText(code)},
			Data:   objects,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_, _ = w.Write(body)
	}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()

		switch r.Method {
		case http.MethodGet:
			reply(w, http.StatusOK, *existing)
		case http.MethodPost, http.MethodPut:
			var payload api_v0.GcpProvider
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			code := http.StatusOK
			if r.Method == http.MethodPost {
				f.posted = &payload
				code = http.StatusCreated
			} else {
				f.put = &payload
			}
			// the client does not send these, the API does
			payload.ID = existing.ID
			payload.CreatedAt = existing.CreatedAt
			reply(w, code, payload)
		case http.MethodDelete:
			f.deleted = true
			reply(w, http.StatusOK, *existing)
		default:
			http.Error(w, "unexpected request", http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(f.Close)

	return f
}

func (f *fakeGcpProviderApi) endpoint() string {
	return strings.TrimPrefix(f.URL, "http://")
}

func storedGcpProvider(syncServiceAccount *bool) *api_v0.GcpProvider {
	createdAt := time.Now()
	return &api_v0.GcpProvider{
		Common:             api_v0.Common{ID: util.Ptr(uint(1)), CreatedAt: &createdAt},
		Name:               util.Ptr("kept"),
		ProjectID:          util.Ptr("proj-a"),
		DefaultProvider:    util.Ptr(false),
		DefaultRegion:      util.Ptr("us-east1"),
		SyncServiceAccount: syncServiceAccount,
	}
}

func gcpProviderValues(syncServiceAccount *bool) GcpProviderConfig {
	return GcpProviderConfig{GcpProvider: GcpProviderValues{
		Name:               util.Ptr("kept"),
		ProjectID:          util.Ptr("proj-a"),
		DefaultProvider:    util.Ptr(false),
		DefaultRegion:      util.Ptr("us-east1"),
		SyncServiceAccount: syncServiceAccount,
	}}
}

// A provider created with SyncServiceAccount false has to say so on the API
// object, because tptctl down never sees the config it was created with.
func TestGcpProviderConfigCreateRecordsSyncServiceAccountFalse(t *testing.T) {
	api := newFakeGcpProviderApi(t, storedGcpProvider(util.Ptr(false)))

	config := gcpProviderValues(util.Ptr(false))
	created, err := config.Create(&http.Client{}, api.endpoint())
	require.NoError(t, err)

	require.NotNil(t, api.posted)
	require.NotNil(t, api.posted.SyncServiceAccount, "the choice must be sent to the API")
	assert.False(t, *api.posted.SyncServiceAccount)
	assert.Nil(t, api.posted.ServiceAccountCredentials, "no service account is created, so no credentials are stored")
	require.NotNil(t, created.GcpProvider.SyncServiceAccount)
	assert.False(t, *created.GcpProvider.SyncServiceAccount)
}

// Replace starts from the stored provider, but clears fields the config leaves
// out. A config with no SyncServiceAccount must not undo an opt-out.
func TestGcpProviderConfigReplaceKeepsSyncServiceAccount(t *testing.T) {
	tests := []struct {
		name   string
		stored *bool
		config *bool
		want   *bool
	}{
		{"unset in the config keeps a stored false", util.Ptr(false), nil, util.Ptr(false)},
		{"unset in the config keeps a stored true", util.Ptr(true), nil, util.Ptr(true)},
		{"unset in the config keeps a missing value missing", nil, nil, nil},
		{"the config can opt out", util.Ptr(true), util.Ptr(false), util.Ptr(false)},
		{"the config can opt back in", util.Ptr(false), util.Ptr(true), util.Ptr(true)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newFakeGcpProviderApi(t, storedGcpProvider(tt.stored))

			config := gcpProviderValues(tt.config)
			_, err := config.Replace(&http.Client{}, api.endpoint(), "kept")
			require.NoError(t, err)

			require.NotNil(t, api.put)
			assert.Equal(t, tt.want, api.put.SyncServiceAccount)
		})
	}
}

// Deleting a provider created with SyncServiceAccount false must not touch its
// GCP service account, even when the delete does not repeat the setting. If it
// did, it would call GCP, which this test has no credentials for, and fail.
func TestGcpProviderConfigDeleteKeepsServiceAccountOfOptedOutProvider(t *testing.T) {
	tests := []struct {
		name   string
		config *bool
	}{
		{"setting not repeated on delete", nil},
		{"setting repeated on delete", util.Ptr(false)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newFakeGcpProviderApi(t, storedGcpProvider(util.Ptr(false)))

			config := gcpProviderValues(tt.config)
			_, err := config.Delete(&http.Client{}, api.endpoint())
			require.NoError(t, err)
			assert.True(t, api.deleted, "the API record is still deleted")
		})
	}
}
