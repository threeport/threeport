package handlers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	api_lib "github.com/threeport/threeport/pkg/api/lib/v0"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
)

// clientErrorCase is one malformed request aimed at one handler, with the status
// that handler owes the client. A request the caller can correct answers 4xx.
// Answering 500 instead tells a controller the server is at fault, so it backs
// off and retries a request that can never succeed.
type clientErrorCase struct {
	name string

	// source is the file defining the handler. Generated handlers all come from
	// one template and their output is identical once the type name is
	// substituted, so one object type stands for every object. The hand-written
	// handlers are not copies of anything and each carries its own rows.
	source string

	// handler is a closure rather than a method value because
	// CustomAddSecretDefinition is echo middleware and has a different shape
	// from the plain handlers.
	handler func(Handler, echo.Context) error

	method string

	// route is the echo route pattern. PayloadCheck recovers the api version
	// from it, so a write case that leaves it empty panics before reaching the
	// bind under test.
	route string

	target string
	body   string

	wantStatus int

	// wantBody, when set, has to appear in the response so the client can tell
	// which input was rejected.
	wantBody string
}

// kubernetesWorkloadDefinitionRoute is the one generated object type this file
// covers. Every other generated object reaches the same helpers through the
// same emitted code.
const kubernetesWorkloadDefinitionRoute = "/v0/kubernetes-workload-definitions"

