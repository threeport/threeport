package v0

import (
	"context"
	"encoding/json"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	lib "github.com/threeport/threeport/pkg/api/lib/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// prefixes parses each CIDR block.
func prefixes(t *testing.T, cidrs ...string) []netip.Prefix {
	t.Helper()
	parsed := make([]netip.Prefix, len(cidrs))
	for i, cidr := range cidrs {
		parsed[i] = netip.MustParsePrefix(cidr)
	}
	return parsed
}

// poolJSON returns cidrs as the JSON array NetworkCidrPool stores.
func poolJSON(t *testing.T, cidrs ...string) *datatypes.JSON {
	t.Helper()
	raw, err := json.Marshal(cidrs)
	require.NoError(t, err)
	value := datatypes.JSON(raw)
	return &value
}

// TestANetworkCidrIsARoutableBlockClearOfTheClusterInternalRanges covers the
// ranges a runtime may be given by hand.
func TestANetworkCidrIsARoutableBlockClearOfTheClusterInternalRanges(t *testing.T) {
	for _, valid := range []string{"10.16.0.0/16", "10.0.0.0/15", "192.168.0.0/16", "172.21.0.0/16", "172.17.0.0/16"} {
		_, err := ParseNetworkCidr("NetworkCidr", valid)
		assert.NoError(t, err, valid)
	}

	for cidr, want := range map[string]string{
		"10.16.0.0/17":    "smaller than a /16",
		"10.16.1.0/16":    `has host bits set, write it as "10.16.0.0/16"`,
		"fd00::/16":       "not an IPv4 CIDR block",
		"10.16.0.0":       "not a CIDR block",
		"10.96.0.0/16":    "overlaps 10.96.0.0/16, which clusters use internally",
		"10.244.0.0/16":   "overlaps 10.244.0.0/16",
		"172.20.0.0/16":   "overlaps 172.20.0.0/16",
		"172.16.0.0/12":   "overlaps 172.20.0.0/16",
		"172.16.0.0/16":   "overlaps 172.16.0.0/28, which clusters use internally",
		"10.0.0.0/8":      "overlaps 10.96.0.0/16",
		"10.240.0.0/12":   "overlaps 10.244.0.0/16",
		"203.0.113.0/250": "not a CIDR block",
	} {
		_, err := ParseNetworkCidr("NetworkCidr", cidr)
		if assert.Error(t, err, cidr) {
			assert.Contains(t, err.Error(), want, cidr)
		}
	}
}

// TestANetworkCidrPoolHoldsBlocksOfASixteenOrLargerThatDontOverlap covers
// the pool a control plane allocates from.
func TestANetworkCidrPoolHoldsBlocksOfASixteenOrLargerThatDontOverlap(t *testing.T) {
	pool, err := ParseNetworkCidrPool(poolJSON(t, "10.16.0.0/12", "192.168.0.0/16", "172.16.0.0/12"))
	require.NoError(t, err)
	assert.Equal(t, prefixes(t, "10.16.0.0/12", "192.168.0.0/16", "172.16.0.0/12"), pool,
		"a pool may include cluster-internal ranges, which allocation skips")

	for _, empty := range []*datatypes.JSON{nil, poolJSON(t), rawJSON("null")} {
		pool, err := ParseNetworkCidrPool(empty)
		require.NoError(t, err)
		assert.Empty(t, pool)
	}

	for name, test := range map[string]struct {
		pool *datatypes.JSON
		want string
	}{
		"smaller than a /16": {poolJSON(t, "10.16.0.0/16", "10.17.0.0/20"), "smaller than a /16"},
		"overlapping":        {poolJSON(t, "10.16.0.0/12", "10.20.0.0/16"), "NetworkCidrPool entries 10.16.0.0/12 and 10.20.0.0/16 overlap"},
		"repeated":           {poolJSON(t, "10.16.0.0/16", "10.16.0.0/16"), "listed more than once"},
		"host bits":          {poolJSON(t, "10.16.0.1/16"), "has host bits set"},
		"IPv6":               {poolJSON(t, "fd00::/8"), "not an IPv4 CIDR block"},
		"not a list":         {rawJSON(`"10.16.0.0/12"`), "must be a JSON array"},
	} {
		_, err := ParseNetworkCidrPool(test.pool)
		if assert.Error(t, err, name) {
			assert.Contains(t, err.Error(), test.want, name)
		}
	}
}

// TestTheDefaultPoolHoldsTwoHundredAndThirtyEightBlocks covers the default
// pool: 10.16.0.0/16 through 10.255.0.0/16, in order, without the two OKE
// ranges.
func TestTheDefaultPoolHoldsTwoHundredAndThirtyEightBlocks(t *testing.T) {
	pool, err := NetworkCidrPoolOrDefault(nil)
	require.NoError(t, err)

	var allocated []netip.Prefix
	for {
		block, err := AllocateNetworkCidr(pool, allocated)
		if err != nil {
			assert.Contains(t, err.Error(), "no /16 is left to allocate in the network CIDR pool 10.16.0.0/12, 10.32.0.0/11, 10.64.0.0/10, 10.128.0.0/9")
			break
		}
		allocated = append(allocated, block)
		require.LessOrEqual(t, len(allocated), 256, "allocation does not end")
	}

	require.Len(t, allocated, 238)
	assert.Equal(t, netip.MustParsePrefix("10.16.0.0/16"), allocated[0])
	assert.Equal(t, netip.MustParsePrefix("10.255.0.0/16"), allocated[len(allocated)-1])
	second := 16
	for _, block := range allocated {
		assert.Equal(t, 16, block.Bits())
		assert.NotContains(t, []string{"10.96.0.0/16", "10.244.0.0/16"}, block.String())
		assert.False(t, block.Overlaps(netip.MustParsePrefix("10.0.0.0/12")), "clusters provisioned before allocation use 10.0.0.0/12")
		// in order, one /16 after another apart from the two skipped
		octet := int(block.Addr().As4()[1])
		if octet == 97 || octet == 245 {
			second++
		}
		assert.Equal(t, second, octet)
		second++
	}

	pool, err = NetworkCidrPoolOrDefault(poolJSON(t, "192.168.0.0/16"))
	require.NoError(t, err)
	assert.Equal(t, prefixes(t, "192.168.0.0/16"), pool, "a pool that is set replaces the default")
}

// TestAllocationSkipsWhatIsInUse covers the blocks allocation passes over:
// other runtimes' ranges of any size, children's pools, and the
// cluster-internal ranges even when the pool includes them.
func TestAllocationSkipsWhatIsInUse(t *testing.T) {
	for name, test := range map[string]struct {
		pool  []string
		inUse []string
		want  string
	}{
		"the first block":                          {[]string{"10.16.0.0/12"}, nil, "10.16.0.0/16"},
		"a runtime's /16":                          {[]string{"10.16.0.0/12"}, []string{"10.16.0.0/16"}, "10.17.0.0/16"},
		"a runtime's larger range":                 {[]string{"10.16.0.0/12"}, []string{"10.16.0.0/15"}, "10.18.0.0/16"},
		"a child's pool":                           {[]string{"10.16.0.0/12"}, []string{"10.16.0.0/13", "10.24.0.0/14"}, "10.28.0.0/16"},
		"a gap left by a deleted runtime":          {[]string{"10.16.0.0/12"}, []string{"10.17.0.0/16"}, "10.16.0.0/16"},
		"an internal range inside a custom pool":   {[]string{"172.20.0.0/15"}, nil, "172.21.0.0/16"},
		"the next entry once one is full":          {[]string{"10.16.0.0/16", "192.168.0.0/16"}, []string{"10.16.0.0/16"}, "192.168.0.0/16"},
		"entries in the order the pool lists them": {[]string{"192.168.0.0/16", "10.16.0.0/12"}, nil, "192.168.0.0/16"},
		"the last block of the address space":      {[]string{"255.255.0.0/16"}, nil, "255.255.0.0/16"},
	} {
		block, err := AllocateNetworkCidr(prefixes(t, test.pool...), prefixes(t, test.inUse...))
		require.NoError(t, err, name)
		assert.Equal(t, test.want, block.String(), name)
	}

	_, err := AllocateNetworkCidr(prefixes(t, "10.96.0.0/16"), nil)
	assert.ErrorContains(t, err, "no /16 is left to allocate in the network CIDR pool 10.96.0.0/16")
	_, err = AllocateNetworkCidr(prefixes(t, "10.16.0.0/15"), prefixes(t, "10.16.0.0/16", "10.17.0.0/16"))
	assert.ErrorContains(t, err, "no /16 is left")
}

// runtimeWithNetworkCidr creates a kubernetes runtime instance with a network
// range, returning the create's error.
func runtimeWithNetworkCidr(t *testing.T, db *gorm.DB, name string, networkCidr *string) (*KubernetesRuntimeInstance, error) {
	t.Helper()
	definition := &KubernetesRuntimeDefinition{
		Definition:    Definition{Name: util.Ptr(name + "-definition")},
		InfraProvider: util.Ptr(KubernetesRuntimeInfraProviderEKS),
	}
	require.NoError(t, db.Create(definition).Error)
	instance := &KubernetesRuntimeInstance{
		Instance:                      Instance{Name: util.Ptr(name)},
		Location:                      util.Ptr("Local"),
		KubernetesRuntimeDefinitionID: definition.ID,
		NetworkCidr:                   networkCidr,
	}

	return instance, db.Create(instance).Error
}

// TestNoTwoRuntimesHoldOverlappingNetworkRanges covers NetworkCidr on a
// kubernetes runtime instance: no two overlap, one may be given outside any
// pool, and a range is set once.
func TestNoTwoRuntimesHoldOverlappingNetworkRanges(t *testing.T) {
	t.Run("on create", func(t *testing.T) {
		db := setupControlPlaneAuthDB(t)
		_, err := runtimeWithNetworkCidr(t, db, "first", util.Ptr("10.16.0.0/16"))
		require.NoError(t, err)
		_, err = runtimeWithNetworkCidr(t, db, "unallocated", nil)
		require.NoError(t, err, "a runtime the control plane has not given a range yet")
		_, err = runtimeWithNetworkCidr(t, db, "outside-the-pool", util.Ptr("192.168.0.0/16"))
		require.NoError(t, err)

		for name, cidr := range map[string]string{
			"same-block":       "10.16.0.0/16",
			"block-holding-it": "10.16.0.0/15",
		} {
			_, err := runtimeWithNetworkCidr(t, db, name, util.Ptr(cidr))
			if assert.Error(t, err, name) {
				assert.Contains(t, err.Error(), "overlaps 10.16.0.0/16, the network range of kubernetes runtime instance first", name)
			}
		}
		_, err = runtimeWithNetworkCidr(t, db, "internal", util.Ptr("10.96.0.0/16"))
		assert.ErrorContains(t, err, "which clusters use internally")
	})

	t.Run("clear of children's pools", func(t *testing.T) {
		db := setupControlPlaneAuthDB(t)
		_, err := controlPlaneWithPool(t, db, "self", true, poolJSON(t, "10.0.0.0/8"))
		require.NoError(t, err)
		_, err = controlPlaneWithPool(t, db, "child", false, poolJSON(t, "10.32.0.0/12"))
		require.NoError(t, err)

		_, err = runtimeWithNetworkCidr(t, db, "inside-the-child", util.Ptr("10.40.0.0/16"))
		assert.ErrorContains(t, err, "NetworkCidr 10.40.0.0/16 overlaps 10.32.0.0/12, in the network range pool of child control plane child")
		_, err = runtimeWithNetworkCidr(t, db, "holding-the-child", util.Ptr("10.0.0.0/10"))
		assert.ErrorContains(t, err, "overlaps 10.32.0.0/12")
		_, err = runtimeWithNetworkCidr(t, db, "in-this-control-planes-pool", util.Ptr("10.16.0.0/16"))
		require.NoError(t, err, "this control plane's own pool is where its runtimes' ranges come from")

		// set by an update, as an allocation is
		runtime, err := runtimeWithNetworkCidr(t, db, "unset", nil)
		require.NoError(t, err)
		var loaded KubernetesRuntimeInstance
		require.NoError(t, db.First(&loaded, *runtime.ID).Error)
		err = db.Model(&loaded).Updates(&KubernetesRuntimeInstance{NetworkCidr: util.Ptr("10.33.0.0/16")}).Error
		assert.ErrorContains(t, err, "in the network range pool of child control plane child")
	})

	t.Run("a deleted runtime's range is free again", func(t *testing.T) {
		db := setupControlPlaneAuthDB(t)
		first, err := runtimeWithNetworkCidr(t, db, "first", util.Ptr("10.16.0.0/16"))
		require.NoError(t, err)
		require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).Delete(first).Error)

		_, err = runtimeWithNetworkCidr(t, db, "second", util.Ptr("10.16.0.0/16"))
		assert.NoError(t, err)
	})

	t.Run("the unique index holds the line the check can't", func(t *testing.T) {
		db := setupControlPlaneAuthDB(t)
		_, err := runtimeWithNetworkCidr(t, db, "first", util.Ptr("10.16.0.0/16"))
		require.NoError(t, err)
		_, err = runtimeWithNetworkCidr(t, db.Session(&gorm.Session{SkipHooks: true}), "second", util.Ptr("10.16.0.0/16"))
		assert.ErrorContains(t, err, "UNIQUE constraint failed")
	})

	t.Run("set once, on either update shape", func(t *testing.T) {
		db := setupControlPlaneAuthDB(t)
		_, err := runtimeWithNetworkCidr(t, db, "other", util.Ptr("10.17.0.0/16"))
		require.NoError(t, err)
		created, err := runtimeWithNetworkCidr(t, db, "allocated-later", nil)
		require.NoError(t, err)
		load := func() *KubernetesRuntimeInstance {
			var loaded KubernetesRuntimeInstance
			require.NoError(t, db.First(&loaded, *created.ID).Error)
			return &loaded
		}

		// what the control plane does when it allocates a range
		err = db.Model(load()).Updates(&KubernetesRuntimeInstance{NetworkCidr: util.Ptr("10.17.0.0/16")}).Error
		assert.ErrorContains(t, err, "overlaps 10.17.0.0/16, the network range of kubernetes runtime instance other")
		require.NoError(t, db.Model(load()).Updates(&KubernetesRuntimeInstance{NetworkCidr: util.Ptr("10.16.0.0/16")}).Error)

		err = db.Model(load()).Updates(&KubernetesRuntimeInstance{NetworkCidr: util.Ptr("10.18.0.0/16")}).Error
		assert.ErrorContains(t, err, ErrMsgNetworkCidrImmutable)
		replaced := load()
		replaced.NetworkCidr = util.Ptr("10.18.0.0/16")
		assert.ErrorContains(t, db.Save(replaced).Error, ErrMsgNetworkCidrImmutable)
		cleared := load()
		cleared.NetworkCidr = nil
		assert.ErrorContains(t, db.Save(cleared).Error, ErrMsgNetworkCidrImmutable)

		require.NoError(t, db.Model(load()).Updates(&KubernetesRuntimeInstance{NetworkCidr: util.Ptr("10.16.0.0/16")}).Error,
			"sending the range it holds is not a change")
		require.NoError(t, db.Save(load()).Error, "nor is a replace that keeps it")
		require.NoError(t, db.Model(load()).Updates(&KubernetesRuntimeInstance{DefaultRuntime: util.Ptr(true)}).Error)
		assert.Equal(t, "10.16.0.0/16", *load().NetworkCidr)
	})

	t.Run("a patch is checked against the row it writes over", func(t *testing.T) {
		db := setupControlPlaneAuthDB(t)
		created, err := runtimeWithNetworkCidr(t, db, "allocated-twice", nil)
		require.NoError(t, err)

		// a patch's receiver is loaded before its transaction begins, and
		// another allocation commits in between
		var stale KubernetesRuntimeInstance
		require.NoError(t, db.First(&stale, *created.ID).Error)
		require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).Model(&KubernetesRuntimeInstance{}).
			Where("id = ?", *created.ID).Update("network_cidr", "10.16.0.0/16").Error)

		err = db.Model(&stale).Updates(&KubernetesRuntimeInstance{NetworkCidr: util.Ptr("10.18.0.0/16")}).Error
		assert.ErrorContains(t, err, ErrMsgNetworkCidrImmutable)
		var stored KubernetesRuntimeInstance
		require.NoError(t, db.First(&stored, *created.ID).Error)
		assert.Equal(t, "10.16.0.0/16", *stored.NetworkCidr)
	})
}

