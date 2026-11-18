package v0

import (
	"fmt"
)

// The network a cluster is laid out in caps how many nodes it can run.  A
// definition whose maximum the network can't hold is refused, since its
// cluster would fail to place pods, or nodes, well short of the maximum.
//
// The capacities are those of the networks new clusters are laid out in
// from a network range (see internal/provider/network_layout.go).  A
// cluster created before ranges were allocated, or by a child control plane
// given no pool, keeps the smaller network every cluster used to get.
const (
	// eksPrivateSubnetAddresses is the addresses an EKS cluster's private
	// subnet in each zone gives its nodes and pods: a /18, less the five
	// AWS reserves in every subnet.
	eksPrivateSubnetAddresses = 1<<(32-18) - 5

	// gkeMaxNodes is the most nodes a GKE cluster's pod range holds.  GKE
	// gives each node a block of the range twice its pods per node: a /25 of
	// the /17 at 64 pods per node, or a /24 of the /16 clusters had before at
	// 110, 256 nodes either way.
	gkeMaxNodes = 256

	// okeMaxNodes is the most nodes an OKE cluster's node subnet holds: a
	// /24, less the three addresses OCI reserves in every subnet.  The pods
	// run on an overlay network and take none of its addresses.
	okeMaxNodes = 1<<(32-24) - 3

	// eksMaxPodsSmall and eksMaxPodsLarge are the most pods an EKS managed
	// node group runs on a node, by its vCPUs, below and from
	// eksMaxPodsLargeVcpus.
	eksMaxPodsSmall      = 110
	eksMaxPodsLarge      = 250
	eksMaxPodsLargeVcpus = 30
)

// EksDefaultZoneCount and EksMaxZoneCount are the zones an EKS cluster's
// nodes run in when its definition gives no ZoneCount, and the most they run
// in: a ZoneCount above it gets as many.  The provider lays the cluster out
// in them (internal/provider/eks_pulumi.go), and a definition's maximum is
// checked against the network they make, so both use these.
const (
	EksDefaultZoneCount = 2
	EksMaxZoneCount     = 3
)

// eksInstanceNetwork is an instance type's network limits.
type eksInstanceNetwork struct {
	enis            int
	addressesPerEni int
	vcpus           int
}

// eksInstanceNetworks are the network limits of the instance types Threeport
// maps node sizes and profiles to (pkg/mapping/v0/node.go), as EC2's
// DescribeInstanceTypes reports them.  An instance type that isn't listed
// isn't checked.
var eksInstanceNetworks = map[string]eksInstanceNetwork{
	"c8g.2xlarge":  {enis: 4, addressesPerEni: 15, vcpus: 8},
	"c8g.4xlarge":  {enis: 8, addressesPerEni: 30, vcpus: 16},
	"c8g.8xlarge":  {enis: 8, addressesPerEni: 30, vcpus: 32},
	"c8g.12xlarge": {enis: 8, addressesPerEni: 30, vcpus: 48},
	"c8g.16xlarge": {enis: 15, addressesPerEni: 50, vcpus: 64},
	"c8g.24xlarge": {enis: 15, addressesPerEni: 50, vcpus: 96},
	"c8g.48xlarge": {enis: 15, addressesPerEni: 50, vcpus: 192},
	"c8g.large":    {enis: 3, addressesPerEni: 10, vcpus: 2},
	"c8g.medium":   {enis: 2, addressesPerEni: 4, vcpus: 1},
	"c8g.xlarge":   {enis: 4, addressesPerEni: 15, vcpus: 4},
	"m8i.2xlarge":  {enis: 4, addressesPerEni: 30, vcpus: 8},
	"m8i.4xlarge":  {enis: 8, addressesPerEni: 50, vcpus: 16},
	"m8i.8xlarge":  {enis: 10, addressesPerEni: 50, vcpus: 32},
	"m8i.12xlarge": {enis: 12, addressesPerEni: 50, vcpus: 48},
	"m8i.16xlarge": {enis: 16, addressesPerEni: 64, vcpus: 64},
	"m8i.24xlarge": {enis: 16, addressesPerEni: 64, vcpus: 96},
	"m8i.32xlarge": {enis: 24, addressesPerEni: 64, vcpus: 128},
	"m8i.48xlarge": {enis: 24, addressesPerEni: 64, vcpus: 192},
	"m8i.large":    {enis: 3, addressesPerEni: 20, vcpus: 2},
	"m8i.xlarge":   {enis: 4, addressesPerEni: 30, vcpus: 4},
	"r8g.2xlarge":  {enis: 4, addressesPerEni: 15, vcpus: 8},
	"r8g.4xlarge":  {enis: 8, addressesPerEni: 30, vcpus: 16},
	"r8g.8xlarge":  {enis: 8, addressesPerEni: 30, vcpus: 32},
	"r8g.12xlarge": {enis: 8, addressesPerEni: 30, vcpus: 48},
	"r8g.16xlarge": {enis: 15, addressesPerEni: 50, vcpus: 64},
	"r8g.24xlarge": {enis: 15, addressesPerEni: 50, vcpus: 96},
	"r8g.48xlarge": {enis: 15, addressesPerEni: 50, vcpus: 192},
	"r8g.large":    {enis: 3, addressesPerEni: 10, vcpus: 2},
	"r8g.xlarge":   {enis: 4, addressesPerEni: 15, vcpus: 4},
	"t3.medium":    {enis: 3, addressesPerEni: 6, vcpus: 2},
	"t3.micro":     {enis: 2, addressesPerEni: 2, vcpus: 2},
	"t3.nano":      {enis: 2, addressesPerEni: 2, vcpus: 2},
	"t3.small":     {enis: 3, addressesPerEni: 4, vcpus: 2}}

