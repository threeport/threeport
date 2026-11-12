package provider

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testEksCluster   = "threeport-test"
	testEksNamespace = "threeport-control-plane"
)

// fakeEbsApi returns pages of volumes and records the deletes it is asked
// for, failing the ones named in failDeletes.
type fakeEbsApi struct {
	pages       [][]ec2types.Volume
	inputs      []*ec2.DescribeVolumesInput
	deleted     []string
	failDeletes map[string]error
}

func (f *fakeEbsApi) DescribeVolumes(_ context.Context, input *ec2.DescribeVolumesInput, _ ...func(*ec2.Options)) (*ec2.DescribeVolumesOutput, error) {
	f.inputs = append(f.inputs, input)
	page := len(f.inputs) - 1
	output := &ec2.DescribeVolumesOutput{Volumes: f.pages[page]}
	if page < len(f.pages)-1 {
		output.NextToken = aws.String("next")
	}

	return output, nil
}

func (f *fakeEbsApi) DeleteVolume(_ context.Context, input *ec2.DeleteVolumeInput, _ ...func(*ec2.Options)) (*ec2.DeleteVolumeOutput, error) {
	if err, ok := f.failDeletes[aws.ToString(input.VolumeId)]; ok {
		return nil, err
	}
	f.deleted = append(f.deleted, aws.ToString(input.VolumeId))

	return &ec2.DeleteVolumeOutput{}, nil
}

// testEbsVolume returns a volume with the CSI driver's tags for cluster and
// namespace.
func testEbsVolume(id, cluster, namespace string, state ec2types.VolumeState) ec2types.Volume {
	return ec2types.Volume{
		VolumeId: aws.String(id),
		State:    state,
		Tags: []ec2types.Tag{
			{Key: aws.String("kubernetes.io/cluster/" + cluster), Value: aws.String("owned")},
			{Key: aws.String("kubernetes.io/created-for/pvc/namespace"), Value: aws.String(namespace)},
			{Key: aws.String("kubernetes.io/created-for/pvc/name"), Value: aws.String("datadir-" + id)},
		},
	}
}

// TestTheLeftoverVolumesAreTheClustersControlPlaneOnes covers what the
// cleanup deletes, which is gone for good: only a volume tagged for this
// cluster, created for a claim in the control plane's namespace, and attached
// to nothing - even if the API returned more than its filters asked for.
func TestTheLeftoverVolumesAreTheClustersControlPlaneOnes(t *testing.T) {
	attached := testEbsVolume("vol-attached", testEksCluster, testEksNamespace, ec2types.VolumeStateAvailable)
	attached.Attachments = []ec2types.VolumeAttachment{{InstanceId: aws.String("i-1")}}
	api := &fakeEbsApi{pages: [][]ec2types.Volume{{
		testEbsVolume("vol-crdb", testEksCluster, testEksNamespace, ec2types.VolumeStateAvailable),
		testEbsVolume("vol-other-cluster", "threeport-other", testEksNamespace, ec2types.VolumeStateAvailable),
		testEbsVolume("vol-retained-workload", testEksCluster, "wordpress", ec2types.VolumeStateAvailable),
		testEbsVolume("vol-in-use", testEksCluster, testEksNamespace, ec2types.VolumeStateInUse),
		attached,
		{VolumeId: aws.String("vol-untagged"), State: ec2types.VolumeStateAvailable},
	}}}

	deleted, err := DeleteLeftoverEksVolumes(context.Background(), api, testEksCluster, testEksNamespace, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"vol-crdb"}, deleted)
	assert.Equal(t, []string{"vol-crdb"}, api.deleted)

	// and the API is asked for exactly those
	require.NotEmpty(t, api.inputs)
	filters := map[string][]string{}
	for _, filter := range api.inputs[0].Filters {
		filters[aws.ToString(filter.Name)] = filter.Values
	}
	assert.Equal(t, map[string][]string{
		"tag-key": {"kubernetes.io/cluster/" + testEksCluster},
		"tag:kubernetes.io/created-for/pvc/namespace": {testEksNamespace},
		"status": {"available"},
	}, filters)
}

func TestEveryPageOfLeftoverVolumesIsDeleted(t *testing.T) {
	api := &fakeEbsApi{pages: [][]ec2types.Volume{
		{testEbsVolume("vol-crdb", testEksCluster, testEksNamespace, ec2types.VolumeStateAvailable)},
		{testEbsVolume("vol-nats", testEksCluster, testEksNamespace, ec2types.VolumeStateAvailable)},
	}}

	deleted, err := DeleteLeftoverEksVolumes(context.Background(), api, testEksCluster, testEksNamespace, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"vol-crdb", "vol-nats"}, deleted)
}

// TestOneVolumeThatCannotBeDeletedDoesNotKeepTheOthers covers a volume that
// fails, which is reported, and one already gone, which is not a failure.
func TestOneVolumeThatCannotBeDeletedDoesNotKeepTheOthers(t *testing.T) {
	api := &fakeEbsApi{
		pages: [][]ec2types.Volume{{
			testEbsVolume("vol-forbidden", testEksCluster, testEksNamespace, ec2types.VolumeStateAvailable),
			testEbsVolume("vol-gone", testEksCluster, testEksNamespace, ec2types.VolumeStateAvailable),
			testEbsVolume("vol-nats", testEksCluster, testEksNamespace, ec2types.VolumeStateAvailable),
		}},
		failDeletes: map[string]error{
			"vol-forbidden": errors.New("UnauthorizedOperation"),
			"vol-gone":      &smithy.GenericAPIError{Code: "InvalidVolume.NotFound"},
		},
	}

	deleted, err := DeleteLeftoverEksVolumes(context.Background(), api, testEksCluster, testEksNamespace, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "vol-forbidden")
	assert.NotContains(t, err.Error(), "vol-gone")
	assert.Equal(t, []string{"vol-nats"}, deleted)
}

// TestARetainedVolumeIsNeverDeleted covers a control plane volume whose
// persistent volume was changed to be retained: the disk keeps the tags it
// was provisioned with and is detached like the others once the cluster is
// gone, so only the list read from the cluster beforehand keeps it.
func TestARetainedVolumeIsNeverDeleted(t *testing.T) {
	api := &fakeEbsApi{pages: [][]ec2types.Volume{{
		testEbsVolume("vol-crdb", testEksCluster, testEksNamespace, ec2types.VolumeStateAvailable),
		testEbsVolume("vol-retained", testEksCluster, testEksNamespace, ec2types.VolumeStateAvailable),
	}}}

	deleted, err := DeleteLeftoverEksVolumes(context.Background(), api, testEksCluster, testEksNamespace, []string{"vol-retained"})
	require.NoError(t, err)
	assert.Equal(t, []string{"vol-crdb"}, deleted)
	assert.NotContains(t, api.deleted, "vol-retained")
}

// TestListingLeftoverVolumesDeletesNothing covers a teardown that could not
// read which volumes are retained, which only names them.
func TestListingLeftoverVolumesDeletesNothing(t *testing.T) {
	api := &fakeEbsApi{pages: [][]ec2types.Volume{{
		testEbsVolume("vol-crdb", testEksCluster, testEksNamespace, ec2types.VolumeStateAvailable),
		testEbsVolume("vol-other-cluster", "threeport-other", testEksNamespace, ec2types.VolumeStateAvailable),
	}}}

	leftover, err := LeftoverEksVolumes(context.Background(), api, testEksCluster, testEksNamespace)
	require.NoError(t, err)
	assert.Equal(t, []string{"vol-crdb"}, leftover)
	assert.Empty(t, api.deleted)
}
