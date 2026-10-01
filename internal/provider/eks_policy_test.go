package provider

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resourceManagerPolicyAllows reports whether the resource manager policy
// grants an action, honoring the trailing wildcards it uses for read-only
// actions.
func resourceManagerPolicyAllows(t *testing.T, action string) bool {
	t.Helper()

	var policy struct {
		Statement []struct {
			Action any `json:"Action"`
		} `json:"Statement"`
	}
	require.NoError(
		t,
		json.Unmarshal([]byte(ResourceManagerPolicyDocument), &policy),
		"the resource manager policy is not valid JSON",
	)

	for _, statement := range policy.Statement {
		var actions []string
		switch typedAction := statement.Action.(type) {
		case string:
			actions = []string{typedAction}
		case []any:
			for _, a := range typedAction {
				actions = append(actions, a.(string))
			}
		}
		for _, granted := range actions {
			if granted == action {
				return true
			}
			if strings.HasSuffix(granted, "*") &&
				strings.HasPrefix(action, strings.TrimSuffix(granted, "*")) {
				return true
			}
		}
	}

	return false
}

// TestResourceManagerPolicyCoversProvisioning checks the role threeport
// assumes to provision a cluster can make every call provisioning needs.
//
// A missing permission is not caught by anything else: the code compiles, the
// unit tests pass, and the failure only appears partway through a real
// provisioning run as an UnauthorizedOperation, with infrastructure already
// half created.  Dropping ec2:DescribeAddressesAttribute - which the Pulumi
// AWS provider reads back after creating an elastic IP, and aws-builder never
// called - did exactly that.
func TestResourceManagerPolicyCoversProvisioning(t *testing.T) {
	// the IAM, EKS and STS calls the bootstrap in aws_bootstrap.go makes
	bootstrapCalls := []string{
		"iam:CreateRole",
		"iam:GetRole",
		"iam:AttachRolePolicy",
		"iam:ListAttachedRolePolicies",
		"iam:DetachRolePolicy",
		"iam:DeleteRole",
		"iam:CreatePolicy",
		"iam:ListPolicyVersions",
		"iam:DeletePolicyVersion",
		"iam:DeletePolicy",
		"iam:CreateOpenIDConnectProvider",
		"iam:ListOpenIDConnectProviders",
		"iam:GetOpenIDConnectProvider",
		"iam:DeleteOpenIDConnectProvider",
		// tagging is a separate action from creating, for every resource type
		// that carries tags.  The OIDC provider is tagged so that teardown can
		// find it without the cluster, which is the only thing that knows its
		// issuer URL.
		"iam:TagOpenIDConnectProvider",
		"iam:TagRole",
		"iam:TagPolicy",
		"eks:TagResource",
		"ec2:CreateTags",
		"iam:PassRole",
		"eks:CreateAddon",
		"eks:DescribeCluster",
	}

	// what the Pulumi AWS provider calls for the resources the program
	// declares, including the attribute read-backs it does after a create
	pulumiProviderCalls := []string{
		"ec2:CreateVpc",
		"ec2:DescribeVpcs",
		"ec2:DescribeVpcAttribute",
		"ec2:CreateSubnet",
		"ec2:DescribeSubnets",
		"ec2:AllocateAddress",
		"ec2:DescribeAddresses",
		"ec2:DescribeAddressesAttribute",
		"ec2:CreateNatGateway",
		"ec2:DescribeNatGateways",
		"ec2:CreateRouteTable",
		"ec2:DescribeRouteTables",
		"ec2:CreateInternetGateway",
		"ec2:DescribeInternetGateways",
		"ec2:DescribeAvailabilityZones",
		"ec2:DescribeSecurityGroups",
		"ec2:DescribeNetworkInterfaces",
		"ec2:CreateTags",
		"ec2:DescribeTags",
		"eks:CreateCluster",
		"eks:CreateNodegroup",
		"eks:DescribeNodegroup",
		"eks:ListNodegroups",
		"eks:DescribeAddon",
		"eks:ListTagsForResource",
	}

	for _, action := range append(bootstrapCalls, pulumiProviderCalls...) {
		assert.True(
			t,
			resourceManagerPolicyAllows(t, action),
			"the resource manager policy does not grant %s", action,
		)
	}
}

// TestResourceManagerPolicyWildcardsAreReadOnly checks the policy only
// wildcards actions that read.  A wildcard over writes would hand the role
// more than provisioning needs.
func TestResourceManagerPolicyWildcardsAreReadOnly(t *testing.T) {
	var policy struct {
		Statement []struct {
			Sid    string `json:"Sid"`
			Action any    `json:"Action"`
		} `json:"Statement"`
	}
	require.NoError(t, json.Unmarshal([]byte(ResourceManagerPolicyDocument), &policy))

	// S3 is granted wholesale for workload storage, which predates this and
	// is not about cluster provisioning
	readOnlyPrefixes := []string{"Describe", "List", "Get"}

	for _, statement := range policy.Statement {
		if statement.Sid == "S3Permissions" {
			continue
		}
		actions, ok := statement.Action.([]any)
		if !ok {
			continue
		}
		for _, rawAction := range actions {
			action := rawAction.(string)
			if !strings.HasSuffix(action, "*") {
				continue
			}
			operation := strings.TrimSuffix(strings.SplitN(action, ":", 2)[1], "*")
			readOnly := false
			for _, prefix := range readOnlyPrefixes {
				if strings.HasPrefix(operation, prefix) {
					readOnly = true
				}
			}
			assert.True(t, readOnly, "%s wildcards something other than a read", action)
		}
	}
}
