package provider

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awseks "github.com/aws/aws-sdk-go-v2/service/eks"
	ekstypes "github.com/aws/aws-sdk-go-v2/service/eks/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeEbsStorageAddon is an EBS CSI driver add-on that reports the statuses
// given, one per describe, and records the version it is updated to.
type fakeEbsStorageAddon struct {
	mu        sync.Mutex
	statuses  []ekstypes.AddonStatus
	version   string
	describes int
	updatedTo string
}

func (f *fakeEbsStorageAddon) DescribeAddon(context.Context, *awseks.DescribeAddonInput, ...func(*awseks.Options)) (*awseks.DescribeAddonOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	status := f.statuses[min(f.describes, len(f.statuses)-1)]
	f.describes++

	version := f.version
	if version == "" {
		version = "v1.0.0-eksbuild.1"
	}

	return &awseks.DescribeAddonOutput{Addon: &ekstypes.Addon{Status: status, AddonVersion: aws.String(version)}}, nil
}

func (f *fakeEbsStorageAddon) DescribeAddonVersions(context.Context, *awseks.DescribeAddonVersionsInput, ...func(*awseks.Options)) (*awseks.DescribeAddonVersionsOutput, error) {
	return &awseks.DescribeAddonVersionsOutput{Addons: []ekstypes.AddonInfo{{AddonVersions: []ekstypes.AddonVersionInfo{{
		AddonVersion:    aws.String("v2.0.0-eksbuild.1"),
		Compatibilities: []ekstypes.Compatibility{{ClusterVersion: aws.String("1.36"), DefaultVersion: true}},
	}}}}}, nil
}

func (f *fakeEbsStorageAddon) UpdateAddon(ctx context.Context, input *awseks.UpdateAddonInput, _ ...func(*awseks.Options)) (*awseks.UpdateAddonOutput, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updatedTo = aws.ToString(input.AddonVersion)

	return &awseks.UpdateAddonOutput{}, nil
}

// TestTheEbsAddonUpgradeWaitsBeforeAndAfterItsUpdate covers an upgrade of the
// EBS CSI driver add-on while it is still updating: it waits for the add-on
// to be active, updates it to the version EKS defaults to for the cluster's,
// and waits for it to be active again.  Each wait makes its own deadline of
// ebsStorageAddonUpgradeTimeout, and the update its own request deadline,
// rather than share the lookups', so the time a wait takes doesn't come out
// of what comes after it.
func TestTheEbsAddonUpgradeWaitsBeforeAndAfterItsUpdate(t *testing.T) {
	timeout, request, interval, newClient := ebsStorageAddonUpgradeTimeout, ebsStorageAddonRequestTimeout, ebsStorageAddonPollInterval, newEbsStorageAddonClient
	t.Cleanup(func() {
		ebsStorageAddonUpgradeTimeout, ebsStorageAddonRequestTimeout, ebsStorageAddonPollInterval, newEbsStorageAddonClient = timeout, request, interval, newClient
	})
	ebsStorageAddonPollInterval = time.Second
	ebsStorageAddonUpgradeTimeout = 10 * time.Second
	// the wait before the update outlasts the lookups' deadline, which the
	// update doesn't share
	ebsStorageAddonRequestTimeout = 200 * time.Millisecond
	addon := &fakeEbsStorageAddon{statuses: []ekstypes.AddonStatus{
		ekstypes.AddonStatusUpdating,                             // the upgrade's first look
		ekstypes.AddonStatusUpdating, ekstypes.AddonStatusActive, // the wait before the update
		ekstypes.AddonStatusUpdating, ekstypes.AddonStatusActive, // the wait after it
	}}
	newEbsStorageAddonClient = func(aws.Config) ebsStorageAddonAPI { return addon }

	require.NoError(t, testEksInfra(2).upgradeEbsStorageAddon("1.36"))
	assert.Equal(t, "v2.0.0-eksbuild.1", addon.updatedTo)
	assert.Equal(t, 5, addon.describes, "both waits ran until the add-on was active")

	// an add-on already on the version isn't updated
	current := &fakeEbsStorageAddon{statuses: []ekstypes.AddonStatus{ekstypes.AddonStatusActive}, version: "v2.0.0-eksbuild.1"}
	newEbsStorageAddonClient = func(aws.Config) ebsStorageAddonAPI { return current }
	require.NoError(t, testEksInfra(2).upgradeEbsStorageAddon("1.36"))
	assert.Empty(t, current.updatedTo)
	assert.Equal(t, 1, current.describes)
}
