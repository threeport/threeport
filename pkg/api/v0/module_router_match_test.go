package v0

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestMatchRoute covers how a request path is matched to a module's route:
// the route's segments have to begin the request's, and whatever follows -
// an object ID, a docs asset - goes to the module too.
func TestMatchRoute(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		registered string
		requested  string
		match      bool
	}{
		{"the route itself", "/v0/widgets", "/v0/widgets", true},
		{"an object under it", "/v0/widgets", "/v0/widgets/7", true},
		{"a docs asset under a module's docs route", "/swagger/module.threeport.io/test-module-api", "/swagger/module.threeport.io/test-module-api/index.html", true},
		{"a different route", "/v0/widgets", "/v0/gadgets", false},
		{"a route sharing only a prefix of a segment", "/v0/widgets", "/v0/widgetsextra", false},
		{"the core API's own docs", "/swagger/module.threeport.io/test-module-api", "/swagger/index.html", false},
		// a request shorter than the route used to index past its end and
		// panic, failing every such request to the core API
		{"a request shorter than the route", "/swagger/module.threeport.io/test-module-api", "/swagger", false},
		{"the root", "/v0/widgets", "/", false},
		{"a version alone", "/v0/widgets", "/v0", false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			assert.Equal(t, testCase.match, matchRoute(testCase.registered, testCase.requested))
		})
	}
}
