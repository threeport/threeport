package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	echo "github.com/labstack/echo/v4"
	nats "github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	zap "go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	awsnotif "github.com/threeport/threeport/internal/aws/notif"
	gcpnotif "github.com/threeport/threeport/internal/gcp/notif"
	lib "github.com/threeport/threeport/pkg/api/lib/v0"
	api "github.com/threeport/threeport/pkg/api/v0"
	auth "github.com/threeport/threeport/pkg/auth/v0"
	notifications "github.com/threeport/threeport/pkg/notifications/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// publishedMessage is a message a recordingJetStream was asked to publish.
type publishedMessage struct {
	subject string
	payload []byte
}

// recordingJetStream records what is published on it.
type recordingJetStream struct {
	nats.JetStreamContext

	mu        sync.Mutex
	published []publishedMessage
	// The number of publishes still to fail before they succeed
	failures int
}

// Publish records the message, or fails while failures remain.
func (r *recordingJetStream) Publish(subject string, data []byte, _ ...nats.PubOpt) (*nats.PubAck, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failures > 0 {
		r.failures--
		return nil, errors.New("nats: no responders available for request")
	}
	r.published = append(r.published, publishedMessage{subject: subject, payload: data})
	return &nats.PubAck{}, nil
}

// newUpgradeHandler returns a handler on an in-memory database holding the
// EKS and GKE definitions and instances, and the stream it publishes on.
func newUpgradeHandler(t *testing.T) (Handler, *recordingJetStream) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&api.AwsEksKubernetesRuntimeDefinition{},
		&api.AwsEksKubernetesRuntimeInstance{},
		&api.GcpGkeKubernetesRuntimeDefinition{},
		&api.GcpGkeKubernetesRuntimeInstance{},
		&api.AttachedObjectReference{},
	))
	stream := &recordingJetStream{}

	return Handler{DB: db, JS: stream, Logger: zap.NewNop()}, stream
}

// runDefinitionUpdate runs middleware around a stand-in for an update handler
// of the definition with id that responds with status.
func runDefinitionUpdate(t *testing.T, middleware echo.MiddlewareFunc, id string, status int) {
	t.Helper()

	request := httptest.NewRequest(http.MethodPatch, "/", nil)
	request = request.WithContext(lib.WithCaller(request.Context(), lib.CallerIdentity{
		OrganizationalUnit: auth.OUControlPlane,
	}))
	c := echo.New().NewContext(request, httptest.NewRecorder())
	c.SetParamNames("id")
	c.SetParamValues(id)

	require.NoError(t, middleware(func(c echo.Context) error {
		return c.NoContent(status)
	})(c))
}

