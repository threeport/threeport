package apiserver

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestHandlerTemplate_ByIDHandlersParseTheID covers the get, update, replace
// and delete handlers of every API built with the SDK, modules' included. Each
// one parses its path ID before it queries: GORM reads a string it is given as
// the condition of First as SQL when the string is not a number. The handler
// tests in pkg/api-server/v0/handlers run the generated output.
func TestHandlerTemplate_ByIDHandlersParseTheID(t *testing.T) {
	text := handlerTemplateSource(t)

	assert.Equal(t, 4, strings.Count(text, "parseObjectID(apiObject.TypeName)"),
		"get, update, replace and delete each read the ID through parseObjectID")
	assert.NotContains(t, text, `Dot("Param")`,
		"no handler reads a path parameter it has not parsed")
	assert.Contains(t, text, `"ParseObjectID",`)
}