// clientErrorCases holds every malformed request this package answers without
// touching the database, because each is decided before the handler queries.
// Write cases are limited to Add handlers: Update and Replace load the existing
// row before binding, so covering their bind path needs a live database and
// belongs in test/integration/.
var clientErrorCases = []clientErrorCase{
	{
		name:       "generated list handler rejects an unknown query param",
		source:     "kubernetes_workload_gen.go",
		handler:    func(h Handler, c echo.Context) error { return h.GetKubernetesWorkloadDefinitions(c) },
		method:     http.MethodGet,
		route:      kubernetesWorkloadDefinitionRoute,
		target:     kubernetesWorkloadDefinitionRoute + "?nmae=my-app",
		wantStatus: http.StatusBadRequest,
		wantBody:   "nmae",
	},
	{
		name:       "generated list handler rejects a zero limit",
		source:     "kubernetes_workload_gen.go",
		handler:    func(h Handler, c echo.Context) error { return h.GetKubernetesWorkloadDefinitions(c) },
		method:     http.MethodGet,
		route:      kubernetesWorkloadDefinitionRoute,
		target:     kubernetesWorkloadDefinitionRoute + "?limit=0",
		wantStatus: http.StatusBadRequest,
	},
	{
		name:       "generated list handler rejects a negative limit",
		source:     "kubernetes_workload_gen.go",
		handler:    func(h Handler, c echo.Context) error { return h.GetKubernetesWorkloadDefinitions(c) },
		method:     http.MethodGet,
		route:      kubernetesWorkloadDefinitionRoute,
		target:     kubernetesWorkloadDefinitionRoute + "?limit=-5",
		wantStatus: http.StatusBadRequest,
	},
	{
		name:       "generated list handler rejects a limit over the maximum",
		source:     "kubernetes_workload_gen.go",
		handler:    func(h Handler, c echo.Context) error { return h.GetKubernetesWorkloadDefinitions(c) },
		method:     http.MethodGet,
		route:      kubernetesWorkloadDefinitionRoute,
		target:     kubernetesWorkloadDefinitionRoute + "?limit=100000",
		wantStatus: http.StatusBadRequest,
	},
	{
		name:       "generated list handler rejects an unparseable limit",
		source:     "kubernetes_workload_gen.go",
		handler:    func(h Handler, c echo.Context) error { return h.GetKubernetesWorkloadDefinitions(c) },
		method:     http.MethodGet,
		route:      kubernetesWorkloadDefinitionRoute,
		target:     kubernetesWorkloadDefinitionRoute + "?limit=lots",
		wantStatus: http.StatusBadRequest,
	},
	{
		name:       "generated list handler rejects an unparseable cursor",
		source:     "kubernetes_workload_gen.go",
		handler:    func(h Handler, c echo.Context) error { return h.GetKubernetesWorkloadDefinitions(c) },
		method:     http.MethodGet,
		route:      kubernetesWorkloadDefinitionRoute,
		target:     kubernetesWorkloadDefinitionRoute + "?cursor=first",
		wantStatus: http.StatusBadRequest,
	},
	{
		name:       "generated add handler rejects a wrong field type",
		source:     "kubernetes_workload_gen.go",
		handler:    func(h Handler, c echo.Context) error { return h.AddKubernetesWorkloadDefinition(c) },
		method:     http.MethodPost,
		route:      kubernetesWorkloadDefinitionRoute,
		target:     kubernetesWorkloadDefinitionRoute,
		body:       `{"Name":12345}`,
		wantStatus: http.StatusBadRequest,
		wantBody:   "Name",
	},
	{
		name:       "generated add handler rejects a truncated body",
		source:     "kubernetes_workload_gen.go",
		handler:    func(h Handler, c echo.Context) error { return h.AddKubernetesWorkloadDefinition(c) },
		method:     http.MethodPost,
		route:      kubernetesWorkloadDefinitionRoute,
		target:     kubernetesWorkloadDefinitionRoute,
		body:       `{"Name":`,
		wantStatus: http.StatusBadRequest,
	},
	{
		name:   "events join handler rejects an unknown query param",
		source: "events.go",
		handler: func(h Handler, c echo.Context) error {
			return h.GetEventsJoinAttachedObjectReferences(c)
		},
		method:     http.MethodGet,
		route:      "/v0/events",
		target:     "/v0/events?nmae=my-app",
		wantStatus: http.StatusBadRequest,
		wantBody:   "nmae",
	},
	{
		name:   "events join handler rejects a zero limit",
		source: "events.go",
		handler: func(h Handler, c echo.Context) error {
			return h.GetEventsJoinAttachedObjectReferences(c)
		},
		method:     http.MethodGet,
		route:      "/v0/events",
		target:     "/v0/events?limit=0",
		wantStatus: http.StatusBadRequest,
	},
	{
		name:   "module object list handler rejects an unknown query param",
		source: "module.go",
		handler: func(h Handler, c echo.Context) error {
			return h.GetModuleObjectsWithModuleApiRoutes(c)
		},
		method:     http.MethodGet,
		route:      "/v0/module-objects",
		target:     "/v0/module-objects?nmae=my-app",
		wantStatus: http.StatusBadRequest,
		wantBody:   "nmae",
	},
	{
		name:   "module object list handler rejects a zero limit",
		source: "module.go",
		handler: func(h Handler, c echo.Context) error {
			return h.GetModuleObjectsWithModuleApiRoutes(c)
		},
		method:     http.MethodGet,
		route:      "/v0/module-objects",
		target:     "/v0/module-objects?limit=0",
		wantStatus: http.StatusBadRequest,
	},
	{
		name:       "secret definition middleware rejects a wrong field type",
		source:     "secret.go",
		handler:    addSecretDefinition,
		method:     http.MethodPost,
		route:      "/v0/secret-definitions",
		target:     "/v0/secret-definitions",
		body:       `{"Name":12345}`,
		wantStatus: http.StatusBadRequest,
		wantBody:   "Name",
	},
	{
		name:       "secret definition middleware rejects a truncated body",
		source:     "secret.go",
		handler:    addSecretDefinition,
		method:     http.MethodPost,
		route:      "/v0/secret-definitions",
		target:     "/v0/secret-definitions",
		body:       `{"Name":`,
		wantStatus: http.StatusBadRequest,
	},
	{
		name:   "resource definition set handler rejects a wrong field type",
		source: "kubernetes_workload.go",
		handler: func(h Handler, c echo.Context) error {
			return h.AddKubernetesWorkloadResourceDefinitions(c)
		},
		method:     http.MethodPost,
		route:      "/v0/kubernetes-workload-resource-definition-sets",
		target:     "/v0/kubernetes-workload-resource-definition-sets",
		body:       `[{"KubernetesWorkloadDefinitionID":"not-a-number"}]`,
		wantStatus: http.StatusBadRequest,
		wantBody:   "KubernetesWorkloadDefinitionID",
	},
	{
		name:   "resource definition set handler rejects a truncated body",
		source: "kubernetes_workload.go",
		handler: func(h Handler, c echo.Context) error {
			return h.AddKubernetesWorkloadResourceDefinitions(c)
		},
		method:     http.MethodPost,
		route:      "/v0/kubernetes-workload-resource-definition-sets",
		target:     "/v0/kubernetes-workload-resource-definition-sets",
		body:       `[{"KubernetesWorkloadDefinitionID":`,
		wantStatus: http.StatusBadRequest,
	},
}

