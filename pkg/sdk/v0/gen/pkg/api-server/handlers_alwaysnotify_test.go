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

// TestHandlerTemplate_AlwaysNotifyOnUpdate covers the two code paths the
// generator emits for the update/replace notification condition: the default
// path (Reconciled gate present) used for every object that does not set
// AlwaysNotifyOnUpdate, and the opt-in path (Reconciled gate omitted) used
// when it's set. Both paths must still apply ReconciliationUpdateNotifiable,
// so enabling the option doesn't turn every trivial write into a redundant
// notification.
func TestHandlerTemplate_AlwaysNotifyOnUpdate(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)

	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "handlers.go"))
	require.NoError(t, err)

	text := string(src)

	branchCondition := regexp.MustCompile(`if apiObject\.AlwaysNotifyOnUpdate \{`)
	assert.Len(t, branchCondition.FindAllString(text, -1), 1,
		"generator branches on apiObject.AlwaysNotifyOnUpdate exactly once")

	reconciledGate := regexp.MustCompile(`Id\(fmt\.Sprintf\("existing%s", apiObject\.TypeName\)\)\.Dot\("Reconciled"\)`)
	assert.Len(t, reconciledGate.FindAllString(text, -1), 2,
		"the Reconciled gate (referenced twice: != nil and !*Reconciled) is still constructed for the default (disabled) branch")

	notifiableCall := regexp.MustCompile(`"ReconciliationUpdateNotifiable"`)
	assert.Len(t, notifiableCall.FindAllString(text, -1), 1,
		"both branches converge on a single ReconciliationUpdateNotifiable call, so the spam guard always applies")
}
