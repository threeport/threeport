package cockroach

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// apiAccessCheckedKey marks a PATCH's context with the hold it reports to
// once its hooks have checked it.
type apiAccessCheckedKey struct{}

// apiAccessHold is where a PATCH stops after its hooks, before it writes.
type apiAccessHold struct {
	// checked is closed when the PATCH's hooks have passed
	checked chan struct{}
	// release, when set, holds the PATCH until it is closed
	release chan struct{}
	// whileHeld runs in the PATCH's transaction once it is released, under a
	// context without the hold
	whileHeld func(tx *gorm.DB) error
}

// holdAfterApiAccessCheck registers a callback on db that stops a PATCH
// whose context carries an apiAccessHold between its hooks and its write:
// the row its hooks read is locked by then, and nothing is written to it yet.
func holdAfterApiAccessCheck(t *testing.T, db *gorm.DB) {
	t.Helper()

	require.NoError(t, db.Callback().Update().
		After("gorm:before_update").Before("gorm:update").
		Register("test:hold_after_api_access_check", func(tx *gorm.DB) {
			hold, ok := tx.Statement.Context.Value(apiAccessCheckedKey{}).(*apiAccessHold)
			if !ok || tx.Error != nil {
				return
			}
			close(hold.checked)
			if hold.release == nil {
				return
			}
			<-hold.release
			if hold.whileHeld != nil {
				if err := hold.whileHeld(tx.Session(&gorm.Session{NewDB: true, SkipHooks: true, Context: context.Background()})); err != nil {
					_ = tx.AddError(err)
				}
			}
		}))
}

// apiAccessCidrs returns n distinct CIDR blocks as AllowedCidrs holds them.
func apiAccessCidrs(t *testing.T, n int) *datatypes.JSON {
	t.Helper()

	cidrs := make([]string, n)
	for i := range cidrs {
		cidrs[i] = fmt.Sprintf("10.%d.%d.0/24", i/256, i%256)
	}
	raw, err := json.Marshal(cidrs)
	require.NoError(t, err)
	value := datatypes.JSON(raw)

	return &value
}

// TestAConcurrentPatchWaitsForTheRowAPatchIsCheckedAgainst covers two
// PATCHes of the Kubernetes API access of one GKE definition against a real
// CockroachDB.  The first has been checked against the row and not written
// yet; the second waits for the first's transaction to end before it is
// checked, rather than being checked against the row as it was before the
// first.  Once the first's transaction has turned PrivateApi off, the second
// is checked against that: its 60 CIDR blocks fit a private cluster but not a
// public one, and it is refused for the public limit.
//
// Nothing that goes through the hooks can change the pair, which is
// immutable, so the first transaction turns PrivateApi off with a write that
// skips them, as a migration or an operator fixing the row would.  A check
// that read the row without locking it would pass the second PATCH while the
// first was still open, and this test would fail at that wait.
func TestAConcurrentPatchWaitsForTheRowAPatchIsCheckedAgainst(t *testing.T) {
	db := freshDatabase(t, "api_access_lock_check")
	require.NoError(t, db.AutoMigrate(&api_v0.AttachedObjectReference{}, &api_v0.KubernetesRuntimeDefinition{}))
	holdAfterApiAccessCheck(t, db)

	definition := &api_v0.KubernetesRuntimeDefinition{
		Definition:       api_v0.Definition{Name: util.Ptr("private-gke")},
		InfraProvider:    util.Ptr(api_v0.KubernetesRuntimeInfraProviderGKE),
		HighAvailability: util.Ptr(false),
		PrivateApi:       util.Ptr(true),
		AllowedCidrs:     apiAccessCidrs(t, 40),
	}
	require.NoError(t, db.Create(definition).Error)

	// both PATCHes' handlers load the definition before either begins
	load := func() *api_v0.KubernetesRuntimeDefinition {
		var loaded api_v0.KubernetesRuntimeDefinition
		require.NoError(t, db.First(&loaded, *definition.ID).Error)
		return &loaded
	}
	firstLoaded, secondLoaded := load(), load()

	// the first PATCH sends back the list it loaded, is checked, and holds
	// the row until released, then turns PrivateApi off in its transaction
	first := &apiAccessHold{
		checked: make(chan struct{}),
		release: make(chan struct{}),
		whileHeld: func(tx *gorm.DB) error {
			return tx.Model(&api_v0.KubernetesRuntimeDefinition{}).
				Where("id = ?", *definition.ID).
				Update("private_api", false).Error
		},
	}
	firstDone := make(chan error, 1)
	go func() {
		ctx := context.WithValue(context.Background(), apiAccessCheckedKey{}, first)
		firstDone <- db.WithContext(ctx).Model(firstLoaded).
			Updates(&api_v0.KubernetesRuntimeDefinition{AllowedCidrs: apiAccessCidrs(t, 40)}).Error
	}()
	select {
	case <-first.checked:
	case err := <-firstDone:
		t.Fatalf("the first PATCH ended before it was checked: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("the first PATCH was not checked")
	}

	// the second PATCH sends 60 blocks, which a private cluster admits
	second := &apiAccessHold{checked: make(chan struct{})}
	secondDone := make(chan error, 1)
	go func() {
		ctx := context.WithValue(context.Background(), apiAccessCheckedKey{}, second)
		secondDone <- db.WithContext(ctx).Model(secondLoaded).
			Updates(&api_v0.KubernetesRuntimeDefinition{AllowedCidrs: apiAccessCidrs(t, 60)}).Error
	}()
	select {
	case <-second.checked:
		close(first.release)
		t.Fatal("the second PATCH was checked while the first held the row")
	case err := <-secondDone:
		close(first.release)
		t.Fatalf("the second PATCH ended while the first held the row: %v", err)
	case <-time.After(time.Second):
	}

	// once the first commits, the second is checked against what it wrote
	close(first.release)
	require.NoError(t, <-firstDone, "the first PATCH")
	select {
	case err := <-secondDone:
		require.Error(t, err, "the second PATCH is checked against the public cluster the first left")
		assert.Contains(t, err.Error(), "AllowedCidrs has 60 entries and gke admits at most 50")
	case <-time.After(10 * time.Second):
		t.Fatal("the second PATCH did not end after the first committed")
	}

	stored := load()
	assert.False(t, *stored.PrivateApi, "the first PATCH's transaction turned PrivateApi off")
	got, err := api_v0.ParseCidrs("AllowedCidrs", stored.AllowedCidrs)
	require.NoError(t, err)
	assert.Len(t, got, 40, "the second PATCH wrote nothing")
}
