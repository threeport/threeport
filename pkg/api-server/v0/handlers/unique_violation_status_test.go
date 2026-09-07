package handlers

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	sqlite "gorm.io/driver/sqlite"
	"gorm.io/gorm"

	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	api_lib "github.com/threeport/threeport/pkg/api/lib/v0"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
)

// sqlite cannot produce SQLSTATE 23505, so these tests inject a *pgconn.PgError
// on create. test/cockroach covers the same handlers against a real database.

// rejectingConstraint is the index name the injected error carries.
const rejectingConstraint = "idx_test_unique_violation"

// uniqueViolationCase is one add handler driven with a rejected write.
type uniqueViolationCase struct {
	name       string
	source     string
	handler    func(Handler, echo.Context) error
	route      string
	body       string
	models     []any
	objectType string
	object     any
}

var uniqueViolationCases = []uniqueViolationCase{
	{
		name:       "generated add handler",
		source:     "kubernetes_workload_gen.go",
		handler:    func(h Handler, c echo.Context) error { return h.AddKubernetesWorkloadDefinition(c) },
		route:      "/v0/kubernetes-workload-definitions",
		body:       `{"Name":"my-app","YAMLDocument":"kind: Namespace"}`,
		models:     []any{&api_v0.KubernetesWorkloadDefinition{}},
		objectType: api_v0.ObjectTypeKubernetesWorkloadDefinition,
		object:     new(api_v0.KubernetesWorkloadDefinition),
	},
	{
		name:   "resource definition set handler",
		source: "kubernetes_workload.go",
		handler: func(h Handler, c echo.Context) error {
			return h.AddKubernetesWorkloadResourceDefinitions(c)
		},
		route:      "/v0/kubernetes-workload-resource-definition-sets",
		body:       `[{"KubernetesWorkloadDefinitionID":1,"JSONDefinition":"e30="}]`,
		models:     []any{&api_v0.KubernetesWorkloadResourceDefinition{}},
		objectType: api_v0.ObjectTypeKubernetesWorkloadResourceDefinition,
		object:     new(api_v0.KubernetesWorkloadResourceDefinition),
	},
	{
		name:   "secret definition middleware",
		source: "secret.go",
		handler: func(h Handler, c echo.Context) error {
			// the middleware never calls next, so it gets a no-op
			return h.CustomAddSecretDefinition(func(echo.Context) error { return nil })(c)
		},
		route:      "/v0/secret-definitions",
		body:       `{"Name":"my-secret","Data":{"key":"value"}}`,
		models:     []any{&api_v0.SecretDefinition{}},
		objectType: api_v0.ObjectTypeSecretDefinition,
		object:     new(api_v0.SecretDefinition),
	},
	{
		name:   "module api route handler",
		source: "module.go",
		handler: func(h Handler, c echo.Context) error {
			return h.AddModuleApiRouteWithModuleObjectReferences(c)
		},
		route:      "/v0/module-api-routes",
		body:       `{"Path":"/v0/routers","ModuleApiID":1}`,
		models:     []any{&api_v0.ModuleApiRoute{}, &api_v0.ModuleApi{}, &api_v0.ModuleObject{}},
		objectType: api_v0.ObjectTypeModuleApiRoute,
		object:     new(api_v0.ModuleApiRoute),
	},
}

// TestHandlersAnswerUniqueViolationWith409 covers a 409 that omits the index name.
func TestHandlersAnswerUniqueViolationWith409(t *testing.T) {
	for _, test := range uniqueViolationCases {
		t.Run(test.name, func(t *testing.T) {
			registerValidateTags(test.objectType, test.object)

			c, rec := newUniqueViolationRequest(test.route, test.body)
			h := newRejectingHandler(t, test.models)

			require.NoError(t, test.handler(h, c))

			assert.Equal(t, http.StatusConflict, rec.Code, "handler defined in %s", test.source)

			assert.NotContains(t, rec.Body.String(), rejectingConstraint,
				"the response does not name the index that rejected the write")
		})
	}
}

// newRejectingHandler returns a handler whose creates fail with SQLSTATE 23505.
func newRejectingHandler(t *testing.T, models []any) Handler {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(models...))

	require.NoError(t, db.Callback().Create().After("gorm:create").Register(
		"test:reject_with_unique_violation",
		func(tx *gorm.DB) {
			tx.Statement.DB.Error = &pgconn.PgError{
				Code:           "23505",
				Severity:       "ERROR",
				Message:        `duplicate key value violates unique constraint "` + rejectingConstraint + `"`,
				ConstraintName: rejectingConstraint,
			}
		},
	))

	return Handler{DB: db, Logger: zap.NewNop()}
}

// registerValidateTags registers obj's validate tags under objectType.
func registerValidateTags(objectType string, obj any) {
	taggedFields := map[string]*apiserver_lib.FieldsByTag{
		string(api_lib.ValidateTag): {
			TagName:              string(api_lib.ValidateTag),
			Required:             []string{},
			Optional:             []string{},
			OptionalAssociations: []string{},
		},
	}
	apiserver_lib.ParseStruct(
		string(api_lib.ValidateTag),
		reflect.ValueOf(obj),
		"",
		apiserver_lib.Translate,
		taggedFields,
	)
	apiserver_lib.ObjectTaggedFields[apiserver_lib.VersionObject{
		Version: "v0",
		Object:  objectType,
	}] = taggedFields[string(api_lib.ValidateTag)]
}

// newUniqueViolationRequest returns a POST context wired like the API server.
func newUniqueViolationRequest(route, body string) (*apiserver_lib.CustomContext, *httptest.ResponseRecorder) {
	e := echo.New()
	e.Binder = apiserver_lib.NewQueryBinder()

	validate := validator.New()
	validate.RegisterValidation("optional", apiserver_lib.IsOptional)
	validate.RegisterValidation("association", apiserver_lib.IsAssociation)
	validate.RegisterValidation("ISO8601date", apiserver_lib.IsISO8601Date)
	e.Validator = &apiserver_lib.CustomValidator{Validator: validate}

	req := httptest.NewRequest(http.MethodPost, route, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)

	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath(route)

	return &apiserver_lib.CustomContext{Context: c}, rec
}