// eksAddressesPerNode returns how many of its subnet's addresses a node of
// an instance type takes at most, and whether the instance type is known.
// The VPC CNI gives each pod an address of the node's subnet, and keeps the
// addresses of one network interface beyond what its pods use, up to every
// address the instance's interfaces hold.  The pods are capped as an EKS
// managed node group caps them: by the instance's interfaces, and at 110 on
// fewer than 30 vCPUs and 250 on more.
func eksAddressesPerNode(instanceType string) (int, bool) {
	network, known := eksInstanceNetworks[instanceType]
	if !known {
		return 0, false
	}

	maxPods := network.enis*(network.addressesPerEni-1) + 2
	podCap := eksMaxPodsSmall
	if network.vcpus >= eksMaxPodsLargeVcpus {
		podCap = eksMaxPodsLarge
	}
	maxPods = min(maxPods, podCap)

	return min(network.enis*network.addressesPerEni, maxPods+network.addressesPerEni), true
}

// eksNodesPerZone returns how many nodes of an instance type an EKS cluster's
// private subnet in a zone holds, and whether the instance type is known.
func eksNodesPerZone(instanceType string) (int, bool) {
	addresses, known := eksAddressesPerNode(instanceType)
	if !known {
		return 0, false
	}

	return eksPrivateSubnetAddresses / addresses, true
}

// eksNetworkNodeLimit returns the largest maximum the network of an EKS
// cluster holds, and the reason, or 0 when its instance type isn't known.
// A node group per zone holds the maximum in each zone; one node group
// spanning the zones keeps them balanced, so its nodes are split across
// them: the zones the provider lays the cluster out in, which a ZoneCount
// above EksMaxZoneCount doesn't add to.
func eksNetworkNodeLimit(instanceType string, zones int, nodeGroupPerZone bool) (int, string) {
	perZone, known := eksNodesPerZone(instanceType)
	if !known {
		return 0, ""
	}
	if zones < 1 {
		zones = EksDefaultZoneCount
	}
	zones = min(zones, EksMaxZoneCount)
	addresses, _ := eksAddressesPerNode(instanceType)
	reason := fmt.Sprintf(
		"each zone's /18 private subnet has %d addresses for nodes and pods, and a %s node takes up to %d of them, so it holds %d nodes",
		eksPrivateSubnetAddresses, instanceType, addresses, perZone,
	)
	if nodeGroupPerZone {
		return perZone, reason + ", the most each zone's node group can have"
	}

	return perZone * zones, fmt.Sprintf("%s, %d across the %d zones", reason, perZone*zones, zones)
}

// gkeNetworkNodeLimit returns the largest maximum a GKE cluster's network
// holds, and the reason.  A definition from before zone counts limits each
// zone's nodes rather than the cluster's, and isn't checked.
func gkeNetworkNodeLimit(zones int) (int, string) {
	if zones < 1 {
		return 0, ""
	}

	return gkeMaxNodes, fmt.Sprintf("the cluster's pod range holds %d nodes", gkeMaxNodes)
}

// okeNetworkNodeLimit returns the largest maximum an OKE cluster's network
// holds, and the reason.
func okeNetworkNodeLimit() (int, string) {
	return okeMaxNodes, fmt.Sprintf("the cluster's /24 node subnet holds %d nodes", okeMaxNodes)
}
