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

// A unique index rejects a write with SQLSTATE 23505, which the pgx driver
// surfaces as a *pgconn.PgError.  The handlers answer it with a 409, which the
// client maps to a conflict error callers match on; a 500 arrives unclassified.
//
// apiserver_lib.UniqueViolation() matches only that error type, so the sqlite
// database these tests run on cannot produce the rejection and a gorm create
// callback injects it instead.  test/cockroach drives generated add handlers
// against a real CockroachDB, where the database produces the rejection.

// rejectingConstraint is the index name the injected rejection carries.  It is
// a string no handler produces for another reason, so the assertion cannot fail
// on a response that leaked nothing.
const rejectingConstraint = "idx_test_unique_violation"

// uniqueViolationCase is one handler driven with a rejected write, plus the
// setup that handler needs before it can be called.
type uniqueViolationCase struct {
	// The subtest name
	name string

	// The file the handler is defined in, named in the failure message
	source string

	// A call of the handler under test.  A closure rather than a method value,
	// since one of the four is echo middleware and takes a next handler
	handler func(Handler, echo.Context) error

	// The request path, whose first segment is the API version PayloadCheck
	// reads off the context
	route string

	// The JSON request body, carrying every field the handler requires
	body string

	// The models to migrate into the database before the write
	models []any

	// The object type the validate tags are registered under
	objectType string

	// An empty object of the handler's type, parsed for its validate tags
	object any
}

// uniqueViolationCases holds one case per shape of add handler: one for every
// generated add handler, since they all answer a failed create through the same
// call, and one each for the hand written handlers and the middleware.
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

// TestHandlersAnswerUniqueViolationWith409 asserts each add handler shape
// answers a rejected write with a conflict that does not name the index.
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

// newRejectingHandler returns a handler over an in-memory database whose
// creates all fail with a unique violation.
func newRejectingHandler(t *testing.T, models []any) Handler {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(models...))

	// fail every create with the rejection a unique index produces
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

// registerValidateTags registers obj's validate tags under objectType, which
// PayloadCheck needs. The versions package does this at server start; it cannot
// run here, since it reaches this package via pkg/api-server/v0.
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

// newUniqueViolationRequest builds a POST context and its recorder with the
// binder, validator, and context wrapper the rest api server installs.
func newUniqueViolationRequest(route, body string) (*apiserver_lib.CustomContext, *httptest.ResponseRecorder) {
	e := echo.New()
	e.Binder = apiserver_lib.NewQueryBinder()

	// register the validator and every tag the api types carry; validation
	// panics without a validator, and again on a tag it has no function for
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
