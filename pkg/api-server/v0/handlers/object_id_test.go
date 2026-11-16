package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	echo "github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	zap "go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	api "github.com/threeport/threeport/pkg/api/v0"
	encryption "github.com/threeport/threeport/pkg/encryption/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// byIDHandler is one handler that reads an object ID from its path. The
// generated handlers all come from one template, so one object type stands
// for every object. The module object handler is hand-written and has its own
// row.
type byIDHandler struct {
	name    string
	method  string
	handler func(Handler, echo.Context) error
}

var byIDHandlers = []byIDHandler{
	{
		name:    "generated get",
		method:  http.MethodGet,
		handler: func(h Handler, c echo.Context) error { return h.GetKubernetesRuntimeInstance(c) },
	},
	{
		name:    "generated update",
		method:  http.MethodPatch,
		handler: func(h Handler, c echo.Context) error { return h.UpdateKubernetesRuntimeInstance(c) },
	},
	{
		name:    "generated replace",
		method:  http.MethodPut,
		handler: func(h Handler, c echo.Context) error { return h.ReplaceKubernetesRuntimeInstance(c) },
	},
	{
		name:    "generated delete",
		method:  http.MethodDelete,
		handler: func(h Handler, c echo.Context) error { return h.DeleteKubernetesRuntimeInstance(c) },
	},
	{
		name:    "module object get",
		method:  http.MethodGet,
		handler: func(h Handler, c echo.Context) error { return h.GetModuleObjectWithModuleApiRoutes(c) },
	},
}

// TestByIDHandlersRejectIDsThatAreNotNumbers covers path IDs GORM would
// otherwise read as SQL. Before the handlers parsed the ID, a GET for
// `1 OR 1=1` answered 200 with the oldest row of the type.
func TestByIDHandlersRejectIDsThatAreNotNumbers(t *testing.T) {
	for _, id := range []string{
		"abc",
		"1 OR 1=1",
		"1) OR (1=1",
		"1; DROP TABLE kubernetes_runtime_instances",
		"-1",
		"+1",
		"1.5",
		"0x10",
		" 1",
		"",
		"18446744073709551616",
	} {
		for _, test := range byIDHandlers {
			t.Run(test.name+"/"+id, func(t *testing.T) {
				h, queries := newObjectIDHandler(t)
				c, rec := newByIDRequest(test.method, id)

				require.NoError(t, test.handler(h, c))

				assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
				assert.Contains(t, rec.Body.String(), "invalid id",
					"the response says which input was rejected")
				assert.Zero(t, *queries, "the database is not queried with an ID that was rejected")

				var runtimes []api.KubernetesRuntimeInstance
				require.NoError(t, h.DB.Order("id").Find(&runtimes).Error)
				require.Len(t, runtimes, 2, "no runtime is deleted")
				assert.Equal(t, "first", *runtimes[0].Name, "no runtime is changed")
				assert.Equal(t, "second", *runtimes[1].Name, "no runtime is changed")
			})
		}
	}
}

// TestByIDHandlersLookUpNumericIDs is the control for the test above: a
// numeric ID still reaches the database. The ID names no row, so every handler
// answers 404 from its lookup before it would read a body.
func TestByIDHandlersLookUpNumericIDs(t *testing.T) {
	for _, test := range byIDHandlers {
		t.Run(test.name, func(t *testing.T) {
			h, queries := newObjectIDHandler(t)
			c, rec := newByIDRequest(test.method, "99")

			require.NoError(t, test.handler(h, c))

			assert.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
			assert.NotZero(t, *queries)
		})
	}

	t.Run("generated get returns the row it names", func(t *testing.T) {
		h, _ := newObjectIDHandler(t)
		c, rec := newByIDRequest(http.MethodGet, "2")

		require.NoError(t, h.GetKubernetesRuntimeInstance(c))

		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.Contains(t, rec.Body.String(), `"Name":"second"`)
		assert.NotContains(t, rec.Body.String(), `"Name":"first"`)
	})
}

// newObjectIDHandler returns a handler whose sqlite database holds two
// runtimes, and a count of the statements run against it once they are
// written.
func newObjectIDHandler(t *testing.T) (Handler, *int) {
	t.Helper()

	// the update and replace handlers check the body against the type's
	// tagged fields and the writes encrypt the runtime's credentials, so an ID
	// that got past the parse would go on to write
	registerTaggedFields(api.ObjectTypeKubernetesRuntimeInstance, new(api.KubernetesRuntimeInstance))
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	t.Setenv(encryption.KeyEnvVar, key)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&api.KubernetesRuntimeInstance{},
		&api.ModuleObject{},
		&api.ModuleApiRoute{},
		&api.AttachedObjectReference{},
	))
	for _, name := range []string{"first", "second"} {
		require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).Create(&api.KubernetesRuntimeInstance{
			Instance:                      api.Instance{Name: util.Ptr(name)},
			Location:                      util.Ptr("local"),
			KubernetesRuntimeDefinitionID: util.Ptr(uint(1)),
		}).Error)
	}

	queries := 0
	count := func(*gorm.DB) { queries++ }
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:count", count))
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:count", count))
	require.NoError(t, db.Callback().Delete().Before("gorm:delete").Register("test:count", count))
	require.NoError(t, db.Callback().Row().Before("gorm:row").Register("test:count", count))
	require.NoError(t, db.Callback().Raw().Before("gorm:raw").Register("test:count", count))

	return Handler{DB: db, Logger: zap.NewNop()}, &queries
}

// newByIDRequest builds a request whose path carries id, already decoded, the
// way echo hands it to a handler.
func newByIDRequest(method, id string) (*apiserver_lib.CustomContext, *httptest.ResponseRecorder) {
	e := echo.New()
	var body *strings.Reader
	if method == http.MethodPatch || method == http.MethodPut {
		body = strings.NewReader(`{"Name":"changed"}`)
	} else {
		body = strings.NewReader("")
	}
	req := httptest.NewRequest(method, api.PathKubernetesRuntimeInstances+"/x", body)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath(api.PathKubernetesRuntimeInstances + "/:id")
	c.SetParamNames("id")
	c.SetParamValues(id)

	return &apiserver_lib.CustomContext{Context: c}, rec
}
