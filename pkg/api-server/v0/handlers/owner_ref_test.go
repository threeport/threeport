package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-playground/validator/v10"
	echo "github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	zap "go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	lib "github.com/threeport/threeport/pkg/api/lib/v0"
	api "github.com/threeport/threeport/pkg/api/v0"
	auth "github.com/threeport/threeport/pkg/auth/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestGetRuntimeShowsWorkloadOwner covers a workload create that names its
// runtime as owner. A later GET of that runtime returns the owned workload id.
func TestGetRuntimeShowsWorkloadOwner(t *testing.T) {
	registerTaggedFields(api.ObjectTypeKubernetesWorkloadInstance, new(api.KubernetesWorkloadInstance))
	h := newOwnerHandler(t)

	// seed the owner outside the create under test
	runtime := &api.KubernetesRuntimeInstance{
		Instance:                      api.Instance{Name: util.Ptr("runtime")},
		Location:                      util.Ptr("local"),
		KubernetesRuntimeDefinitionID: util.Ptr(uint(1)),
	}
	require.NoError(t, h.DB.Session(&gorm.Session{SkipHooks: true}).Create(runtime).Error)

	// create the workload through the add handler
	created := postWorkload(t, h, map[string]any{
		"Name":                           "owned",
		"Reconciled":                     true,
		"KubernetesRuntimeInstanceID":    runtime.ID,
		"KubernetesWorkloadDefinitionID": uint(2),
		"Owner": map[string]any{
			"Type":  runtime.GetFullyQualifiedType(),
			"ID":    runtime.ID,
			"Field": "GatewayControllerInstanceID",
		},
	})

	// read the owner back through its get handler
	got := getRuntime(t, h, *runtime.ID)
	require.NotNil(t, got.GatewayControllerInstanceID)
	assert.Equal(t, created, *got.GatewayControllerInstanceID)
}

// TestCreateRejectsPartialOwner covers a create whose owner omits a field.
// The handler rejects it before any workload row is written.
func TestCreateRejectsPartialOwner(t *testing.T) {
	registerTaggedFields(api.ObjectTypeKubernetesWorkloadInstance, new(api.KubernetesWorkloadInstance))
	h := newOwnerHandler(t)

	// post an owner that names a type and an id and leaves the field unset
	raw, err := json.Marshal(map[string]any{
		"Name":                           "partial",
		"Reconciled":                     true,
		"KubernetesRuntimeInstanceID":    uint(1),
		"KubernetesWorkloadDefinitionID": uint(2),
		"Owner": map[string]any{
			"Type": "KubernetesRuntimeInstance",
			"ID":   uint(1),
		},
	})
	require.NoError(t, err)
	e := echo.New()
	validate := validator.New()
	require.NoError(t, validate.RegisterValidation("optional", apiserver_lib.IsOptional))
	require.NoError(t, validate.RegisterValidation("association", apiserver_lib.IsAssociation))
	e.Validator = &apiserver_lib.CustomValidator{Validator: validate}
	req := httptest.NewRequest(http.MethodPost, api.PathKubernetesWorkloadInstances, bytes.NewReader(raw))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath(api.PathKubernetesWorkloadInstances)

	// the handler returns the missing-field status and writes nothing
	require.NoError(t, h.AddKubernetesWorkloadInstance(&apiserver_lib.CustomContext{Context: c}))
	assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	var count int64
	require.NoError(t, h.DB.Model(&api.KubernetesWorkloadInstance{}).Count(&count).Error)
	assert.Equal(t, int64(0), count)
}

// newOwnerHandler returns a handler whose sqlite database holds a runtime, a
// workload, and the reference rows a create writes with them.
func newOwnerHandler(t *testing.T) Handler {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&api.KubernetesWorkloadInstance{},
		&api.KubernetesRuntimeInstance{},
		&api.AttachedObjectReference{},
	))

	return Handler{DB: db, Logger: zap.NewNop()}
}

// postWorkload posts one workload and returns the id from the created body.
func postWorkload(t *testing.T, h Handler, body map[string]any) uint {
	t.Helper()

	raw, err := json.Marshal(body)
	require.NoError(t, err)

	e := echo.New()
	validate := validator.New()
	require.NoError(t, validate.RegisterValidation("optional", apiserver_lib.IsOptional))
	require.NoError(t, validate.RegisterValidation("association", apiserver_lib.IsAssociation))
	e.Validator = &apiserver_lib.CustomValidator{Validator: validate}

	req := httptest.NewRequest(http.MethodPost, api.PathKubernetesWorkloadInstances, bytes.NewReader(raw))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req = req.WithContext(lib.WithCaller(req.Context(), lib.CallerIdentity{
		OrganizationalUnit: auth.OUControlPlane,
	}))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath(api.PathKubernetesWorkloadInstances)

	require.NoError(t, h.AddKubernetesWorkloadInstance(&apiserver_lib.CustomContext{Context: c}))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	var envelope struct {
		Data []api.KubernetesWorkloadInstance
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	require.Len(t, envelope.Data, 1)
	require.NotNil(t, envelope.Data[0].ID)

	return *envelope.Data[0].ID
}

// getRuntime gets one runtime by id and returns the decoded object.
func getRuntime(t *testing.T, h Handler, id uint) api.KubernetesRuntimeInstance {
	t.Helper()

	e := echo.New()
	path := fmt.Sprintf("%s/%d", api.PathKubernetesRuntimeInstances, id)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(fmt.Sprintf("%d", id))

	require.NoError(t, h.GetKubernetesRuntimeInstance(&apiserver_lib.CustomContext{Context: c}))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var envelope struct {
		Data []api.KubernetesRuntimeInstance
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	require.Len(t, envelope.Data, 1)

	return envelope.Data[0]
}
