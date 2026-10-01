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

// handlerTemplateSource returns the generator this package emits handlers from.
func handlerTemplateSource(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)

	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "handlers.go"))
	require.NoError(t, err)

	return string(src)
}

// TestHandlerTemplate_UpdateNotificationDoesNotGateOnReconciled covers the
// condition the generated update and replace handlers publish a notification
// under.
//
// Gating it on the object's own Reconciled field swallowed the notification for
// anything already marked reconciled, and nothing sets that back to false when a
// spec field is edited - so there was no way to make a controller revisit an
// object marked reconciled that was not. The condition is notifiability alone.
func TestHandlerTemplate_UpdateNotificationDoesNotGateOnReconciled(t *testing.T) {
	text := handlerTemplateSource(t)

	notifiable := regexp.MustCompile(`notifyControllersUpdateHandler\.If\(Qual\(`)
	assert.Len(t, notifiable.FindAllString(text, -1), 1,
		"the update notification condition is a single call to ReconciliationUpdateNotifiable")

	gated := regexp.MustCompile(`notifyControllersUpdateHandler\.If\([\s\S]{0,200}Dot\("Reconciled"\)`)
	assert.Empty(t, gated.FindAllString(text, -1),
		"the update notification must not be gated on the object's Reconciled field")
}

// the check that is meant to be there still is: a write that changes nothing
// about reconciliation should not wake a controller
func TestHandlerTemplate_UpdateNotificationKeepsNotifiableCheck(t *testing.T) {
	text := handlerTemplateSource(t)

	assert.Contains(t, text, `"ReconciliationUpdateNotifiable"`)
}