// controlPlaneWithPool creates a control plane instance with a pool,
// returning the create's error.
func controlPlaneWithPool(t *testing.T, db *gorm.DB, name string, self bool, pool *datatypes.JSON) (*ControlPlaneInstance, error) {
	t.Helper()
	instance := &ControlPlaneInstance{
		Instance:                    Instance{Name: util.Ptr(name)},
		Namespace:                   util.Ptr("threeport-control-plane"),
		IsSelf:                      util.Ptr(self),
		ControlPlaneDefinitionID:    controlPlaneDefinitionWithAuth(t, db, name+"-definition", true),
		KubernetesRuntimeInstanceID: runtimeOn(t, db, KubernetesRuntimeInfraProviderEKS, name+"-runtime"),
		NetworkCidrPool:             pool,
	}

	return instance, db.Create(instance).Error
}

// TestAChildsPoolIsClearOfItsParentsRanges covers NetworkCidrPool: a valid
// pool on any control plane, and on a child, the one row here this API does
// not serve, a pool clear of the ranges this control plane has allocated.
func TestAChildsPoolIsClearOfItsParentsRanges(t *testing.T) {
	t.Run("on create", func(t *testing.T) {
		db := setupControlPlaneAuthDB(t)
		_, err := runtimeWithNetworkCidr(t, db, "workload", util.Ptr("10.16.0.0/16"))
		require.NoError(t, err)

		_, err = controlPlaneWithPool(t, db, "self", true, poolJSON(t, "10.16.0.0/12"))
		require.NoError(t, err, "the pool this control plane allocates from holds its own ranges")
		_, err = controlPlaneWithPool(t, db, "child", false, poolJSON(t, "10.32.0.0/12"))
		require.NoError(t, err)
		_, err = controlPlaneWithPool(t, db, "child-without-a-pool", false, nil)
		require.NoError(t, err)

		_, err = controlPlaneWithPool(t, db, "overlapping-child", false, poolJSON(t, "10.48.0.0/12", "10.0.0.0/11"))
		assert.ErrorContains(t, err, "NetworkCidrPool entry 10.0.0.0/11 overlaps 10.16.0.0/16, the network range of kubernetes runtime instance workload")
		_, err = controlPlaneWithPool(t, db, "bad-pool", true, poolJSON(t, "10.16.0.0/20"))
		assert.ErrorContains(t, err, "smaller than a /16")
	})

	t.Run("on update", func(t *testing.T) {
		db := setupControlPlaneAuthDB(t)
		_, err := runtimeWithNetworkCidr(t, db, "workload", util.Ptr("10.16.0.0/16"))
		require.NoError(t, err)
		self, err := controlPlaneWithPool(t, db, "self", true, nil)
		require.NoError(t, err)
		child, err := controlPlaneWithPool(t, db, "child", false, nil)
		require.NoError(t, err)
		// this control plane manages the child
		require.NoError(t, db.Model(child).Updates(&ControlPlaneInstance{ParentControlPlaneInstanceID: self.ID}).Error)
		load := func(id *uint) *ControlPlaneInstance {
			var loaded ControlPlaneInstance
			require.NoError(t, db.First(&loaded, *id).Error)
			return &loaded
		}

		require.NoError(t, db.Model(load(self.ID)).Updates(&ControlPlaneInstance{NetworkCidrPool: poolJSON(t, "10.0.0.0/8")}).Error,
			"a change to this control plane's own pool affects only later allocations")
		err = db.Model(load(self.ID)).Updates(&ControlPlaneInstance{NetworkCidrPool: poolJSON(t, "10.0.0.0/8", "10.16.0.0/16")}).Error
		assert.ErrorContains(t, err, "overlap")

		err = db.Model(load(child.ID)).Updates(&ControlPlaneInstance{NetworkCidrPool: poolJSON(t, "10.16.0.0/12")}).Error
		assert.ErrorContains(t, err, "overlaps 10.16.0.0/16, the network range of kubernetes runtime instance workload")
		replaced := load(child.ID)
		replaced.NetworkCidrPool = poolJSON(t, "10.16.0.0/12")
		assert.ErrorContains(t, db.Save(replaced).Error, "overlaps 10.16.0.0/16")
		require.NoError(t, db.Model(load(child.ID)).Updates(&ControlPlaneInstance{NetworkCidrPool: poolJSON(t, "10.32.0.0/12")}).Error)

		// the child may have allocated from any block of its pool, which this
		// database doesn't hold, so the pool can only grow
		require.NoError(t, db.Model(load(child.ID)).Updates(&ControlPlaneInstance{NetworkCidrPool: poolJSON(t, "10.32.0.0/12", "10.64.0.0/12")}).Error,
			"a block added")
		require.NoError(t, db.Model(load(child.ID)).Updates(&ControlPlaneInstance{NetworkCidrPool: poolJSON(t, "10.32.0.0/11", "10.64.0.0/12")}).Error,
			"a block replaced by a larger one that holds it")
		assert.JSONEq(t, `["10.32.0.0/11", "10.64.0.0/12"]`, string(*load(child.ID).NetworkCidrPool))
		err = db.Model(load(child.ID)).Updates(&ControlPlaneInstance{NetworkCidrPool: poolJSON(t, "10.48.0.0/12", "10.64.0.0/12")}).Error
		assert.ErrorContains(t, err, "NetworkCidrPool must keep 10.32.0.0/11")
		err = db.Model(load(child.ID)).Updates(&ControlPlaneInstance{NetworkCidrPool: poolJSON(t, "10.32.0.0/12", "10.64.0.0/12")}).Error
		assert.ErrorContains(t, err, "NetworkCidrPool must keep 10.32.0.0/11", "a block replaced by a smaller one it held")
		replaced = load(child.ID)
		replaced.NetworkCidrPool = poolJSON(t, "10.64.0.0/12")
		assert.ErrorContains(t, db.Save(replaced).Error, "NetworkCidrPool must keep 10.32.0.0/11")
		cleared := load(child.ID)
		cleared.NetworkCidrPool = nil
		assert.ErrorContains(t, db.Save(cleared).Error, "NetworkCidrPool must keep 10.32.0.0/11", "clearing it removes every block")
		err = db.Model(load(child.ID)).Updates(&ControlPlaneInstance{NetworkCidrPool: poolJSON(t, "10.32.0.0/11", "10.64.0.0/12", "10.0.0.0/11")}).Error
		assert.ErrorContains(t, err, "overlaps 10.16.0.0/16", "a block added is checked as a first pool is")
		assert.JSONEq(t, `["10.32.0.0/11", "10.64.0.0/12"]`, string(*load(child.ID).NetworkCidrPool))

		// this control plane's own pool may shrink: the ranges it allocated
		// are its runtimes', which allocation skips whatever the pool
		require.NoError(t, db.Model(load(self.ID)).Updates(&ControlPlaneInstance{NetworkCidrPool: poolJSON(t, "10.128.0.0/9")}).Error)
	})

	t.Run("children's pools don't overlap", func(t *testing.T) {
		db := setupControlPlaneAuthDB(t)
		self, err := controlPlaneWithPool(t, db, "self", true, poolJSON(t, "10.0.0.0/8"))
		require.NoError(t, err, "this control plane's own pool may hold its children's")
		first, err := controlPlaneWithPool(t, db, "first-child", false, poolJSON(t, "10.32.0.0/12"))
		require.NoError(t, err)
		// this control plane manages the first child
		require.NoError(t, db.Model(first).Updates(&ControlPlaneInstance{ParentControlPlaneInstanceID: self.ID}).Error)

		_, err = controlPlaneWithPool(t, db, "overlapping-child", false, poolJSON(t, "10.64.0.0/12", "10.32.0.0/11"))
		assert.ErrorContains(t, err, "NetworkCidrPool entry 10.32.0.0/11 overlaps 10.32.0.0/12, in the network range pool of child control plane first-child")
		second, err := controlPlaneWithPool(t, db, "second-child", false, poolJSON(t, "10.48.0.0/12"))
		require.NoError(t, err)

		var loaded ControlPlaneInstance
		require.NoError(t, db.First(&loaded, *second.ID).Error)
		err = db.Model(&loaded).Updates(&ControlPlaneInstance{NetworkCidrPool: poolJSON(t, "10.48.0.0/12", "10.36.0.0/16")}).Error
		assert.ErrorContains(t, err, "overlaps 10.32.0.0/12, in the network range pool of child control plane first-child")
		var firstLoaded ControlPlaneInstance
		require.NoError(t, db.First(&firstLoaded, *first.ID).Error)
		require.NoError(t, db.Model(&firstLoaded).Updates(&ControlPlaneInstance{NetworkCidrPool: poolJSON(t, "10.32.0.0/12", "10.64.0.0/12")}).Error,
			"a child's pool isn't checked against itself")
	})

	t.Run("only the parent sets the pool of a row it manages", func(t *testing.T) {
		db := setupControlPlaneAuthDB(t)
		parent, err := controlPlaneWithPool(t, db, "parent", false, nil)
		require.NoError(t, err)
		owned, err := controlPlaneWithPool(t, db, "owned-self", true, poolJSON(t, "10.32.0.0/12"))
		require.NoError(t, err)
		require.NoError(t, db.Model(owned).Updates(&ControlPlaneInstance{ParentControlPlaneInstanceID: parent.ID}).Error)
		load := func(id *uint) *ControlPlaneInstance {
			var loaded ControlPlaneInstance
			require.NoError(t, db.First(&loaded, *id).Error)
			return &loaded
		}

		// the parent checked the pool against its own ranges, which this API
		// can't see
		err = db.Model(load(owned.ID)).Updates(&ControlPlaneInstance{NetworkCidrPool: poolJSON(t, "10.48.0.0/12")}).Error
		assert.ErrorContains(t, err, "cannot update NetworkCidrPool on a control plane its parent manages")
		owner := db.WithContext(lib.WithCaller(context.Background(), lib.CallerIdentity{CommonName: "threeport-owner"}))
		require.NoError(t, owner.Model(load(owned.ID)).Updates(&ControlPlaneInstance{NetworkCidrPool: poolJSON(t, "10.48.0.0/12")}).Error)

		// the parent's row here allocates nothing, so it carries no pool
		err = db.Model(load(parent.ID)).Updates(&ControlPlaneInstance{NetworkCidrPool: poolJSON(t, "10.64.0.0/12")}).Error
		assert.ErrorContains(t, err, "cannot update NetworkCidrPool on a control plane this API does not serve")
	})
}
