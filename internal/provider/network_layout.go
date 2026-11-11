package provider

import (
	"encoding/json"
	"fmt"
	"net/netip"

	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// A cluster's network is laid out in the block allocated to it, the
// KubernetesRuntimeInstance's NetworkCidr: every range is an offset into the
// block's first /16, so the same layout works for any block.  A cluster with
// no block, as one created before blocks were allocated has, keeps the ranges
// every cluster used to get, so its stack plans no change to its network.

// eksServiceCidr is the range an EKS cluster's services get addresses from.
// It is used only inside the cluster, so every cluster has the same one, set
// explicitly so it stays outside the pool blocks are allocated from.
const eksServiceCidr = "172.20.0.0/16"

// eksNetworkLayout is the network of an EKS cluster: its VPC, and a public and
// a private subnet in each of up to three zones.
type eksNetworkLayout struct {
	Vpc            string
	PublicSubnets  []string
	PrivateSubnets []string

	// The service range, "" for a cluster created before it was set, whose
	// range EKS chose.
	ServiceCidr string
}

// gkeNetworkLayout is the network of a GKE cluster: its nodes' subnet and the
// secondary ranges of its pods and services.
type gkeNetworkLayout struct {
	Nodes    string
	Pods     string
	Services string
}

// okeNetworkLayout is the network of an OKE cluster: its VCN and the subnets of
// its API endpoint, nodes and load balancers.  Its pods and services use the
// cluster-internal ranges, which the pool leaves out.
type okeNetworkLayout struct {
	Vcn          string
	Public       string
	Private      string
	LoadBalancer string
}

// eksLayout returns the EKS network for a block, or the one every cluster
// used to get when block is "".  With a block, each zone's private subnet,
// where the nodes and, through the VPC CNI, the pods get addresses, is a /18,
// and its public subnet, for NAT gateways and load balancers, a /22.
func eksLayout(block string) (eksNetworkLayout, error) {
	if block == "" {
		return eksNetworkLayout{
			Vpc:            "10.0.0.0/16",
			PublicSubnets:  []string{"10.0.0.0/22", "10.0.8.0/22", "10.0.16.0/22"},
			PrivateSubnets: []string{"10.0.4.0/22", "10.0.12.0/22", "10.0.20.0/22"},
		}, nil
	}

	base, err := parseNetworkBase(block)
	if err != nil {
		return eksNetworkLayout{}, err
	}

	return eksNetworkLayout{
		Vpc:            block,
		PrivateSubnets: []string{base.subnet(0, 18), base.subnet(64, 18), base.subnet(128, 18)},
		PublicSubnets:  []string{base.subnet(192, 22), base.subnet(196, 22), base.subnet(200, 22)},
		ServiceCidr:    eksServiceCidr,
	}, nil
}

// gkeLayout returns the GKE network for a block, or the one every cluster
// used to get when block is "".  With a block, the pods' /17 holds a /24 for
// each of 128 nodes at 110 pods per node, GKE's default.
func gkeLayout(block string) (gkeNetworkLayout, error) {
	if block == "" {
		return gkeNetworkLayout{Nodes: "10.0.0.0/16", Pods: "10.1.0.0/16", Services: "10.2.0.0/20"}, nil
	}

	base, err := parseNetworkBase(block)
	if err != nil {
		return gkeNetworkLayout{}, err
	}

	return gkeNetworkLayout{
		Pods:     base.subnet(0, 17),
		Nodes:    base.subnet(128, 20),
		Services: base.subnet(144, 20),
	}, nil
}

// okeLayout returns the OKE network for a block, or the one every cluster
// used to get when block is "".  With a block, the subnets sit where they
// always have, relative to the block.
func okeLayout(block string) (okeNetworkLayout, error) {
	if block == "" {
		return okeNetworkLayout{
			Vcn:          "10.0.0.0/16",
			Public:       "10.0.0.0/28",
			Private:      "10.0.10.0/24",
			LoadBalancer: "10.0.20.0/24",
		}, nil
	}

	base, err := parseNetworkBase(block)
	if err != nil {
		return okeNetworkLayout{}, err
	}

	return okeNetworkLayout{
		Vcn:          block,
		Public:       base.subnet(0, 28),
		Private:      base.subnet(10, 24),
		LoadBalancer: base.subnet(20, 24),
	}, nil
}

// networkBase is the first /16 of a cluster's block, which its ranges are
// offsets into.
type networkBase [2]byte

// parseNetworkBase checks a cluster's block, as the API does, and returns its
// first /16.
func parseNetworkBase(block string) (networkBase, error) {
	prefix, err := v0.ParseNetworkCidr("NetworkCidr", block)
	if err != nil {
		return networkBase{}, fmt.Errorf("failed to lay out the cluster's network: %w", err)
	}
	address := prefix.Addr().As4()

	return networkBase{address[0], address[1]}, nil
}

// subnet returns the range of the given size at third octet offset in the
// base /16.
func (b networkBase) subnet(offset byte, bits int) string {
	return netip.PrefixFrom(netip.AddrFrom4([4]byte{b[0], b[1], offset, 0}), bits).String()
}

// resourceInputInStack returns an input of the first resource of a type an
// exported deployment holds, and whether it holds one.
func resourceInputInStack(deployment json.RawMessage, resourceType, input string) (string, bool) {
	var contents struct {
		Resources []struct {
			Type   string         `json:"type"`
			Inputs map[string]any `json:"inputs"`
		} `json:"resources"`
	}
	if len(deployment) == 0 || json.Unmarshal(deployment, &contents) != nil {
		return "", false
	}
	for _, resource := range contents.Resources {
		if resource.Type == resourceType {
			value, _ := resource.Inputs[input].(string)
			return value, true
		}
	}

	return "", false
}

// A create resumed on a stack that already holds the cluster's network keeps
// the range it was laid out in, rather than laying it out again in the one
// asked for, which would replace it: the stack may predate ranges being
// allocated, or have been created with another.

// keepStackNetwork sets NetworkCidr to the range of the VPC the stack holds,
// if it holds one: "" for one laid out before ranges were allocated.
func (i *KubernetesRuntimeInfraEKS) keepStackNetwork(deployment json.RawMessage) {
	vpc, held := resourceInputInStack(deployment, "aws:ec2/vpc:Vpc", "cidrBlock")
	if !held {
		return
	}
	legacy, _ := eksLayout("")
	if vpc == legacy.Vpc {
		vpc = ""
	}
	i.NetworkCidr = vpc
}

// keepStackNetwork sets NetworkCidr to the range the subnet the stack holds
// was laid out in, if it holds one: "" for one laid out before ranges were
// allocated.
func (i *KubernetesRuntimeInfraGKE) keepStackNetwork(deployment json.RawMessage) {
	nodes, held := resourceInputInStack(deployment, "gcp:compute/subnetwork:Subnetwork", "ipCidrRange")
	if !held {
		return
	}
	legacy, _ := gkeLayout("")
	if nodes == legacy.Nodes {
		i.NetworkCidr = ""
		return
	}
	prefix, err := netip.ParsePrefix(nodes)
	if err != nil {
		return
	}
	address := prefix.Addr().As4()
	i.NetworkCidr = netip.PrefixFrom(netip.AddrFrom4([4]byte{address[0], address[1], 0, 0}), v0.NetworkCidrPrefixLength).String()
}

// okeStackNetwork returns the range of the VCN a stack holds, and whether it
// holds one: "" for one laid out before ranges were allocated.
func okeStackNetwork(deployment json.RawMessage) (string, bool) {
	vcn, held := resourceInputInStack(deployment, "oci:Core/vcn:Vcn", "cidrBlock")
	if !held {
		return "", false
	}
	legacy, _ := okeLayout("")
	if vcn == legacy.Vcn {
		return "", true
	}

	return vcn, true
}
