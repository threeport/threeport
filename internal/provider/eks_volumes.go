package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// The tags the EBS CSI driver puts on a volume it provisions: the cluster it
// was created for, and the namespace of the claim it was created for.
const (
	ebsClusterTagPrefix     = "kubernetes.io/cluster/"
	ebsClaimNamespaceTag    = "kubernetes.io/created-for/pvc/namespace"
	ebsVolumeAvailable      = "available"
	ebsVolumeDeleteNotFound = "InvalidVolume.NotFound"
)

// ebsVolumeApi is the part of the EC2 API the leftover volume cleanup uses.
type ebsVolumeApi interface {
	DescribeVolumes(context.Context, *ec2.DescribeVolumesInput, ...func(*ec2.Options)) (*ec2.DescribeVolumesOutput, error)
	DeleteVolume(context.Context, *ec2.DeleteVolumeInput, ...func(*ec2.Options)) (*ec2.DeleteVolumeOutput, error)
}

// DeleteLeftoverEksVolumes deletes the EBS volumes the cluster's storage
// driver provisioned for claims in namespace that are still there once the
// cluster is gone, and returns their ids.
//
// Tearing down a control plane waits for the driver to delete the disks
// behind its claims, but a teardown with --infra-only does not delete the
// control plane at all, and a driver that has not finished when the cluster
// goes takes the rest of the deletion with it. Either way the volume is left
// unattached and billing.
//
// Only a volume that matches all three is deleted: tagged for this cluster,
// created for a claim in namespace, and attached to nothing. A volume another
// claim retained on purpose is in another namespace, and one still in use
// belongs to a cluster that still exists.
func DeleteLeftoverEksVolumes(ctx context.Context, ec2Api ebsVolumeApi, clusterName, namespace string) ([]string, error) {
	clusterTag := ebsClusterTagPrefix + clusterName
	paginator := ec2.NewDescribeVolumesPaginator(ec2Api, &ec2.DescribeVolumesInput{
		Filters: []ec2types.Filter{
			{Name: aws.String("tag-key"), Values: []string{clusterTag}},
			{Name: aws.String("tag:" + ebsClaimNamespaceTag), Values: []string{namespace}},
			{Name: aws.String("status"), Values: []string{ebsVolumeAvailable}},
		},
	})

	var deleted []string
	var deleteErrors []error
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return deleted, fmt.Errorf("failed to list volumes left by cluster %s: %w", clusterName, err)
		}
		for _, volume := range page.Volumes {
			// checked again rather than trusting the filter alone, since a
			// volume deleted here is gone for good
			if volume.VolumeId == nil || !leftoverEksVolume(volume, clusterTag, namespace) {
				continue
			}
			if _, err := ec2Api.DeleteVolume(ctx, &ec2.DeleteVolumeInput{VolumeId: volume.VolumeId}); err != nil {
				if IsAwsErrorCode(err, ebsVolumeDeleteNotFound) {
					continue
				}
				deleteErrors = append(deleteErrors, fmt.Errorf("failed to delete volume %s: %w", *volume.VolumeId, err))
				continue
			}
			deleted = append(deleted, *volume.VolumeId)
		}
	}

	return deleted, errors.Join(deleteErrors...)
}

// leftoverEksVolume reports whether a volume is one the cleanup deletes.
func leftoverEksVolume(volume ec2types.Volume, clusterTag, namespace string) bool {
	if volume.State != ec2types.VolumeStateAvailable || len(volume.Attachments) > 0 {
		return false
	}
	taggedForCluster, taggedForNamespace := false, false
	for _, tag := range volume.Tags {
		switch aws.ToString(tag.Key) {
		case clusterTag:
			taggedForCluster = true
		case ebsClaimNamespaceTag:
			taggedForNamespace = aws.ToString(tag.Value) == namespace
		}
	}

	return taggedForCluster && taggedForNamespace
}
