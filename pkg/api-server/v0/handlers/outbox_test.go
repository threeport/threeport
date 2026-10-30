package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v4"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	sqlite "gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/threeport/threeport/internal/kubernetes-workload/notif"
	secretnotif "github.com/threeport/threeport/internal/secret/notif"
	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	"github.com/threeport/threeport/pkg/encryption/v0"
	notifications "github.com/threeport/threeport/pkg/notifications/v0"
)

// A write that has committed must leave a notification on record even when
// JetStream will not take it. These tests drive generated handlers with a
// JetStream context whose publish fails, which is what a full NATS volume does.

const workloadDefinitionsRoute = "/v0/kubernetes-workload-definitions"

// fakeJetStream stands in for a JetStream context. Only Publish is ever called.
type fakeJetStream struct {
	nats.JetStreamContext

	mu        sync.Mutex
	err       error
	published []string
}

// failWith makes every later publish fail with err. A nil err heals it.
func (f *fakeJetStream) failWith(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

func (f *fakeJetStream) Publish(subj string, data []byte, opts ...nats.PubOpt) (*nats.PubAck, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.err != nil {
		return nil, f.err
	}
	f.published = append(f.published, subj)

	return &nats.PubAck{}, nil
}

// publishedSubjects returns the subjects published so far.
func (f *fakeJetStream) publishedSubjects() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.published...)
}

var errVolumeFull = errors.New("nats: JetStream resource limits exceeded for server")

// newOutboxHandler returns a handler over an empty sqlite database.
func newOutboxHandler(t *testing.T, js *fakeJetStream) Handler {
	t.Helper()

	name := strings.ReplaceAll(t.Name(), "/", "_")
	db, err := gorm.Open(
		sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", name)),
		&gorm.Config{},
	)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&api_v0.KubernetesWorkloadDefinition{},
		&api_v0.KubernetesWorkloadInstance{},
		&api_v0.KubernetesWorkloadResourceDefinition{},
		&api_v0.SecretDefinition{},
		&api_v0.AttachedObjectReference{},
		&api_v0.OutboxMessage{},
		&api_v0.Event{},
	))

	registerValidateTags(api_v0.ObjectTypeKubernetesWorkloadDefinition, new(api_v0.KubernetesWorkloadDefinition))
	registerValidateTags(api_v0.ObjectTypeSecretDefinition, new(api_v0.SecretDefinition))

	return Handler{DB: db, JS: js, Logger: zap.NewNop()}
}

// newOutboxRequest returns a request context wired like the API server. A
// non-empty id is set as the route's :id.
func newOutboxRequest(method, route, id, body string) (*apiserver_lib.CustomContext, *httptest.ResponseRecorder) {
	e := echo.New()
	e.Binder = apiserver_lib.NewQueryBinder()

	validate := validator.New()
	validate.RegisterValidation("optional", apiserver_lib.IsOptional)
	validate.RegisterValidation("association", apiserver_lib.IsAssociation)
	validate.RegisterValidation("ISO8601date", apiserver_lib.IsISO8601Date)
	e.Validator = &apiserver_lib.CustomValidator{Validator: validate}

	req := httptest.NewRequest(method, route, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)

	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if id != "" {
		c.SetPath(route + "/:id")
		c.SetParamNames("id")
		c.SetParamValues(id)
	} else {
		c.SetPath(route)
	}

	return &apiserver_lib.CustomContext{Context: c}, rec
}

// pendingMessages returns the outbox messages that have not been published.
func pendingMessages(t *testing.T, h Handler) []api_v0.OutboxMessage {
	t.Helper()

	var messages []api_v0.OutboxMessage
	require.NoError(t, h.DB.Where("published_at IS NULL").Order("id asc").Find(&messages).Error)

	return messages
}

// addWorkloadDefinition creates a workload definition through the handler and
// returns its id.
func addWorkloadDefinition(t *testing.T, h Handler, name string) uint {
	t.Helper()

	c, rec := newOutboxRequest(http.MethodPost, workloadDefinitionsRoute, "",
		fmt.Sprintf(`{"Name":%q,"YAMLDocument":"kind: Namespace"}`, name))
	require.NoError(t, h.AddKubernetesWorkloadDefinition(c))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	var definition api_v0.KubernetesWorkloadDefinition
	require.NoError(t, h.DB.Where("name = ?", name).First(&definition).Error)

	return *definition.ID
}

