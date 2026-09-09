package v0

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestErrWithEvent_ErrorReturnsMessage covers Error returning the Message
// field verbatim, not the event note.
func TestErrWithEvent_ErrorReturnsMessage(t *testing.T) {
	// construct an error whose message differs from the event note
	err := &ErrWithEvent{
		Message: "ssh dial failed",
		Event: v0.Event{
			Reason: util.Ptr("SSHConnectFailed"),
			Type:   util.Ptr("Warning"),
			Note:   util.Ptr("dial tcp: refused"),
		},
	}

	// check Error returns Message with no prefix
	require.Equal(t, "ssh dial failed", err.Error())
}

// TestErrWithEvent_UnwrapsThroughErrorsAs covers errors.As finding the
// typed error through two wrapping layers.
func TestErrWithEvent_UnwrapsThroughErrorsAs(t *testing.T) {
	// wrap the typed error twice with fmt.Errorf
	inner := &ErrWithEvent{
		Message: "boom",
		Event: v0.Event{
			Reason: util.Ptr("CreateResourceError"),
			Type:   util.Ptr("Warning"),
		},
	}
	outer := fmt.Errorf("wrap two: %w", fmt.Errorf("wrap one: %w", inner))

	// check errors.As recovers the carried event
	var got *ErrWithEvent
	require.True(t, errors.As(outer, &got), "errors.As must unwrap ErrWithEvent through fmt.Errorf %%w layers")
	require.NotNil(t, got)
	require.NotNil(t, got.Event.Reason)
	assert.Equal(t, "CreateResourceError", *got.Event.Reason, "unwrap surfaces the sentinel's carried event")
}
