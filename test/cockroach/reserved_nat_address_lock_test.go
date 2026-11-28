package cockroach

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/threeport/threeport/pkg/api-server/v0/handlers"
	"github.com/threeport/threeport/pkg/api-server/v0/versions"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestAConcurrentPatchWaitsForTheReservedNatAddressItIsCheckedAgainst covers
// #791 against a real CockroachDB: a PATCH of a GKE instance's
// ReservedNatAddress, loaded before another transaction locked the row,
// waits for that transaction to end before it is checked, and is then
// checked against what it left rather than against the row as its handler
// loaded it.
//
// Nothing that goes through the hooks can change the field, so the other
// transaction sets it with a write that skips them, as a migration or an
// operator fixing the row would.  It locks the row with SELECT ... FOR
// UPDATE first and writes only once the PATCH waits for it.
//
// A check that didn't lock the row would read it as its handler loaded it,
// without the field, and pass the PATCH, whose write would then wait for the
// other transaction and fail with CockroachDB's retry error rather than the
// reason the field can't change.  CockroachDB holds back a read that doesn't
// lock behind the lock too, by default, which would hide that, so the test
// turns it off while it runs, as an operator may.
func TestAConcurrentPatchWaitsForTheReservedNatAddressItIsCheckedAgainst(t *testing.T) {
	db := runtimeDatabase(t, "reserved_nat_address_lock")
	const blockingReads = "kv.lock.exclusive_locks_block_non_locking_reads.enabled"
	var blocking string
	require.NoError(t, db.Raw("SHOW CLUSTER SETTING "+blockingReads).Scan(&blocking).Error)
	require.NoError(t, db.Exec("SET CLUSTER SETTING "+blockingReads+" = false").Error)
	t.Cleanup(func() {
		require.NoError(t, db.Exec("SET CLUSTER SETTING "+blockingReads+" = "+blocking).Error)
	})

	// a GKE instance created without a reserved address, with what it
	// refers to
	raw := db.Session(&gorm.Session{SkipHooks: true})
	gcpProvider := &api_v0.GcpProvider{
		Name:          util.Ptr("nat-lock"),
		ProjectID:     util.Ptr("nat-lock"),
		DefaultRegion: util.Ptr("us-central1"),
	}
	require.NoError(t, raw.Create(gcpProvider).Error)
	definition := &api_v0.KubernetesRuntimeDefinition{
		Definition:    api_v0.Definition{Name: util.Ptr("nat-lock")},
		InfraProvider: util.Ptr(api_v0.KubernetesRuntimeInfraProviderGKE),
	}
	require.NoError(t, raw.Create(definition).Error)
	gkeDefinition := &api_v0.GcpGkeKubernetesRuntimeDefinition{
		Definition:                    api_v0.Definition{Name: util.Ptr("nat-lock")},
		ZoneCount:                     util.Ptr(1),
		DefaultNodeGroupInstanceType:  util.Ptr("e2-standard-2"),
		DefaultNodeGroupInitialSize:   util.Ptr(1),
		DefaultNodeGroupMinimumSize:   util.Ptr(1),
		DefaultNodeGroupMaximumSize:   util.Ptr(3),
		KubernetesRuntimeDefinitionID: definition.ID,
	}
	require.NoError(t, raw.Create(gkeDefinition).Error)
	instance := &api_v0.KubernetesRuntimeInstance{
		Instance:                      api_v0.Instance{Name: util.Ptr("nat-lock")},
		Location:                      util.Ptr("NorthAmerica:Iowa"),
		KubernetesRuntimeDefinitionID: definition.ID,
	}
	require.NoError(t, raw.Create(instance).Error)
	gkeInstance := &api_v0.GcpGkeKubernetesRuntimeInstance{
		Instance:                            api_v0.Instance{Name: util.Ptr("nat-lock")},
		Region:                              util.Ptr("us-central1"),
		GcpProviderID:                       gcpProvider.ID,
		GcpGkeKubernetesRuntimeDefinitionID: gkeDefinition.ID,
		KubernetesRuntimeInstanceID:         instance.ID,
	}
	require.NoError(t, raw.Create(gkeInstance).Error)

	// the PATCH's handler loads the instance before the other transaction
	// begins
	load := func() *api_v0.GcpGkeKubernetesRuntimeInstance {
		var loaded api_v0.GcpGkeKubernetesRuntimeInstance
		require.NoError(t, db.First(&loaded, *gkeInstance.ID).Error)
		return &loaded
	}
	loaded := load()

	// every query the test makes, and every wait, ends by this deadline
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// the other transaction locks the row, and reserves the address once
	// released
	locked := make(chan struct{})
	release := make(chan struct{})
	var releaseOther sync.Once
	defer releaseOther.Do(func() { close(release) })
	otherDone := make(chan error, 1)
	go func() {
		otherDone <- raw.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var row api_v0.GcpGkeKubernetesRuntimeInstance
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, *gkeInstance.ID).Error; err != nil {
				return err
			}
			close(locked)
			<-release
			return tx.Model(&api_v0.GcpGkeKubernetesRuntimeInstance{}).
				Where("id = ?", *gkeInstance.ID).
				Update("reserved_nat_address", true).Error
		})
	}()
	select {
	case <-locked:
	case err := <-otherDone:
		t.Fatalf("the other transaction ended before it locked the row: %v", err)
	case <-ctx.Done():
		t.Fatal("the other transaction did not lock the row")
	}

	// the PATCH keeps the field off, as its handler loaded it
	patchDone := make(chan error, 1)
	go func() {
		patchDone <- db.WithContext(ctx).Model(loaded).
			Updates(&api_v0.GcpGkeKubernetesRuntimeInstance{ReservedNatAddress: util.Ptr(false)}).Error
	}()

	// the PATCH waits in CockroachDB for the lock on the row, rather than
	// having merely not started yet
	statement := &gorm.Statement{DB: db}
	require.NoError(t, statement.Parse(&api_v0.GcpGkeKubernetesRuntimeInstance{}))
	for waiting := false; !waiting; {
		select {
		case err := <-patchDone:
			t.Fatalf("the PATCH ended while the other transaction held the row: %v", err)
		case <-ctx.Done():
			t.Fatal("the PATCH never waited for the row the other transaction held")
		case <-time.After(20 * time.Millisecond):
		}
		var waiters int64
		err := db.WithContext(ctx).Raw(
			"SELECT count(*) FROM crdb_internal.cluster_locks WHERE database_name = current_database() AND table_name = ? AND NOT granted",
			statement.Schema.Table,
		).Scan(&waiters).Error
		if ctx.Err() != nil {
			t.Fatal("the PATCH never waited for the row the other transaction held")
		}
		require.NoError(t, err)
		waiting = waiters > 0
	}

	// once the other transaction commits, the PATCH is checked against what
	// it wrote
	releaseOther.Do(func() { close(release) })
	select {
	case err := <-otherDone:
		require.NoError(t, err, "the other transaction")
	case <-ctx.Done():
		t.Fatal("the other transaction did not end once released")
	}
	select {
	case err := <-patchDone:
		require.Error(t, err, "the PATCH is checked against the address the other transaction reserved")
		assert.Contains(t, err.Error(), api_v0.ErrMsgReservedNatAddressImmutable)
	case <-ctx.Done():
		t.Fatal("the PATCH did not end after the other transaction committed")
	}

	stored := load()
	require.NotNil(t, stored.ReservedNatAddress)
	assert.True(t, *stored.ReservedNatAddress, "the PATCH wrote nothing over the reserved address")

	// the API's own PATCH handler refuses the change with a 400 and the
	// reason, against the row as it now is
	versions.AddGcpGkeKubernetesRuntimeInstanceVersions()
	handler := handlers.Handler{DB: db, Logger: zap.NewNop()}
	c, recorder := newAPIRequest(
		http.MethodPatch,
		api_v0.PathGcpGkeKubernetesRuntimeInstances+"/:id",
		fmt.Sprint(*gkeInstance.ID),
		`{"ReservedNatAddress": false}`,
	)
	require.NoError(t, handler.UpdateGcpGkeKubernetesRuntimeInstance(c))
	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Contains(t, recorder.Body.String(), api_v0.ErrMsgReservedNatAddressImmutable)
	stored = load()
	require.NotNil(t, stored.ReservedNatAddress)
	assert.True(t, *stored.ReservedNatAddress)
}
