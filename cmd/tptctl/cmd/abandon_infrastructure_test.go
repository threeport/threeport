package cmd

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
	config_v0 "github.com/threeport/threeport/pkg/config/v0"
)

// TestWithdrawAbandonInfrastructure covers when a failed delete switches the
// override back off. The override stays set until changed, so a delete that
// never got going must clear it. After a timeout the controller may still need
// it, so that stays.
func TestWithdrawAbandonInfrastructure(t *testing.T) {
	cases := []struct {
		name          string
		deleteErr     error
		withdrawErr   error
		wantWithdrawn bool
	}{
		{
			name:          "request rejected",
			deleteErr:     fmt.Errorf("failed to delete: %w", client_lib.ErrDeleteBlocked),
			wantWithdrawn: true,
		},
		{
			name:          "network error",
			deleteErr:     errors.New("connection refused"),
			wantWithdrawn: true,
		},
		{
			name:          "timeout waiting on the controller keeps it",
			deleteErr:     fmt.Errorf("%w: still present", config_v0.ErrDeleteTimedOut),
			wantWithdrawn: false,
		},
		{
			name:          "a failed withdrawal is only reported",
			deleteErr:     errors.New("connection refused"),
			withdrawErr:   errors.New("injected: patch failed"),
			wantWithdrawn: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withdrawn := false

			withdrawAbandonInfrastructure(tc.deleteErr, "thing", "dev-1", func() error {
				withdrawn = true
				return tc.withdrawErr
			})

			assert.Equal(t, tc.wantWithdrawn, withdrawn)
		})
	}
}
