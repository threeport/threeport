package v0

import (
	"fmt"
	"net/netip"
	"strings"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	lib "github.com/threeport/threeport/pkg/api/lib/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// ErrMsgNetworkCidrImmutable is the rejection of an update that changes a
// kubernetes runtime instance's NetworkCidr once it is set.
const ErrMsgNetworkCidrImmutable = "NetworkCidr cannot be changed once set: a cluster's network can't be moved to another range"

// NetworkCidrPrefixLength is the size of the network range a control plane
// allocates to each Kubernetes runtime it provisions: a /16.  A range set on a
// runtime by hand may be larger.
const NetworkCidrPrefixLength = 16

// DefaultNetworkCidrPool is the pool a control plane allocates from when its
// ControlPlaneInstance sets no NetworkCidrPool: 10.16.0.0/16 through
// 10.255.0.0/16.  It starts at 10.16 to stay clear of 10.0.0.0/12, which holds
// every range that clusters provisioned before allocation use, so that new
// clusters can be peered with them.  The cluster-internal ranges inside it,
// 10.96.0.0/16 and 10.244.0.0/16, are skipped as in any pool, which leaves 238
// blocks.
var DefaultNetworkCidrPool = []string{
	"10.16.0.0/12",
	"10.32.0.0/11",
	"10.64.0.0/10",
	"10.128.0.0/9",
}

// ClusterInternalCidrs are the ranges a cluster uses only inside itself, the
// same on every cluster.  They are not routed outside it, but a cluster could
// not reach a peer whose network overlaps them, so no network range may.
var ClusterInternalCidrs = []string{
	// OKE services
	"10.96.0.0/16",
	// OKE pods
	"10.244.0.0/16",
	// EKS services
	"172.20.0.0/16",
	// GKE control plane's private endpoint
	"172.16.0.0/28",
}

// ParseNetworkCidr reads a network range for a Kubernetes runtime: an IPv4
// CIDR block, written as its network address, no smaller than a /16 and clear
// of ClusterInternalCidrs.
func ParseNetworkCidr(field, cidr string) (netip.Prefix, error) {
	prefix, err := parseNetworkBlock(field, cidr)
	if err != nil {
		return netip.Prefix{}, err
	}
	for _, internal := range clusterInternalPrefixes() {
		if prefix.Overlaps(internal) {
			return netip.Prefix{}, fmt.Errorf(
				"%s %s overlaps %s, which clusters use internally",
				field, cidr, internal,
			)
		}
	}

	return prefix, nil
}

// ParseNetworkCidrPool reads a NetworkCidrPool: a JSON array of IPv4 CIDR
// blocks, each no smaller than a /16, none overlapping another.  A pool may
// include cluster-internal ranges, which allocation skips.  An unset or empty
// pool holds no blocks.
func ParseNetworkCidrPool(raw *datatypes.JSON) ([]netip.Prefix, error) {
	cidrs, err := ParseCidrs("NetworkCidrPool", raw)
	if err != nil {
		return nil, err
	}

	pool := make([]netip.Prefix, 0, len(cidrs))
	for _, cidr := range cidrs {
		prefix, err := parseNetworkBlock("NetworkCidrPool entry", cidr)
		if err != nil {
			return nil, err
		}
		for _, earlier := range pool {
			if prefix.Overlaps(earlier) {
				return nil, fmt.Errorf("NetworkCidrPool entries %s and %s overlap", earlier, prefix)
			}
		}
		pool = append(pool, prefix)
	}

	return pool, nil
}

// NetworkCidrPoolOrDefault returns the pool a control plane allocates from:
// its NetworkCidrPool, or DefaultNetworkCidrPool when that is empty.
func NetworkCidrPoolOrDefault(raw *datatypes.JSON) ([]netip.Prefix, error) {
	pool, err := ParseNetworkCidrPool(raw)
	if err != nil {
		return nil, err
	}
	if len(pool) > 0 {
		return pool, nil
	}

	defaultPool := make([]netip.Prefix, len(DefaultNetworkCidrPool))
	for i, cidr := range DefaultNetworkCidrPool {
		defaultPool[i] = netip.MustParsePrefix(cidr)
	}

	return defaultPool, nil
}

// AllocateNetworkCidr returns the first /16 in pool, in the pool's order, that
// overlaps none of the ranges in use and none of ClusterInternalCidrs.  The
// ranges in use are the other runtimes' network ranges and the pools of the
// control plane's children, which allocate from those themselves.
func AllocateNetworkCidr(pool []netip.Prefix, inUse []netip.Prefix) (netip.Prefix, error) {
	unavailable := append(clusterInternalPrefixes(), inUse...)
	for _, entry := range pool {
		for block := netip.PrefixFrom(entry.Addr(), NetworkCidrPrefixLength); entry.Contains(block.Addr()); block = nextBlock(block) {
			if !overlapsAny(block, unavailable) {
				return block, nil
			}
			if block.Addr() == lastBlockAddr {
				break
			}
		}
	}

	entries := make([]string, len(pool))
	for i, entry := range pool {
		entries[i] = entry.String()
	}
	return netip.Prefix{}, fmt.Errorf(
		"no /%d is left to allocate in the network CIDR pool %s",
		NetworkCidrPrefixLength, strings.Join(entries, ", "),
	)
}

// lastBlockAddr is the address of the last /16 in the IPv4 space, after which
// there is no next block.
var lastBlockAddr = netip.MustParseAddr("255.255.0.0")

// nextBlock returns the /16 after block.
func nextBlock(block netip.Prefix) netip.Prefix {
	addr := block.Addr().As4()
	value := uint32(addr[0])<<24 | uint32(addr[1])<<16
	value += 1 << (32 - NetworkCidrPrefixLength)
	next := netip.AddrFrom4([4]byte{byte(value >> 24), byte(value >> 16), 0, 0})

	return netip.PrefixFrom(next, NetworkCidrPrefixLength)
}

// parseNetworkBlock reads an IPv4 CIDR block written as its network address
// and no smaller than a /16.
func parseNetworkBlock(field, cidr string) (netip.Prefix, error) {
	prefix, err := netip.ParsePrefix(cidr)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("%s %q is not a CIDR block: %w", field, cidr, err)
	}
	if !prefix.Addr().Is4() {
		return netip.Prefix{}, fmt.Errorf("%s %q is not an IPv4 CIDR block", field, cidr)
	}
	if prefix.Masked() != prefix {
		return netip.Prefix{}, fmt.Errorf(
			"%s %q has host bits set, write it as %q",
			field, cidr, prefix.Masked().String(),
		)
	}
	if prefix.Bits() > NetworkCidrPrefixLength {
		return netip.Prefix{}, fmt.Errorf(
			"%s %s is smaller than a /%d", field, cidr, NetworkCidrPrefixLength,
		)
	}

	return prefix, nil
}

