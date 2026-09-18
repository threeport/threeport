package apiserver

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHandlerTemplate_BindErrUsesFullyQualifiedType covers bind-error
// responses using the qualified type string.
func TestHandlerTemplate_BindErrUsesFullyQualifiedType(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)

	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "handlers.go"))
	require.NoError(t, err)

	text := string(src)
	bindErrQualified := regexp.MustCompile(`"ResponseStatusBindErr"[\s\S]{0,240}Id\("fullyQualifiedType"\)`)
	bindErrShort := regexp.MustCompile(`"ResponseStatusBindErr"[\s\S]{0,240}Id\("objectType"\)`)
	assert.Len(t, bindErrQualified.FindAllString(text, -1), 3,
		"create, update, and replace each emit one bind-error path")
	assert.Empty(t, bindErrShort.FindAllString(text, -1))
}