func TestCreateWithFailedPublishSucceedsAndLeavesNotificationOnRecord(t *testing.T) {
	js := &fakeJetStream{}
	js.failWith(errVolumeFull)
	h := newOutboxHandler(t, js)

	c, rec := newOutboxRequest(http.MethodPost, workloadDefinitionsRoute, "",
		`{"Name":"my-app","YAMLDocument":"kind: Namespace"}`)
	require.NoError(t, h.AddKubernetesWorkloadDefinition(c))

	assert.Equal(t, http.StatusCreated, rec.Code,
		"the row committed, so the client is told it did")

	var count int64
	require.NoError(t, h.DB.Model(&api_v0.KubernetesWorkloadDefinition{}).Count(&count).Error)
	require.EqualValues(t, 1, count)

	pending := pendingMessages(t, h)
	require.Len(t, pending, 1, "the notification the write owes is on record")
	assert.Equal(t, notif.KubernetesWorkloadDefinitionCreateSubject, pending[0].Subject)
	assert.Equal(t, "threeport.io/v0.KubernetesWorkloadDefinition", pending[0].ObjectType)
	assert.NotZero(t, pending[0].ObjectID)
	assert.Equal(t, 1, pending[0].Attempts, "the failed publish is counted")
	assert.Contains(t, pending[0].LastError, "resource limits exceeded")
	assert.Contains(t, string(pending[0].Payload), string(notifications.NotificationOperationCreated))
	assert.Empty(t, js.publishedSubjects())
}

func TestCreateWithWorkingPublishStampsTheMessage(t *testing.T) {
	js := &fakeJetStream{}
	h := newOutboxHandler(t, js)

	addWorkloadDefinition(t, h, "my-app")

	assert.Equal(t, []string{notif.KubernetesWorkloadDefinitionCreateSubject}, js.publishedSubjects())
	assert.Empty(t, pendingMessages(t, h), "an acknowledged notification is not left pending")

	var message api_v0.OutboxMessage
	require.NoError(t, h.DB.First(&message).Error)
	assert.NotNil(t, message.PublishedAt)
}

func TestCreateThatNeedsNoReconciliationRecordsNothing(t *testing.T) {
	js := &fakeJetStream{}
	h := newOutboxHandler(t, js)

	c, rec := newOutboxRequest(http.MethodPost, workloadDefinitionsRoute, "",
		`{"Name":"my-app","YAMLDocument":"kind: Namespace","Reconciled":true}`)
	require.NoError(t, h.AddKubernetesWorkloadDefinition(c))
	require.Equal(t, http.StatusCreated, rec.Code)

	var count int64
	require.NoError(t, h.DB.Model(&api_v0.OutboxMessage{}).Count(&count).Error)
	assert.Zero(t, count)
	assert.Empty(t, js.publishedSubjects())
}

func TestRejectedCreateLeavesNothingOnRecord(t *testing.T) {
	js := &fakeJetStream{}
	h := newOutboxHandler(t, js)
	addWorkloadDefinition(t, h, "my-app")

	// the name is taken, so the write fails and rolls back
	c, rec := newOutboxRequest(http.MethodPost, workloadDefinitionsRoute, "",
		`{"Name":"my-app","YAMLDocument":"kind: Namespace"}`)
	require.NoError(t, h.AddKubernetesWorkloadDefinition(c))
	assert.GreaterOrEqual(t, rec.Code, http.StatusBadRequest)

	var count int64
	require.NoError(t, h.DB.Model(&api_v0.OutboxMessage{}).Count(&count).Error)
	assert.EqualValues(t, 1, count, "a write that did not commit owes no notification")
	assert.Equal(t, []string{notif.KubernetesWorkloadDefinitionCreateSubject}, js.publishedSubjects())
}

// The notification and the write share a fate: when the notification cannot be
// recorded the write is rolled back and the client is told, so the retry works.
func TestWriteIsRolledBackWhenNotificationCannotBeRecorded(t *testing.T) {
	js := &fakeJetStream{}
	h := newOutboxHandler(t, js)
	require.NoError(t, h.DB.Migrator().DropTable(&api_v0.OutboxMessage{}))

	c, rec := newOutboxRequest(http.MethodPost, workloadDefinitionsRoute, "",
		`{"Name":"my-app","YAMLDocument":"kind: Namespace"}`)
	require.NoError(t, h.AddKubernetesWorkloadDefinition(c))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	var count int64
	require.NoError(t, h.DB.Model(&api_v0.KubernetesWorkloadDefinition{}).Count(&count).Error)
	assert.Zero(t, count, "the object must not be left behind without its notification")
}