// clusterInternalPrefixes returns ClusterInternalCidrs as prefixes.
func clusterInternalPrefixes() []netip.Prefix {
	prefixes := make([]netip.Prefix, len(ClusterInternalCidrs))
	for i, cidr := range ClusterInternalCidrs {
		prefixes[i] = netip.MustParsePrefix(cidr)
	}

	return prefixes
}

// overlapsAny reports whether prefix overlaps any of others.
func overlapsAny(prefix netip.Prefix, others []netip.Prefix) bool {
	for _, other := range others {
		if prefix.Overlaps(other) {
			return true
		}
	}

	return false
}

// runtimeNetworkCidr is the network range a kubernetes runtime instance
// holds.
type runtimeNetworkCidr struct {
	name   string
	prefix netip.Prefix
}

// runtimeNetworkCidrs returns the network ranges the kubernetes runtime
// instances hold, leaving out the one with excludeID.  It reads them in the
// write's transaction, so that a concurrent write that changes them conflicts
// with this one instead of slipping past the check.
func runtimeNetworkCidrs(tx *gorm.DB, excludeID *uint) ([]runtimeNetworkCidr, error) {
	query := lib.NewCleanSession(tx).Model(&KubernetesRuntimeInstance{}).Where("network_cidr IS NOT NULL")
	if excludeID != nil {
		query = query.Where("id <> ?", *excludeID)
	}
	var rows []KubernetesRuntimeInstance
	if err := query.Select("name", "network_cidr").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to read the kubernetes runtime instances' network ranges: %w", err)
	}

	ranges := make([]runtimeNetworkCidr, 0, len(rows))
	for _, row := range rows {
		if row.NetworkCidr == nil || *row.NetworkCidr == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(*row.NetworkCidr)
		if err != nil {
			return nil, fmt.Errorf(
				"kubernetes runtime instance %s holds network range %q, which is not a CIDR block: %w",
				util.DerefString(row.Name), *row.NetworkCidr, err,
			)
		}
		ranges = append(ranges, runtimeNetworkCidr{name: util.DerefString(row.Name), prefix: prefix})
	}

	return ranges, nil
}

// childNetworkCidrPool is the pool of a control plane this API does not
// serve, a child, which allocates ranges from it in its own database.
type childNetworkCidrPool struct {
	name string
	pool []netip.Prefix
}

