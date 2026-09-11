package handlers

import (
	"fmt"
	"net/http"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	sqlite "gorm.io/driver/sqlite"
	"gorm.io/gorm"

	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	"github.com/threeport/threeport/pkg/encryption/v0"
)

// sqlite cannot produce SQLSTATE 40001, so these tests inject a *pgconn.PgError
// on the first create and let RetryWrite run the write again.

// TestCreateHandlersClearIDOnSerializationRetry covers a retried create that
// does not reuse the primary key a rolled-back attempt was given.
func TestCreateHandlersClearIDOnSerializationRetry(t *testing.T) {
	// each case is one create path that must clear id on retry
	cases := []uniqueViolationCase{
		{
			name:       "generated add handler",
			source:     "kubernetes_workload_gen.go",
			handler:    func(h Handler, c echo.Context) error { return h.AddKubernetesWorkloadDefinition(c) },
			route:      "/v0/kubernetes-workload-definitions",
			body:       `{"Name":"my-app","YAMLDocument":"kind: Namespace","Reconciled":true}`,
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
				return h.CustomAddSecretDefinition(func(echo.Context) error { return nil })(c)
			},
			route:      "/v0/secret-definitions",
			body:       `{"Name":"my-secret","Data":{"key":"value"},"Reconciled":true}`,
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

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			// encrypt hook reads this env var on secret create
			if test.source == "secret.go" {
				t.Setenv(encryption.KeyEnvVar, "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
			}
			registerValidateTags(test.objectType, test.object)

			// first create fails with 40001; later creates succeed
			var creates atomic.Int32
			var idsBefore []bool
			h := newSerializationRetryHandler(t, test.models, &creates, &idsBefore)

			c, rec := newUniqueViolationRequest(test.route, test.body)
			err := test.handler(h, c)
			require.NoError(t, err, "handler defined in %s", test.source)

			// RetryWrite must run create twice and clear id the second time
			assert.GreaterOrEqual(t, int(creates.Load()), 2,
				"RetryWrite re-runs create after SQLSTATE 40001")
			require.GreaterOrEqual(t, len(idsBefore), 2,
				"each create attempt records whether the dest already had an id")
			assert.False(t, idsBefore[0], "the first create starts with no id")
			assert.False(t, idsBefore[1],
				"the retried create clears the id a rolled-back attempt was given")

			// secret and module still need nats or a parent row after persist
			if test.source == "secret.go" || test.source == "module.go" {
				return
			}
			assert.Equal(t, http.StatusCreated, rec.Code, "handler defined in %s", test.source)
		})
	}
}

// newSerializationRetryHandler returns a handler whose first create fails with
// SQLSTATE 40001 and whose later creates succeed.
func newSerializationRetryHandler(
	t *testing.T,
	models []any,
	creates *atomic.Int32,
	idsBefore *[]bool,
) Handler {
	t.Helper()

	// sqlite cannot emit 40001; callbacks inject it
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(models...))

	require.NoError(t, db.Callback().Create().Before("gorm:create").Register(
		"test:record_create_id",
		func(tx *gorm.DB) {
			creates.Add(1)
			*idsBefore = append(*idsBefore, destHasID(tx))
		},
	))
	require.NoError(t, db.Callback().Create().After("gorm:create").Register(
		"test:reject_first_create_with_serialization_failure",
		func(tx *gorm.DB) {
			if creates.Load() != 1 {
				return
			}
			// sqlite already wrote the row; a rolled-back cockroach
			// attempt would not have, so drop it before retrying
			if tx.Statement != nil && tx.Statement.Table != "" {
				_ = tx.Exec("DELETE FROM `" + tx.Statement.Table + "`").Error
			}
			tx.Error = fmt.Errorf("persist object: %w", &pgconn.PgError{
				Code:     "40001",
				Severity: "ERROR",
				Message:  "restart transaction: TransactionRetryWithProtoRefreshError",
			})
		},
	))

	return Handler{DB: db, Logger: zap.NewNop()}
}

// destHasID reports a non-nil ID on the create dest.
func destHasID(tx *gorm.DB) bool {
	if tx.Statement == nil {
		return false
	}
	rv := tx.Statement.ReflectValue
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	if !rv.IsValid() || rv.Kind() != reflect.Struct {
		return false
	}
	field := rv.FieldByName("ID")
	return field.IsValid() && field.Kind() == reflect.Ptr && !field.IsNil()
}
