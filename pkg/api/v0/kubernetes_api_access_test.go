package v0

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	util "github.com/threeport/threeport/pkg/util/v0"
)

// cidrsJSON returns cidrs as the JSON array the API stores.
func cidrsJSON(t *testing.T, cidrs ...string) *datatypes.JSON {
	t.Helper()
	raw, err := json.Marshal(cidrs)
	require.NoError(t, err)
	value := datatypes.JSON(raw)
	return &value
}

// rawJSON returns raw as a stored JSON value.
func rawJSON(raw string) *datatypes.JSON {
	value := datatypes.JSON(raw)
	return &value
}

// distinctCidrs returns n different /32 blocks.
func distinctCidrs(n int) []string {
	cidrs := make([]string, n)
	for i := range cidrs {
		cidrs[i] = fmt.Sprintf("198.51.%d.%d/32", i/256, i%256)
	}
	return cidrs
}

// TestCidrListsAcceptOnlyIPv4NetworkBlocks covers #729: AllowedCidrs and
// EgressCidrs hold IPv4 CIDR blocks written as their network address, with
// no repeats, so a value the providers would refuse or read differently is
// turned away when it is stored rather than when a cluster is provisioned.
func TestCidrListsAcceptOnlyIPv4NetworkBlocks(t *testing.T) {
	cases := []struct {
		name    string
		raw     *datatypes.JSON
		want    []string
		wantErr string
	}{
		{"unset", nil, nil, ""},
		{"null", rawJSON("null"), nil, ""},
		{"empty", rawJSON("[]"), []string{}, ""},
		{"blocks", rawJSON(`["203.0.113.0/24","198.51.100.7/32"]`), []string{"203.0.113.0/24", "198.51.100.7/32"}, ""},
		{"the whole internet", rawJSON(`["0.0.0.0/0"]`), []string{"0.0.0.0/0"}, ""},
		{"not an array", rawJSON(`"203.0.113.0/24"`), nil, "must be a JSON array of CIDR blocks"},
		{"not strings", rawJSON(`[24]`), nil, "must be a JSON array of CIDR blocks"},
		{"an address with no prefix", rawJSON(`["203.0.113.7"]`), nil, `"203.0.113.7" is not a CIDR block`},
		{"not an address", rawJSON(`["office/24"]`), nil, `"office/24" is not a CIDR block`},
		{"a prefix past 32", rawJSON(`["203.0.113.0/33"]`), nil, `"203.0.113.0/33" is not a CIDR block`},
		{"surrounding spaces", rawJSON(`[" 203.0.113.0/24"]`), nil, "is not a CIDR block"},
		{"IPv6", rawJSON(`["2001:db8::/32"]`), nil, `"2001:db8::/32" is not an IPv4 CIDR block`},
		{"IPv4 mapped into IPv6", rawJSON(`["::ffff:203.0.113.0/120"]`), nil, "is not an IPv4 CIDR block"},
		{"host bits set", rawJSON(`["203.0.113.7/24"]`), nil, `"203.0.113.7/24" has host bits set, write it as "203.0.113.0/24"`},
		{"a repeat", rawJSON(`["203.0.113.0/24","203.0.113.0/24"]`), nil, `"203.0.113.0/24" is listed more than once`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseCidrs("AllowedCidrs", c.raw)
			if c.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "AllowedCidrs")
				assert.Contains(t, err.Error(), c.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, c.want, got)
		})
	}
}

// TestEachProviderAdmitsItsOwnNumberOfAllowedCidrs pins the providers'
// documented limits, and that a kind runtime has no allowlist at all.
func TestEachProviderAdmitsItsOwnNumberOfAllowedCidrs(t *testing.T) {
	cases := []struct {
		provider     string
		privateApi   bool
		limit        int
		restrictable bool
	}{
		{KubernetesRuntimeInfraProviderEKS, false, 40, true},
		{KubernetesRuntimeInfraProviderEKS, true, 40, true},
		{KubernetesRuntimeInfraProviderGKE, false, 50, true},
		{KubernetesRuntimeInfraProviderGKE, true, 100, true},
		{KubernetesRuntimeInfraProviderOKE, false, 120, true},
		{KubernetesRuntimeInfraProviderOKE, true, 120, true},
		{KubernetesRuntimeInfraProviderKind, false, 0, false},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("%s private=%t", c.provider, c.privateApi), func(t *testing.T) {
			limit, restrictable := AllowedCidrsLimit(c.provider, c.privateApi)
			assert.Equal(t, c.limit, limit)
			assert.Equal(t, c.restrictable, restrictable)
		})
	}
}

