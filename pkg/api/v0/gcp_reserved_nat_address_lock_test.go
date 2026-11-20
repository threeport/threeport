package v0

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	util "github.com/threeport/threeport/pkg/util/v0"
)

// reservedNatAddressLocks returns how many times a PATCH of a GKE runtime
// instance with the given values, an object or a map, locked its row to
// check it.
func reservedNatAddressLocks(t *testing.T, patch any) int {
	t.Helper()
	db := dryRunDB(t)
	var queries []string
	require.NoError(t, db.Callback().Query().After("gorm:query").Register(
		"test:record_query",
		func(tx *gorm.DB) { queries = append(queries, tx.Statement.SQL.String()) },
	))

	loaded := &GcpGkeKubernetesRuntimeInstance{ReservedNatAddress: util.Ptr(true)}
	loaded.ID = util.Ptr(uint(7))
	_ = db.Model(loaded).Updates(patch)

	locked := 0
	for _, query := range queries {
		if strings.HasPrefix(query, `SELECT * FROM "v0_gcp_gke_kubernetes_runtime_instances" WHERE`) &&
			strings.HasSuffix(query, "FOR UPDATE") {
			locked++
		}
	}
	return locked
}

// TestAReservedNatAddressPatchLocksTheRowItIsCheckedAgainst covers #791: a
// PATCH that carries ReservedNatAddress reads the row it writes over with
// SELECT ... FOR UPDATE, as the PrivateApi and AllowedCidrs check does, so a
// concurrent update of the row waits for it.  A PATCH that doesn't carry it
// reads nothing.
func TestAReservedNatAddressPatchLocksTheRowItIsCheckedAgainst(t *testing.T) {
	assert.Equal(t, 1, reservedNatAddressLocks(t, &GcpGkeKubernetesRuntimeInstance{ReservedNatAddress: util.Ptr(true)}),
		"the same value as the handler loaded is still checked against the row")
	assert.Equal(t, 1, reservedNatAddressLocks(t, &GcpGkeKubernetesRuntimeInstance{ReservedNatAddress: util.Ptr(false)}))
	assert.Zero(t, reservedNatAddressLocks(t, &GcpGkeKubernetesRuntimeInstance{Region: util.Ptr("us-central1")}),
		"a PATCH without the field leaves it as it is")

	assert.Equal(t, 1, reservedNatAddressLocks(t, map[string]interface{}{"reserved_nat_address": true}), "a map by column")
	assert.Equal(t, 1, reservedNatAddressLocks(t, map[string]interface{}{"ReservedNatAddress": util.Ptr(true)}), "a map by field")
	assert.Equal(t, 1, reservedNatAddressLocks(t, map[string]interface{}{"reserved_nat_address": nil}), "a map clearing it")
	assert.Zero(t, reservedNatAddressLocks(t, map[string]interface{}{"region": "us-central1"}), "a map without it")
}

// TestAReservedNatAddressPatchIsCheckedAgainstTheStoredRow covers a PATCH
// whose handler loaded the row before something else changed it: it is
// checked against the row as it is when the PATCH writes over it, not as
// the handler loaded it.
func TestAReservedNatAddressPatchIsCheckedAgainstTheStoredRow(t *testing.T) {
	object := func(value *bool) any { return &GcpGkeKubernetesRuntimeInstance{ReservedNatAddress: value} }
	column := func(value *bool) any {
		if value == nil {
			return map[string]interface{}{"reserved_nat_address": nil}
		}
		return map[string]interface{}{"reserved_nat_address": *value}
	}
	cases := []struct {
		name    string
		loaded  *bool
		stored  *bool
		patch   any
		wantErr bool
	}{
		{"setting what the handler loaded over a row without it", util.Ptr(true), nil, object(util.Ptr(true)), true},
		{"clearing what the handler loaded over a row that has it", nil, util.Ptr(true), object(util.Ptr(false)), true},
		{"setting what the row already has, though the handler loaded it without", nil, util.Ptr(true), object(util.Ptr(true)), false},
		{"clearing what the row no longer has, though the handler loaded it", util.Ptr(true), nil, object(util.Ptr(false)), false},
		{"a map setting what the handler loaded over a row without it", util.Ptr(true), nil, column(util.Ptr(true)), true},
		{"a map clearing what the handler loaded over a row that has it", nil, util.Ptr(true), column(nil), true},
		{"a map setting what the row already has", nil, util.Ptr(true), column(util.Ptr(true)), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			db := setupKubernetesApiAccessDB(t)
			require.NoError(t, db.AutoMigrate(&GcpGkeKubernetesRuntimeInstance{}))
			created := &GcpGkeKubernetesRuntimeInstance{
				Instance:                            Instance{Name: util.Ptr("gke")},
				GcpProviderID:                       util.Ptr(uint(1)),
				GcpGkeKubernetesRuntimeDefinitionID: util.Ptr(uint(1)),
				KubernetesRuntimeInstanceID:         util.Ptr(uint(1)),
				ReservedNatAddress:                  c.loaded,
			}
			require.NoError(t, db.Create(created).Error)
			var loaded GcpGkeKubernetesRuntimeInstance
			require.NoError(t, db.First(&loaded, *created.ID).Error)

			// the row changes after the handler loaded it
			require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).Model(&GcpGkeKubernetesRuntimeInstance{}).
				Where("id = ?", *created.ID).Update("reserved_nat_address", c.stored).Error)

			err := db.Model(&loaded).Updates(c.patch).Error
			if c.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), ErrMsgReservedNatAddressImmutable)
				return
			}
			require.NoError(t, err)
		})
	}
}

// TestAReservedNatAddressPatchWithAValueItCannotReadIsRefused covers a map
// PATCH that sets ReservedNatAddress to something that isn't a bool: it is
// refused rather than written unchecked.
func TestAReservedNatAddressPatchWithAValueItCannotReadIsRefused(t *testing.T) {
	db := setupKubernetesApiAccessDB(t)
	require.NoError(t, db.AutoMigrate(&GcpGkeKubernetesRuntimeInstance{}))
	created := &GcpGkeKubernetesRuntimeInstance{
		Instance:                            Instance{Name: util.Ptr("gke")},
		GcpProviderID:                       util.Ptr(uint(1)),
		GcpGkeKubernetesRuntimeDefinitionID: util.Ptr(uint(1)),
		KubernetesRuntimeInstanceID:         util.Ptr(uint(1)),
	}
	require.NoError(t, db.Create(created).Error)

	err := db.Model(created).Updates(map[string]interface{}{"reserved_nat_address": "yes"}).Error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot check ReservedNatAddress: update sets it to string")
}