// addSecretDefinition adapts the secret definition middleware to the shape the
// case table holds. Every case aimed at it is rejected before the wrapped
// handler runs, so that handler writes nothing and a 200 in the recorder
// reports that the rejection did not happen.
func addSecretDefinition(h Handler, c echo.Context) error {
	return h.CustomAddSecretDefinition(func(echo.Context) error { return nil })(c)
}

// registerTaggedFields populates the tagged-field map PayloadCheck reads, for
// one object type. The versions package does this for every object at server
// start, and this test cannot call it: versions reaches handlers through
// pkg/api-server/v0 and pkg/api-server/v0/routes, so importing it from an
// in-package test is an import cycle.
func registerTaggedFields(objectType string, obj interface{}) {
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

// newHandlerRequest drives a handler the way the router does: the strict query
// binder registered on the echo instance, the route pattern set so PayloadCheck
// can read the api version, and the context wrapped so a handler's assertion to
// CustomContext succeeds.
func newHandlerRequest(test clientErrorCase) (*apiserver_lib.CustomContext, *httptest.ResponseRecorder) {
	e := echo.New()
	e.Binder = apiserver_lib.NewQueryBinder()

	var body io.Reader
	if test.body != "" {
		body = strings.NewReader(test.body)
	}
	req := httptest.NewRequest(test.method, test.target, body)
	if test.body != "" {
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	}

	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath(test.route)

	return &apiserver_lib.CustomContext{Context: c}, rec
}

// TestHandlersAnswerClientErrorsWithClientStatus asserts the layer that sets the
// status rather than the binder underneath it. The binder tests in
// pkg/api-server/lib/v0 prove the error is produced; these prove each handler
// turns it into a status the caller can act on.
func TestHandlersAnswerClientErrorsWithClientStatus(t *testing.T) {
	// PayloadCheck looks an object's tagged fields up by api version and object
	// type, and answers 500 when that lookup misses, which would mask every
	// write case below
	registerTaggedFields(api_v0.ObjectTypeKubernetesWorkloadDefinition, new(api_v0.KubernetesWorkloadDefinition))
	registerTaggedFields(api_v0.ObjectTypeKubernetesWorkloadResourceDefinition, new(api_v0.KubernetesWorkloadResourceDefinition))
	registerTaggedFields(api_v0.ObjectTypeSecretDefinition, new(api_v0.SecretDefinition))

	for _, test := range clientErrorCases {
		t.Run(test.name, func(t *testing.T) {
			c, rec := newHandlerRequest(test)
			h := Handler{Logger: zap.NewNop()}

			require.NoError(t, test.handler(h, c))

			assert.Equal(t, test.wantStatus, rec.Code, "handler defined in %s", test.source)
			if test.wantBody != "" {
				assert.Contains(t, rec.Body.String(), test.wantBody,
					"the response names the input that was rejected")
			}
		})
	}
}

// TestListHandlerAcceptsReservedQueryParams asserts the keys the api-server
// consumes itself are not mistaken for unknown filters. These reach the handler
// on ordinary requests, so rejecting them would break pagination and the bulk id
// lookup rather than catching a typo.
func TestListHandlerAcceptsReservedQueryParams(t *testing.T) {
	for _, target := range []string{
		kubernetesWorkloadDefinitionRoute + "?includedeleted=true",
		kubernetesWorkloadDefinitionRoute + "?ids=1,2,3",
		kubernetesWorkloadDefinitionRoute + "?limit=10",
		kubernetesWorkloadDefinitionRoute + "?name=my-app",
	} {
		t.Run(target, func(t *testing.T) {
			c, rec := newHandlerRequest(clientErrorCase{
				method: http.MethodGet,
				route:  kubernetesWorkloadDefinitionRoute,
				target: target,
			})
			h, _ := newDryRunHandler(t, apiserver_lib.PaginationModeAsOfSystemTime)

			// the bind has to succeed and let the handler reach its count
			// query, which the dry-run database then refuses. Any status but
			// 400 proves the key passed the unknown-key gate
			_ = h.GetKubernetesWorkloadDefinitions(c)

			assert.NotEqual(t, http.StatusBadRequest, rec.Code,
				"a reserved key must not read as an unknown filter")
		})
	}
}