// childNetworkCidrPools returns the pools of the control plane instances this
// API does not serve, leaving out the one with excludeID.  The rows a child
// keeps for its parent and for the genesis control plane carry no pool.  It
// reads them in the write's transaction, as runtimeNetworkCidrs does.
func childNetworkCidrPools(tx *gorm.DB, excludeID *uint) ([]childNetworkCidrPool, error) {
	query := lib.NewCleanSession(tx).Model(&ControlPlaneInstance{}).
		Where("network_cidr_pool IS NOT NULL").
		Where("is_self IS NULL OR is_self = ?", false)
	if excludeID != nil {
		query = query.Where("id <> ?", *excludeID)
	}
	var rows []ControlPlaneInstance
	if err := query.Select("name", "network_cidr_pool").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to read the child control planes' network range pools: %w", err)
	}

	pools := make([]childNetworkCidrPool, 0, len(rows))
	for _, row := range rows {
		pool, err := ParseNetworkCidrPool(row.NetworkCidrPool)
		if err != nil {
			return nil, fmt.Errorf(
				"control plane instance %s holds a network range pool that can't be read: %w",
				util.DerefString(row.Name), err,
			)
		}
		if len(pool) > 0 {
			pools = append(pools, childNetworkCidrPool{name: util.DerefString(row.Name), pool: pool})
		}
	}

	return pools, nil
}

// validateRuntimeNetworkCidr checks a kubernetes runtime instance's network
// range: a valid range, overlapping no other instance's and no child control
// plane's pool, from which the child may allocate the same range to a runtime
// this database doesn't hold.
func validateRuntimeNetworkCidr(tx *gorm.DB, networkCidr *string, id *uint) error {
	if networkCidr == nil {
		return nil
	}
	prefix, err := ParseNetworkCidr("NetworkCidr", *networkCidr)
	if err != nil {
		return util.NewBadRequestError(err.Error())
	}

	others, err := runtimeNetworkCidrs(tx, id)
	if err != nil {
		return err
	}
	for _, other := range others {
		if prefix.Overlaps(other.prefix) {
			return util.NewBadRequestError(fmt.Sprintf(
				"NetworkCidr %s overlaps %s, the network range of kubernetes runtime instance %s",
				prefix, other.prefix, other.name,
			))
		}
	}

	children, err := childNetworkCidrPools(tx, nil)
	if err != nil {
		return err
	}
	for _, child := range children {
		for _, entry := range child.pool {
			if prefix.Overlaps(entry) {
				return util.NewBadRequestError(fmt.Sprintf(
					"NetworkCidr %s overlaps %s, in the network range pool of child control plane %s",
					prefix, entry, child.name,
				))
			}
		}
	}

	return nil
}

// validateNetworkCidrPool checks a control plane instance's NetworkCidrPool.
// The pool of a control plane this API does not serve, a child, must also be
// clear of the network ranges this control plane has allocated and of the
// other children's pools, so that no two control planes hand out the same
// range.  id is the instance's, nil for one being created.
func validateNetworkCidrPool(tx *gorm.DB, raw *datatypes.JSON, self bool, id *uint) error {
	pool, err := ParseNetworkCidrPool(raw)
	if err != nil {
		return util.NewBadRequestError(err.Error())
	}
	if self || len(pool) == 0 {
		return nil
	}

	allocated, err := runtimeNetworkCidrs(tx, nil)
	if err != nil {
		return err
	}
	for _, entry := range pool {
		for _, runtime := range allocated {
			if entry.Overlaps(runtime.prefix) {
				return util.NewBadRequestError(fmt.Sprintf(
					"NetworkCidrPool entry %s overlaps %s, the network range of kubernetes runtime instance %s",
					entry, runtime.prefix, runtime.name,
				))
			}
		}
	}

	siblings, err := childNetworkCidrPools(tx, id)
	if err != nil {
		return err
	}
	for _, entry := range pool {
		for _, sibling := range siblings {
			for _, other := range sibling.pool {
				if entry.Overlaps(other) {
					return util.NewBadRequestError(fmt.Sprintf(
						"NetworkCidrPool entry %s overlaps %s, in the network range pool of child control plane %s",
						entry, other, sibling.name,
					))
				}
			}
		}
	}

	return nil
}

// validateNetworkCidrPoolGrows checks a change to a child's pool keeps every
// block it held: the child may have allocated ranges from any of them, which
// this database doesn't hold, so a block can be added, or replaced by a
// larger one that holds it, but not removed.
func validateNetworkCidrPoolGrows(stored, incoming *datatypes.JSON) error {
	old, err := ParseNetworkCidrPool(stored)
	if err != nil {
		return err
	}
	pool, err := ParseNetworkCidrPool(incoming)
	if err != nil {
		return util.NewBadRequestError(err.Error())
	}
	for _, block := range old {
		held := false
		for _, entry := range pool {
			if entry.Bits() <= block.Bits() && entry.Contains(block.Addr()) {
				held = true
				break
			}
		}
		if !held {
			return util.NewBadRequestError(fmt.Sprintf(
				"NetworkCidrPool must keep %s: a child control plane's pool can only grow, since the child may have allocated ranges from it",
				block,
			))
		}
	}

	return nil
}