// TestAGkeDefinitionUpdateAsksItsClustersToUpgrade covers the update of a
// GKE definition: each of its instances not yet asked for the definition's
// version is marked unreconciled, has a refused or failed upgrade cleared,
// records the version, and its controller is notified.  An instance already
// asked, another definition's instance, and an update that wasn't saved are
// left alone.
func TestAGkeDefinitionUpdateAsksItsClustersToUpgrade(t *testing.T) {
	h, stream := newUpgradeHandler(t)
	seed := h.DB.Session(&gorm.Session{SkipHooks: true})
	require.NoError(t, seed.Create(&api.GcpGkeKubernetesRuntimeDefinition{
		Common: api.Common{ID: util.Ptr(uint(3))}, Definition: api.Definition{Name: util.Ptr("gke")},
		ZoneCount: util.Ptr(3), DefaultNodeGroupInstanceType: util.Ptr("e2-medium"), DefaultNodeGroupInitialSize: util.Ptr(1),
		DefaultNodeGroupMinimumSize: util.Ptr(1), DefaultNodeGroupMaximumSize: util.Ptr(3),
		KubernetesRuntimeDefinitionID: util.Ptr(uint(1)), KubernetesVersion: util.Ptr("1.36"),
	}).Error)
	instance := func(id, definitionID uint, requested *string) *api.GcpGkeKubernetesRuntimeInstance {
		return &api.GcpGkeKubernetesRuntimeInstance{
			Common:                              api.Common{ID: util.Ptr(id)},
			Instance:                            api.Instance{Name: util.Ptr(fmt.Sprintf("gke-%d", id))},
			Reconciliation:                      api.Reconciliation{Reconciled: util.Ptr(true)},
			GcpGkeKubernetesRuntimeDefinitionID: util.Ptr(definitionID),
			GcpProviderID:                       util.Ptr(uint(1)),
			KubernetesRuntimeInstanceID:         util.Ptr(id),
			UpgradeRequestedVersion:             requested,
			UpgradeRefused:                      util.Ptr(true),
			UpgradeFailed:                       util.Ptr(true),
		}
	}
	require.NoError(t, seed.Create(instance(21, 3, util.Ptr("1.35"))).Error)
	require.NoError(t, seed.Create(instance(22, 3, nil)).Error)
	require.NoError(t, seed.Create(instance(23, 3, util.Ptr("1.36"))).Error)
	require.NoError(t, seed.Create(instance(24, 4, util.Ptr("1.35"))).Error)
	load := func(id uint) api.GcpGkeKubernetesRuntimeInstance {
		var loaded api.GcpGkeKubernetesRuntimeInstance
		require.NoError(t, h.DB.First(&loaded, id).Error)
		return loaded
	}

	runDefinitionUpdate(t, h.RequestGcpGkeUpgrades, "3", http.StatusBadRequest)
	assert.Empty(t, stream.published, "an update that wasn't saved asks nothing")
	assert.True(t, *load(21).Reconciled)

	runDefinitionUpdate(t, h.RequestGcpGkeUpgrades, "3", http.StatusOK)
	for _, id := range []uint{21, 22} {
		asked := load(id)
		assert.False(t, *asked.Reconciled, "instance %d", id)
		assert.Equal(t, "1.36", *asked.UpgradeRequestedVersion, "instance %d", id)
		assert.False(t, *asked.UpgradeRefused, "instance %d", id)
		assert.False(t, *asked.UpgradeFailed, "instance %d", id)
	}
	assert.True(t, *load(23).Reconciled, "already asked for the version")
	assert.True(t, *load(23).UpgradeRefused)
	assert.True(t, *load(24).Reconciled, "another definition's")

	require.Len(t, stream.published, 2)
	for _, message := range stream.published {
		assert.Equal(t, gcpnotif.GcpGkeKubernetesRuntimeInstanceUpdateSubject, message.subject)
		var notification notifications.Notification
		require.NoError(t, json.Unmarshal(message.payload, &notification))
		assert.EqualValues(t, notifications.NotificationOperationUpdated, notification.Operation)
	}

	// the next update finds every instance asked
	runDefinitionUpdate(t, h.RequestGcpGkeUpgrades, "3", http.StatusOK)
	assert.Len(t, stream.published, 2)
}

