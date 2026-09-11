package cockroach

import (
	"fmt"
	"net/http"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	handlers "github.com/threeport/threeport/pkg/api-server/v0/handlers"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// Under SERIALIZABLE isolation CockroachDB answers a write conflict with
// SQLSTATE 40001. RetryWrite absorbs that; the client should not see 500.

const serializationConflict = "database serialization conflict"

const serializationWriters = 6

const serializationRounds = 6

// TestConcurrentUpdatesAbsorbSerializationFailures covers overlapping
// updates of one row through the generated handler.
func TestConcurrentUpdatesAbsorbSerializationFailures(t *testing.T) {
	// seed one row every writer will fight over
	id := seedDomainNameDefinition(t, "serial-retry-row")

	// run overlapping updates against that row
	codes, bodies := runConcurrentDomainNameUpdates(t, id)

	// none of the writers may see a serialization abort or a non-200
	assertNoSerializationConflict(t, codes, bodies, http.StatusOK)
}

// TestConcurrentCreatesOfTheSameNameDoNotLeakSerializationFailures covers
// overlapping creates of one unique name: one 201, the rest 409, none 500.
func TestConcurrentCreatesOfTheSameNameDoNotLeakSerializationFailures(t *testing.T) {
	const name = "serial-create-same"

	// drop leftover rows from a failed run
	t.Cleanup(func() {
		_ = testDb.Unscoped().Where("name = ?", name).Delete(&api_v0.DomainNameDefinition{}).Error
	})

	// run overlapping creates of the same name
	codes, bodies := runConcurrentDomainNameCreates(t, name)

	// no writer may leak a serialization abort
	for n, body := range bodies {
		assert.NotContains(t, body, serializationConflict,
			"writer %d leaked a serialization abort", n)
	}

	// one create is accepted; the rest collide on the unique name
	created, conflicted := 0, 0
	for n, code := range codes {
		switch code {
		case http.StatusCreated:
			created++
		case http.StatusConflict:
			conflicted++
		default:
			t.Errorf("writer %d: status %d body %s", n, code, bodies[n])
		}
	}
	assert.Equal(t, 1, created, "exactly one create is accepted")
	assert.Equal(t, serializationWriters-1, conflicted, "the rest collide on the unique name")
}

// seedDomainNameDefinition inserts one definition and returns its id.
func seedDomainNameDefinition(t *testing.T, name string) uint {
	t.Helper()

	registerValidateTags(api_v0.ObjectTypeDomainNameDefinition, new(api_v0.DomainNameDefinition))

	row := api_v0.DomainNameDefinition{
		Definition: api_v0.Definition{Name: util.Ptr(name)},
		Domain:     util.Ptr("serial-retry.example"),
		Zone:       util.Ptr("serial-retry"),
		AdminEmail: util.Ptr("serial@example.com"),
	}
	require.NoError(t, testDb.Create(&row).Error, "the seed row is accepted")

	t.Cleanup(func() {
		_ = testDb.Unscoped().Delete(&api_v0.DomainNameDefinition{}, *row.ID).Error
	})

	return *row.ID
}

// runConcurrentDomainNameUpdates patches one row from several goroutines.
func runConcurrentDomainNameUpdates(t *testing.T, id uint) (codes [][]int, bodies [][]string) {
	t.Helper()

	h := handlers.Handler{DB: testDb, Logger: zap.NewNop()}

	// collect status and body per writer per round
	codes = make([][]int, serializationWriters)
	bodies = make([][]string, serializationWriters)
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(serializationWriters)

	for n := 0; n < serializationWriters; n++ {
		codes[n] = make([]int, serializationRounds)
		bodies[n] = make([]string, serializationRounds)

		// each writer loops so attempts overlap long enough to abort
		go func(n int) {
			defer wg.Done()
			<-start
			for round := 0; round < serializationRounds; round++ {
				name := fmt.Sprintf("serial-upd-%d-%d", n, round)
				c, rec := newAPIRequest(
					http.MethodPatch,
					api_v0.PathDomainNameDefinitions+"/:id",
					fmt.Sprint(id),
					fmt.Sprintf(`{"Name":%q}`, name),
				)
				err := h.UpdateDomainNameDefinition(c)
				require.NoError(t, err, "writer %d round %d", n, round)
				codes[n][round] = rec.Code
				bodies[n][round] = rec.Body.String()
			}
		}(n)
	}

	close(start)
	wg.Wait()
	return codes, bodies
}

// runConcurrentDomainNameCreates posts the same unique name from several goroutines.
func runConcurrentDomainNameCreates(t *testing.T, name string) (codes []int, bodies []string) {
	t.Helper()

	registerValidateTags(api_v0.ObjectTypeDomainNameDefinition, new(api_v0.DomainNameDefinition))

	h := handlers.Handler{DB: testDb, Logger: zap.NewNop()}
	codes = make([]int, serializationWriters)
	bodies = make([]string, serializationWriters)
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(serializationWriters)

	for n := 0; n < serializationWriters; n++ {
		go func(n int) {
			defer wg.Done()
			<-start
			body := fmt.Sprintf(
				`{"Name":%q,"Domain":"serial-create.example","Zone":"serial-create","AdminEmail":"serial@example.com"}`,
				name,
			)
			c, rec := newCreateRequest(api_v0.PathDomainNameDefinitions, body)
			err := h.AddDomainNameDefinition(c)
			require.NoError(t, err, "writer %d", n)
			codes[n] = rec.Code
			bodies[n] = rec.Body.String()
		}(n)
	}

	close(start)
	wg.Wait()
	return codes, bodies
}

// assertNoSerializationConflict reports a leaked abort or a status other than want.
func assertNoSerializationConflict(t *testing.T, codes [][]int, bodies [][]string, want int) {
	t.Helper()

	for n := range codes {
		for round, code := range codes[n] {
			assert.NotContains(t, bodies[n][round], serializationConflict,
				"writer %d round %d leaked a serialization abort", n, round)
			assert.Equal(t, want, code,
				"writer %d round %d: %s", n, round, bodies[n][round])
		}
	}
}
