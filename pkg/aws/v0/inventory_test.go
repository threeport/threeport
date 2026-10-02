package v0

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestLeakedResourcesReportsOnlyAdditions checks the comparison names what an
// install added and left behind, and stays quiet about everything else.
//
// This is what stands between a teardown that reports success and a NAT
// gateway billing by the hour that nobody notices.
func TestLeakedResourcesReportsOnlyAdditions(t *testing.T) {
	before := AwsResourceInventory{
		"vpc":          {"vpc-pre-existing"},
		"iam-role":     {"/SomeoneElsesRole"},
		"nat-gateway":  {},
		"elastic-ip":   {"eipalloc-pre-existing"},
		"eks-cluster":  {},
		"ec2-instance": {"i-pre-existing"},
	}

	t.Run("a clean teardown leaks nothing", func(t *testing.T) {
		assert.Empty(t, LeakedResources(before, before))
	})

	t.Run("resources left behind are named", func(t *testing.T) {
		after := AwsResourceInventory{
			"vpc":          {"vpc-pre-existing", "vpc-left-behind"},
			"iam-role":     {"/SomeoneElsesRole", "/cluster-role-test"},
			"nat-gateway":  {"nat-left-behind"},
			"elastic-ip":   {"eipalloc-pre-existing"},
			"eks-cluster":  {},
			"ec2-instance": {"i-pre-existing"},
		}

		assert.Equal(t, []string{
			"iam-role /cluster-role-test",
			"nat-gateway nat-left-behind",
			"vpc vpc-left-behind",
		}, LeakedResources(before, after))
	})

	t.Run("a resource type absent beforehand still reports", func(t *testing.T) {
		// a load balancer type the account had none of is the case most
		// likely to be missed, since there is no prior entry to compare to
		after := AwsResourceInventory{
			"load-balancer": {"arn:aws:elasticloadbalancing:::loadbalancer/app/left-behind"},
		}

		assert.Equal(t, []string{
			"load-balancer arn:aws:elasticloadbalancing:::loadbalancer/app/left-behind",
		}, LeakedResources(before, after))
	})

	t.Run("resources removed by someone else are not reported", func(t *testing.T) {
		// the comparison is about what the install added, not about keeping
		// the account frozen
		after := AwsResourceInventory{
			"vpc":          {},
			"iam-role":     {"/SomeoneElsesRole"},
			"ec2-instance": {"i-pre-existing"},
		}

		assert.Empty(t, LeakedResources(before, after))
	})
}