func TestUpdateWithFailedPublishLeavesNotificationOnRecord(t *testing.T) {
	js := &fakeJetStream{}
	h := newOutboxHandler(t, js)
	id := addWorkloadDefinition(t, h, "my-app")

	js.failWith(errVolumeFull)
	c, rec := newOutboxRequest(http.MethodPatch, workloadDefinitionsRoute, fmt.Sprint(id),
		`{"YAMLDocument":"kind: ConfigMap"}`)
	require.NoError(t, h.UpdateKubernetesWorkloadDefinition(c))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	pending := pendingMessages(t, h)
	require.Len(t, pending, 1)
	assert.Equal(t, notif.KubernetesWorkloadDefinitionUpdateSubject, pending[0].Subject)
	assert.Equal(t, id, pending[0].ObjectID)
	assert.Contains(t, string(pending[0].Payload), "kind: ConfigMap",
		"the notification carries the updated object")
}

func TestReplaceWithFailedPublishLeavesNotificationOnRecord(t *testing.T) {
	js := &fakeJetStream{}
	h := newOutboxHandler(t, js)
	id := addWorkloadDefinition(t, h, "my-app")

	js.failWith(errVolumeFull)
	c, rec := newOutboxRequest(http.MethodPut, workloadDefinitionsRoute, fmt.Sprint(id),
		`{"Name":"my-app","YAMLDocument":"kind: ConfigMap"}`)
	require.NoError(t, h.ReplaceKubernetesWorkloadDefinition(c))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	pending := pendingMessages(t, h)
	require.Len(t, pending, 1)
	assert.Equal(t, notif.KubernetesWorkloadDefinitionUpdateSubject, pending[0].Subject)
	assert.Contains(t, string(pending[0].Payload), "kind: ConfigMap",
		"the notification carries the stored object")
}

// The case from the issue: a delete is accepted and the row is marked for
// deletion, but the notification never went out. A repeated delete cannot help,
// because it gets the 409 and publishes nothing, so the notification has to
// come from the record that was left behind.
func TestDeleteWithFailedPublishIsRecoveredBySweeper(t *testing.T) {
	js := &fakeJetStream{}
	h := newOutboxHandler(t, js)
	id := addWorkloadDefinition(t, h, "my-app")
	require.NoError(t, h.DB.Model(&api_v0.OutboxMessage{}).Where("1 = 1").Delete(&api_v0.OutboxMessage{}).Error)

	published := js.publishedSubjects()
	js.failWith(errVolumeFull)
	c, rec := newOutboxRequest(http.MethodDelete, workloadDefinitionsRoute, fmt.Sprint(id), "")
	require.NoError(t, h.DeleteKubernetesWorkloadDefinition(c))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var definition api_v0.KubernetesWorkloadDefinition
	require.NoError(t, h.DB.First(&definition, id).Error)
	require.NotNil(t, definition.DeletionScheduled, "the row is scheduled for deletion")

	// a repeat DELETE is the 409 from the issue and publishes nothing
	c, rec = newOutboxRequest(http.MethodDelete, workloadDefinitionsRoute, fmt.Sprint(id), "")
	require.NoError(t, h.DeleteKubernetesWorkloadDefinition(c))
	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Equal(t, published, js.publishedSubjects())

	pending := pendingMessages(t, h)
	require.Len(t, pending, 1, "the delete notification is on record")
	assert.Equal(t, notif.KubernetesWorkloadDefinitionDeleteSubject, pending[0].Subject)
	assert.Contains(t, string(pending[0].Payload), string(notifications.NotificationOperationDeleted))

	// NATS recovers and the sweeper delivers it
	js.failWith(nil)
	require.NoError(t, h.DB.Model(&api_v0.OutboxMessage{}).Where("1 = 1").
		UpdateColumn("created_at", time.Now().UTC().Add(-time.Minute)).Error)
	swept, failed, err := apiserver_lib.SweepOutbox(h.DB, h.JS, h.Logger)
	require.NoError(t, err)
	assert.False(t, failed)
	assert.Equal(t, 1, swept)
	assert.Equal(t,
		append(published, notif.KubernetesWorkloadDefinitionDeleteSubject),
		js.publishedSubjects())
	assert.Empty(t, pendingMessages(t, h))
}

func TestSecretCreateWithFailedPublishLeavesNotificationOnRecord(t *testing.T) {
	t.Setenv(encryption.KeyEnvVar, "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	js := &fakeJetStream{}
	js.failWith(errVolumeFull)
	h := newOutboxHandler(t, js)

	c, rec := newOutboxRequest(http.MethodPost, "/v0/secret-definitions", "",
		`{"Name":"my-secret","Data":{"key":"value"}}`)
	require.NoError(t, h.CustomAddSecretDefinition(func(echo.Context) error { return nil })(c))
	assert.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	pending := pendingMessages(t, h)
	require.Len(t, pending, 1)
	assert.Equal(t, secretnotif.SecretDefinitionCreateSubject, pending[0].Subject)
	assert.NotContains(t, string(pending[0].Payload), `"value"`,
		"the notification carries the encrypted data, never the plaintext")
}