// setupKubernetesApiAccessDB returns the kubernetes runtime validation DB
// with the provider definition tables added.
func setupKubernetesApiAccessDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := setupKubernetesRuntimeValidateDB(t)
	require.NoError(t, db.AutoMigrate(
		&AwsEksKubernetesRuntimeDefinition{},
		&GcpGkeKubernetesRuntimeDefinition{},
		&OciOkeKubernetesRuntimeDefinition{},
	))
	return db
}

// newKRD returns a definition on provider with the given access settings.
func newKRD(name, provider string, privateApi *bool, allowedCidrs *datatypes.JSON) *KubernetesRuntimeDefinition {
	return &KubernetesRuntimeDefinition{
		Definition:       Definition{Name: util.Ptr(name)},
		InfraProvider:    util.Ptr(provider),
		HighAvailability: util.Ptr(false),
		PrivateApi:       privateApi,
		AllowedCidrs:     allowedCidrs,
	}
}

// TestAKubernetesRuntimeDefinitionIsCheckedForItsProvider covers creating a
// definition: the allowlist is checked against the definition's own
// provider, and a kind runtime, whose API Threeport cannot restrict, refuses
// both fields rather than ignoring them.
func TestAKubernetesRuntimeDefinitionIsCheckedForItsProvider(t *testing.T) {
	cases := []struct {
		name         string
		provider     string
		privateApi   *bool
		allowedCidrs *datatypes.JSON
		wantErr      string
	}{
		{"nothing set, as today", KubernetesRuntimeInfraProviderEKS, nil, nil, ""},
		{"nothing set on kind", KubernetesRuntimeInfraProviderKind, nil, nil, ""},
		{"public on kind", KubernetesRuntimeInfraProviderKind, util.Ptr(false), rawJSON("[]"), ""},
		{"an allowlist", KubernetesRuntimeInfraProviderEKS, nil, rawJSON(`["203.0.113.0/24"]`), ""},
		{"private", KubernetesRuntimeInfraProviderGKE, util.Ptr(true), nil, ErrMsgPrivateApiNotSupported},
		{"private with an allowlist", KubernetesRuntimeInfraProviderOKE, util.Ptr(true), rawJSON(`["10.8.0.0/16"]`), ErrMsgPrivateApiNotSupported},
		{"explicitly public", KubernetesRuntimeInfraProviderGKE, util.Ptr(false), rawJSON(`["203.0.113.0/24"]`), ""},
		{"EKS at its limit", KubernetesRuntimeInfraProviderEKS, nil, cidrsJSON(t, distinctCidrs(40)...), ""},
		{"EKS past its limit", KubernetesRuntimeInfraProviderEKS, nil, cidrsJSON(t, distinctCidrs(41)...), "AllowedCidrs has 41 entries and eks admits at most 40"},
		{"public GKE past its limit", KubernetesRuntimeInfraProviderGKE, util.Ptr(false), cidrsJSON(t, distinctCidrs(51)...), "admits at most 50"},
		{"OKE past its limit", KubernetesRuntimeInfraProviderOKE, nil, cidrsJSON(t, distinctCidrs(121)...), "admits at most 120"},
		{"a bad block", KubernetesRuntimeInfraProviderEKS, nil, rawJSON(`["203.0.113.7/24"]`), "has host bits set"},
		{"private on kind", KubernetesRuntimeInfraProviderKind, util.Ptr(true), nil, "not supported on kind runtimes"},
		{"an allowlist on kind", KubernetesRuntimeInfraProviderKind, nil, rawJSON(`["203.0.113.0/24"]`), "not supported on kind runtimes"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			db := setupKubernetesApiAccessDB(t)
			err := db.Create(newKRD("krd", c.provider, c.privateApi, c.allowedCidrs)).Error
			if c.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), c.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

// TestADefinitionCreatedWithoutTheFieldsKeepsAPublicApi covers the default:
// a definition that sets neither field is stored public and unrestricted.
func TestADefinitionCreatedWithoutTheFieldsKeepsAPublicApi(t *testing.T) {
	db := setupKubernetesApiAccessDB(t)
	created := newKRD("krd", KubernetesRuntimeInfraProviderEKS, nil, nil)
	require.NoError(t, db.Create(created).Error)

	var loaded KubernetesRuntimeDefinition
	require.NoError(t, db.First(&loaded, *created.ID).Error)
	require.NotNil(t, loaded.PrivateApi)
	assert.False(t, *loaded.PrivateApi)
	assert.Nil(t, loaded.AllowedCidrs)
}

// TestAnUpdateIsCheckedAgainstWhatTheDefinitionWillHold covers both update
// shapes: the values an update would leave are validated, and refused when
// they would change how the cluster is restricted, since nothing applies the
// change to it.  An update that sends the same restriction in another form
// is accepted.
func TestAnUpdateIsCheckedAgainstWhatTheDefinitionWillHold(t *testing.T) {
	cases := []struct {
		name     string
		provider string
		stored   *datatypes.JSON
		update   func(db *gorm.DB, loaded KubernetesRuntimeDefinition) error
		wantErr  string
	}{
		{
			name: "patch in an allowlist", provider: KubernetesRuntimeInfraProviderEKS,
			update: func(db *gorm.DB, loaded KubernetesRuntimeDefinition) error {
				return db.Model(&loaded).Updates(&KubernetesRuntimeDefinition{AllowedCidrs: rawJSON(`["203.0.113.0/24"]`)}).Error
			},
			wantErr: ErrMsgKubernetesApiAccessImmutable,
		},
		{
			name: "patch another allowlist over a stored one", provider: KubernetesRuntimeInfraProviderEKS,
			stored: rawJSON(`["203.0.113.0/24"]`),
			update: func(db *gorm.DB, loaded KubernetesRuntimeDefinition) error {
				return db.Model(&loaded).Updates(&KubernetesRuntimeDefinition{AllowedCidrs: rawJSON(`["198.51.100.0/24"]`)}).Error
			},
			wantErr: ErrMsgKubernetesApiAccessImmutable,
		},
		{
			name: "patch the same blocks in another order", provider: KubernetesRuntimeInfraProviderEKS,
			stored: rawJSON(`["203.0.113.0/24","198.51.100.0/24"]`),
			update: func(db *gorm.DB, loaded KubernetesRuntimeDefinition) error {
				return db.Model(&loaded).Updates(&KubernetesRuntimeDefinition{AllowedCidrs: rawJSON(`["198.51.100.0/24", "203.0.113.0/24"]`)}).Error
			},
		},
		{
			name: "patch an empty list over none", provider: KubernetesRuntimeInfraProviderEKS,
			update: func(db *gorm.DB, loaded KubernetesRuntimeDefinition) error {
				return db.Model(&loaded).Updates(&KubernetesRuntimeDefinition{AllowedCidrs: rawJSON(`[]`), PrivateApi: util.Ptr(false)}).Error
			},
		},
		{
			name: "patch a bad block", provider: KubernetesRuntimeInfraProviderEKS,
			update: func(db *gorm.DB, loaded KubernetesRuntimeDefinition) error {
				return db.Model(&loaded).Updates(&KubernetesRuntimeDefinition{AllowedCidrs: rawJSON(`["2001:db8::/32"]`)}).Error
			},
			wantErr: "is not an IPv4 CIDR block",
		},
		{
			name: "patch past the limit", provider: KubernetesRuntimeInfraProviderEKS,
			update: func(db *gorm.DB, loaded KubernetesRuntimeDefinition) error {
				return db.Model(&loaded).Updates(&KubernetesRuntimeDefinition{AllowedCidrs: cidrsJSON(t, distinctCidrs(41)...)}).Error
			},
			wantErr: "admits at most 40",
		},
		{
			name: "patch PrivateApi on", provider: KubernetesRuntimeInfraProviderGKE,
			stored: rawJSON(`["203.0.113.0/24"]`),
			update: func(db *gorm.DB, loaded KubernetesRuntimeDefinition) error {
				return db.Model(&loaded).Updates(&KubernetesRuntimeDefinition{PrivateApi: util.Ptr(true)}).Error
			},
			wantErr: ErrMsgPrivateApiNotSupported,
		},
		{
			name: "replace with PrivateApi on", provider: KubernetesRuntimeInfraProviderEKS,
			update: func(db *gorm.DB, loaded KubernetesRuntimeDefinition) error {
				loaded.PrivateApi = util.Ptr(true)
				return db.Save(&loaded).Error
			},
			wantErr: ErrMsgPrivateApiNotSupported,
		},
		{
			name: "patch PrivateApi with a map", provider: KubernetesRuntimeInfraProviderGKE,
			update: func(db *gorm.DB, loaded KubernetesRuntimeDefinition) error {
				return db.Model(&loaded).Updates(map[string]any{"PrivateApi": true}).Error
			},
			wantErr: "cannot validate Kubernetes API access",
		},
		{
			name: "patch PrivateApi on on kind", provider: KubernetesRuntimeInfraProviderKind,
			update: func(db *gorm.DB, loaded KubernetesRuntimeDefinition) error {
				return db.Model(&loaded).Updates(&KubernetesRuntimeDefinition{PrivateApi: util.Ptr(true)}).Error
			},
			wantErr: "not supported on kind runtimes",
		},
		{
			name: "patch an unrelated field over a stored list", provider: KubernetesRuntimeInfraProviderEKS,
			stored: rawJSON(`["203.0.113.0/24"]`),
			update: func(db *gorm.DB, loaded KubernetesRuntimeDefinition) error {
				return db.Model(&loaded).Updates(&KubernetesRuntimeDefinition{NodeSize: util.Ptr("Large")}).Error
			},
		},
		{
			name: "replace with a bad block", provider: KubernetesRuntimeInfraProviderEKS,
			update: func(db *gorm.DB, loaded KubernetesRuntimeDefinition) error {
				loaded.AllowedCidrs = rawJSON(`["203.0.113.7/24"]`)
				return db.Save(&loaded).Error
			},
			wantErr: "has host bits set",
		},
		{
			name: "replace clearing the list", provider: KubernetesRuntimeInfraProviderEKS,
			stored: rawJSON(`["203.0.113.0/24"]`),
			update: func(db *gorm.DB, loaded KubernetesRuntimeDefinition) error {
				loaded.AllowedCidrs = nil
				return db.Save(&loaded).Error
			},
			wantErr: ErrMsgKubernetesApiAccessImmutable,
		},
		{
			name: "replace keeping the list", provider: KubernetesRuntimeInfraProviderEKS,
			stored: rawJSON(`["203.0.113.0/24"]`),
			update: func(db *gorm.DB, loaded KubernetesRuntimeDefinition) error {
				loaded.NodeSize = util.Ptr("Large")
				loaded.AllowedCidrs = rawJSON(`[ "203.0.113.0/24" ]`)
				return db.Save(&loaded).Error
			},
		},
		{
			name: "replace an empty list with none", provider: KubernetesRuntimeInfraProviderEKS,
			stored: rawJSON(`[]`),
			update: func(db *gorm.DB, loaded KubernetesRuntimeDefinition) error {
				loaded.AllowedCidrs = nil
				return db.Save(&loaded).Error
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			db := setupKubernetesApiAccessDB(t)
			created := newKRD("krd", c.provider, nil, c.stored)
			require.NoError(t, db.Create(created).Error)
			var loaded KubernetesRuntimeDefinition
			require.NoError(t, db.First(&loaded, *created.ID).Error)

			err := c.update(db, loaded)
			if c.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), c.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

// TestTheProviderDefinitionsAreCheckedLikeTheirKubernetesRuntimeDefinition
// covers the EKS, GKE and OKE definitions, which can be created through the
// API directly, and are checked against their own provider's limit.
func TestTheProviderDefinitionsAreCheckedLikeTheirKubernetesRuntimeDefinition(t *testing.T) {
	krdID := util.Ptr(uint(1))
	common := func(name string) Definition { return Definition{Name: util.Ptr(name)} }
	eks := func(cidrs *datatypes.JSON) any {
		return &AwsEksKubernetesRuntimeDefinition{
			Definition: common("eks"), ZoneCount: util.Ptr(2), DefaultNodeGroupInstanceType: util.Ptr("m5.large"),
			DefaultNodeGroupInitialSize: util.Ptr(2), DefaultNodeGroupMinimumSize: util.Ptr(0), DefaultNodeGroupMaximumSize: util.Ptr(5),
			KubernetesRuntimeDefinitionID: krdID, AllowedCidrs: cidrs,
		}
	}
	gke := func(cidrs *datatypes.JSON) any {
		return &GcpGkeKubernetesRuntimeDefinition{
			Definition: common("gke"), ZoneCount: util.Ptr(2), DefaultNodeGroupInstanceType: util.Ptr("e2-standard-4"),
			DefaultNodeGroupInitialSize: util.Ptr(2), DefaultNodeGroupMinimumSize: util.Ptr(0), DefaultNodeGroupMaximumSize: util.Ptr(5),
			KubernetesRuntimeDefinitionID: krdID, AllowedCidrs: cidrs,
		}
	}
	oke := func(cidrs *datatypes.JSON) any {
		return &OciOkeKubernetesRuntimeDefinition{
			Definition: common("oke"), WorkerNodeShape: util.Ptr("VM.Standard.E4.Flex"), WorkerNodeInitialCount: util.Ptr(int32(2)),
			KubernetesRuntimeDefinitionID: krdID, AllowedCidrs: cidrs,
		}
	}

	cases := []struct {
		name    string
		build   func(*datatypes.JSON) any
		limit   int
		pkgName string
	}{
		{"EKS", eks, 40, "eks"},
		{"GKE", gke, 50, "gke"},
		{"OKE", oke, 120, "oke"},
	}
	for _, c := range cases {
		t.Run(c.name+" at its limit", func(t *testing.T) {
			db := setupKubernetesApiAccessDB(t)
			require.NoError(t, db.Create(c.build(cidrsJSON(t, distinctCidrs(c.limit)...))).Error)
		})
		t.Run(c.name+" past its limit", func(t *testing.T) {
			db := setupKubernetesApiAccessDB(t)
			err := db.Create(c.build(cidrsJSON(t, distinctCidrs(c.limit+1)...))).Error
			require.Error(t, err)
			assert.Contains(t, err.Error(), fmt.Sprintf("%s admits at most %d", c.pkgName, c.limit))
		})
		t.Run(c.name+" with a bad block", func(t *testing.T) {
			db := setupKubernetesApiAccessDB(t)
			err := db.Create(c.build(rawJSON(`["203.0.113.7/24"]`))).Error
			require.Error(t, err)
			assert.Contains(t, err.Error(), "has host bits set")
		})
		t.Run(c.name+" patched with another list", func(t *testing.T) {
			db := setupKubernetesApiAccessDB(t)
			created := c.build(rawJSON(`["203.0.113.0/24"]`))
			require.NoError(t, db.Create(created).Error)
			err := db.Model(created).Updates(c.build(rawJSON(`["198.51.100.0/24"]`))).Error
			require.Error(t, err)
			assert.Contains(t, err.Error(), ErrMsgKubernetesApiAccessImmutable)
		})
		t.Run(c.name+" patched with a bad block", func(t *testing.T) {
			db := setupKubernetesApiAccessDB(t)
			created := c.build(nil)
			require.NoError(t, db.Create(created).Error)
			patch := c.build(rawJSON(`["2001:db8::/32"]`))
			err := db.Model(created).Updates(patch).Error
			require.Error(t, err)
			assert.Contains(t, err.Error(), "is not an IPv4 CIDR block")
		})
	}
}

// TestAnInstancesEgressCidrsAreChecked covers EgressCidrs on create and on
// both update shapes.
func TestAnInstancesEgressCidrsAreChecked(t *testing.T) {
	t.Run("created with addresses", func(t *testing.T) {
		db := setupKubernetesApiAccessDB(t)
		loaded := createValidKRI(t, db, "NorthAmerica:NewYork")
		assert.Nil(t, loaded.EgressCidrs)

		created := &KubernetesRuntimeInstance{
			Instance:                      Instance{Name: util.Ptr("with-egress")},
			Location:                      util.Ptr("NorthAmerica:NewYork"),
			KubernetesRuntimeDefinitionID: loaded.KubernetesRuntimeDefinitionID,
			EgressCidrs:                   rawJSON(`["198.51.100.7/32","198.51.100.8/32"]`),
		}
		require.NoError(t, db.Create(created).Error)
	})
	t.Run("created with a bad address", func(t *testing.T) {
		db := setupKubernetesApiAccessDB(t)
		loaded := createValidKRI(t, db, "NorthAmerica:NewYork")
		created := &KubernetesRuntimeInstance{
			Instance:                      Instance{Name: util.Ptr("bad-egress")},
			Location:                      util.Ptr("NorthAmerica:NewYork"),
			KubernetesRuntimeDefinitionID: loaded.KubernetesRuntimeDefinitionID,
			EgressCidrs:                   rawJSON(`["198.51.100.7"]`),
		}
		err := db.Create(created).Error
		require.Error(t, err)
		assert.Contains(t, err.Error(), `EgressCidrs entry "198.51.100.7" is not a CIDR block`)
	})
	t.Run("patched with addresses", func(t *testing.T) {
		db := setupKubernetesApiAccessDB(t)
		loaded := createValidKRI(t, db, "NorthAmerica:NewYork")
		patch := &KubernetesRuntimeInstance{EgressCidrs: rawJSON(`["198.51.100.7/32"]`)}
		require.NoError(t, db.Model(&loaded).Updates(patch).Error)

		var reloaded KubernetesRuntimeInstance
		require.NoError(t, db.First(&reloaded, *loaded.ID).Error)
		got, err := ParseCidrs("EgressCidrs", reloaded.EgressCidrs)
		require.NoError(t, err)
		assert.Equal(t, []string{"198.51.100.7/32"}, got)
	})
	t.Run("patched with a repeat", func(t *testing.T) {
		db := setupKubernetesApiAccessDB(t)
		loaded := createValidKRI(t, db, "NorthAmerica:NewYork")
		patch := &KubernetesRuntimeInstance{EgressCidrs: rawJSON(`["198.51.100.7/32","198.51.100.7/32"]`)}
		err := db.Model(&loaded).Updates(patch).Error
		require.Error(t, err)
		assert.Contains(t, err.Error(), "is listed more than once")
	})
	t.Run("replaced with IPv6", func(t *testing.T) {
		db := setupKubernetesApiAccessDB(t)
		loaded := createValidKRI(t, db, "NorthAmerica:NewYork")
		loaded.EgressCidrs = rawJSON(`["2001:db8::1/128"]`)
		err := db.Save(&loaded).Error
		require.Error(t, err)
		assert.Contains(t, err.Error(), "is not an IPv4 CIDR block")
	})
}

// TestAPatchIsCheckedAgainstTheRowItWritesOver covers a PATCH of a
// definition that another write changed after the PATCH's handler loaded it.
// The PATCH is checked against the row as that write left it, not as the
// handler loaded it: sending back the values it loaded would otherwise pass as
// no change and undo the other write.
func TestAPatchIsCheckedAgainstTheRowItWritesOver(t *testing.T) {
	types := []struct {
		name   string
		create func(t *testing.T, db *gorm.DB, privateApi *bool, cidrs *datatypes.JSON) uint
		load   func(t *testing.T, db *gorm.DB, id uint) any
		patch  func(privateApi *bool, cidrs *datatypes.JSON) any
		model  any
	}{
		{
			name: "KubernetesRuntimeDefinition",
			create: func(t *testing.T, db *gorm.DB, privateApi *bool, cidrs *datatypes.JSON) uint {
				created := newKRD("krd", KubernetesRuntimeInfraProviderGKE, privateApi, cidrs)
				require.NoError(t, db.Create(created).Error)
				return *created.ID
			},
			load: func(t *testing.T, db *gorm.DB, id uint) any {
				var loaded KubernetesRuntimeDefinition
				require.NoError(t, db.First(&loaded, id).Error)
				return &loaded
			},
			patch: func(privateApi *bool, cidrs *datatypes.JSON) any {
				return &KubernetesRuntimeDefinition{PrivateApi: privateApi, AllowedCidrs: cidrs}
			},
			model: &KubernetesRuntimeDefinition{},
		},
		{
			name: "GcpGkeKubernetesRuntimeDefinition",
			create: func(t *testing.T, db *gorm.DB, privateApi *bool, cidrs *datatypes.JSON) uint {
				created := &GcpGkeKubernetesRuntimeDefinition{
					Definition: Definition{Name: util.Ptr("gke")}, ZoneCount: util.Ptr(2),
					DefaultNodeGroupInstanceType: util.Ptr("e2-standard-4"), DefaultNodeGroupInitialSize: util.Ptr(2),
					DefaultNodeGroupMinimumSize: util.Ptr(0), DefaultNodeGroupMaximumSize: util.Ptr(5),
					KubernetesRuntimeDefinitionID: util.Ptr(uint(1)), PrivateApi: privateApi, AllowedCidrs: cidrs,
				}
				require.NoError(t, db.Create(created).Error)
				return *created.ID
			},
			load: func(t *testing.T, db *gorm.DB, id uint) any {
				var loaded GcpGkeKubernetesRuntimeDefinition
				require.NoError(t, db.First(&loaded, id).Error)
				return &loaded
			},
			patch: func(privateApi *bool, cidrs *datatypes.JSON) any {
				return &GcpGkeKubernetesRuntimeDefinition{PrivateApi: privateApi, AllowedCidrs: cidrs}
			},
			model: &GcpGkeKubernetesRuntimeDefinition{},
		},
	}

	cases := []struct {
		name string
		// written is what the other write leaves in the row
		written map[string]any
		// patchPrivateApi and patchCidrs are what the PATCH sends
		patchPrivateApi *bool
		patchCidrs      int
		wantErr         string
	}{
		{
			name:            "PrivateApi sent back as loaded after the other write turned it on",
			written:         map[string]any{"private_api": true},
			patchPrivateApi: util.Ptr(false),
			wantErr:         ErrMsgKubernetesApiAccessImmutable,
		},
		{
			name:       "the list sent back as loaded after the other write shrank it",
			written:    map[string]any{"allowed_cidrs": cidrsJSON(t, distinctCidrs(30)...)},
			patchCidrs: 40,
			wantErr:    ErrMsgKubernetesApiAccessImmutable,
		},
		{
			name:       "the list the other write left sent back",
			written:    map[string]any{"allowed_cidrs": cidrsJSON(t, distinctCidrs(30)...)},
			patchCidrs: 30,
		},
	}
	for _, objectType := range types {
		for _, c := range cases {
			t.Run(objectType.name+": "+c.name, func(t *testing.T) {
				db := setupKubernetesApiAccessDB(t)
				id := objectType.create(t, db, util.Ptr(false), cidrsJSON(t, distinctCidrs(40)...))

				// the PATCH's handler loads the definition, then the other
				// write changes it
				loaded := objectType.load(t, db, id)
				require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).
					Model(objectType.model).Where("id = ?", id).Updates(c.written).Error)

				var cidrs *datatypes.JSON
				if c.patchCidrs > 0 {
					cidrs = cidrsJSON(t, distinctCidrs(c.patchCidrs)...)
				}
				err := db.Model(loaded).Updates(objectType.patch(c.patchPrivateApi, cidrs)).Error
				if c.wantErr == "" {
					require.NoError(t, err)
					return
				}
				require.Error(t, err)
				assert.Contains(t, err.Error(), c.wantErr)
			})
		}
	}
}

// TestAPatchLocksTheRowItIsCheckedAgainst checks that the row a PATCH is
// checked against is read with SELECT ... FOR UPDATE, so that a concurrent
// update of the same row waits for this one's transaction to end.  SQLite
// leaves the lock out, so the statement is rendered for Postgres, the
// dialect the API server talks to CockroachDB in.
func TestAPatchLocksTheRowItIsCheckedAgainst(t *testing.T) {
	db := dryRunDB(t)
	var queries []string
	require.NoError(t, db.Callback().Query().After("gorm:query").Register(
		"test:record_query",
		func(tx *gorm.DB) { queries = append(queries, tx.Statement.SQL.String()) },
	))

	loaded := newKRD("krd", KubernetesRuntimeInfraProviderGKE, util.Ptr(true), nil)
	loaded.ID = util.Ptr(uint(7))
	_ = db.Model(loaded).Updates(&KubernetesRuntimeDefinition{PrivateApi: util.Ptr(false)})

	locked := 0
	for _, query := range queries {
		if strings.HasPrefix(query, `SELECT * FROM "v0_kubernetes_runtime_definitions" WHERE`) &&
			strings.HasSuffix(query, "FOR UPDATE") {
			locked++
		}
	}
	assert.Equal(t, 1, locked, "queries: %q", queries)
}