// TestAnEksDefinitionUpdateAsksItsClustersToUpgrade covers the same request
// for an EKS definition, whose version defaults when unset.
func TestAnEksDefinitionUpdateAsksItsClustersToUpgrade(t *testing.T) {
	h, stream := newUpgradeHandler(t)
	seed := h.DB.Session(&gorm.Session{SkipHooks: true})
	require.NoError(t, seed.Create(&api.AwsEksKubernetesRuntimeDefinition{
		Common: api.Common{ID: util.Ptr(uint(3))}, Definition: api.Definition{Name: util.Ptr("eks")},
		ZoneCount: util.Ptr(2), DefaultNodeGroupInstanceType: util.Ptr("t3.medium"), DefaultNodeGroupInitialSize: util.Ptr(1),
		DefaultNodeGroupMinimumSize: util.Ptr(1), DefaultNodeGroupMaximumSize: util.Ptr(3),
		KubernetesRuntimeDefinitionID: util.Ptr(uint(1)),
	}).Error)
	require.NoError(t, seed.Create(&api.AwsEksKubernetesRuntimeInstance{
		Common:                              api.Common{ID: util.Ptr(uint(21))},
		Instance:                            api.Instance{Name: util.Ptr("eks")},
		Reconciliation:                      api.Reconciliation{Reconciled: util.Ptr(true)},
		AwsEksKubernetesRuntimeDefinitionID: util.Ptr(uint(3)),
		AwsProviderID:                       util.Ptr(uint(1)),
		KubernetesRuntimeInstanceID:         util.Ptr(uint(11)),
	}).Error)

	runDefinitionUpdate(t, h.RequestAwsEksUpgrades, "3", http.StatusOK)

	var asked api.AwsEksKubernetesRuntimeInstance
	require.NoError(t, h.DB.First(&asked, 21).Error)
	assert.False(t, *asked.Reconciled)
	assert.Equal(t, api.DefaultKubernetesVersion, *asked.UpgradeRequestedVersion)
	require.Len(t, stream.published, 1)
	assert.Equal(t, awsnotif.AwsEksKubernetesRuntimeInstanceUpdateSubject, stream.published[0].subject)

	// the routes run the middleware on both update shapes, and nothing else
	assert.Len(t, h.PatchAwsEksKubernetesRuntimeDefinitionMiddleware(), 1)
	assert.Len(t, h.PutAwsEksKubernetesRuntimeDefinitionMiddleware(), 1)
	assert.Len(t, h.PatchGcpGkeKubernetesRuntimeDefinitionMiddleware(), 1)
	assert.Len(t, h.PutGcpGkeKubernetesRuntimeDefinitionMiddleware(), 1)
	assert.Empty(t, h.AddAwsEksKubernetesRuntimeDefinitionMiddleware())
	assert.Empty(t, h.DeleteGcpGkeKubernetesRuntimeDefinitionMiddleware())
}

// TestAnUpgradeRequestWhoseNotificationFailedIsAskedAgain covers a
// notification that fails to publish: the instance isn't recorded as asked,
// so the next update of the definition asks it again, and records it once its
// controller has been told.
func TestAnUpgradeRequestWhoseNotificationFailedIsAskedAgain(t *testing.T) {
	h, stream := newUpgradeHandler(t)
	stream.failures = 1
	seed := h.DB.Session(&gorm.Session{SkipHooks: true})
	require.NoError(t, seed.Create(&api.GcpGkeKubernetesRuntimeDefinition{
		Common: api.Common{ID: util.Ptr(uint(3))}, Definition: api.Definition{Name: util.Ptr("gke")},
		ZoneCount: util.Ptr(3), DefaultNodeGroupInstanceType: util.Ptr("e2-medium"), DefaultNodeGroupInitialSize: util.Ptr(1),
		DefaultNodeGroupMinimumSize: util.Ptr(1), DefaultNodeGroupMaximumSize: util.Ptr(3),
		KubernetesRuntimeDefinitionID: util.Ptr(uint(1)), KubernetesVersion: util.Ptr("1.36"),
	}).Error)
	require.NoError(t, seed.Create(&api.GcpGkeKubernetesRuntimeInstance{
		Common:                              api.Common{ID: util.Ptr(uint(21))},
		Instance:                            api.Instance{Name: util.Ptr("gke-21")},
		Reconciliation:                      api.Reconciliation{Reconciled: util.Ptr(true)},
		GcpGkeKubernetesRuntimeDefinitionID: util.Ptr(uint(3)),
		GcpProviderID:                       util.Ptr(uint(1)),
		KubernetesRuntimeInstanceID:         util.Ptr(uint(21)),
	}).Error)
	requested := func() *string {
		var loaded api.GcpGkeKubernetesRuntimeInstance
		require.NoError(t, h.DB.First(&loaded, 21).Error)
		return loaded.UpgradeRequestedVersion
	}

	runDefinitionUpdate(t, h.RequestGcpGkeUpgrades, "3", http.StatusOK)
	assert.Empty(t, stream.published)
	assert.Nil(t, requested(), "its controller wasn't told, so it isn't recorded as asked")

	runDefinitionUpdate(t, h.RequestGcpGkeUpgrades, "3", http.StatusOK)
	assert.Len(t, stream.published, 1)
	assert.Equal(t, util.Ptr("1.36"), requested())
}
