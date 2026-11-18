package provider

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awseks "github.com/aws/aws-sdk-go-v2/service/eks"
	ekstypes "github.com/aws/aws-sdk-go-v2/service/eks/types"
)

// eksDeletionProtectionAPI is the part of the EKS API that turns a cluster's
// deletion protection on and off.
type eksDeletionProtectionAPI interface {
	DescribeCluster(context.Context, *awseks.DescribeClusterInput, ...func(*awseks.Options)) (*awseks.DescribeClusterOutput, error)
	UpdateClusterConfig(context.Context, *awseks.UpdateClusterConfigInput, ...func(*awseks.Options)) (*awseks.UpdateClusterConfigOutput, error)
	DescribeUpdate(context.Context, *awseks.DescribeUpdateInput, ...func(*awseks.Options)) (*awseks.DescribeUpdateOutput, error)
}

// newEksDeletionProtectionClient returns the EKS client for an AWS config.  A
// test stands in for it.
var newEksDeletionProtectionClient = func(cfg aws.Config) eksDeletionProtectionAPI {
	return awseks.NewFromConfig(cfg)
}

var (
	// eksDeletionProtectionPollInterval is how often the cluster and the
	// update are checked while waiting.
	eksDeletionProtectionPollInterval = 15 * time.Second

	// eksDeletionProtectionTimeout bounds the wait for the cluster to be
	// active and for the update to finish.
	eksDeletionProtectionTimeout = 30 * time.Minute
)

// setEksDeletionProtection turns the cluster's own deletion protection on or
// off, so EKS refuses or allows its deletion through its API.  It is set
// through the API rather than the Pulumi program so it can be turned off
// without applying the program.  A cluster that doesn't exist, or already has
// it as asked, is left alone.
func (i *KubernetesRuntimeInfraEKS) setEksDeletionProtection(protect bool) error {
	if i.AwsConfig == nil {
		return errors.New("cannot set the EKS cluster's deletion protection without an AWS config")
	}
	ctx, cancel := context.WithTimeout(context.Background(), eksDeletionProtectionTimeout)
	defer cancel()

	return setEksDeletionProtection(ctx, newEksDeletionProtectionClient(*i.AwsConfig), i.RuntimeInstanceName, protect)
}

// setEksDeletionProtection sets a cluster's deletion protection with client.
// EKS changes a cluster's configuration only while it is active, with no other
// update in progress, so it waits for that, and then for its own update to
// finish.
func setEksDeletionProtection(ctx context.Context, client eksDeletionProtectionAPI, clusterName string, protect bool) error {
	for {
		output, err := client.DescribeCluster(ctx, &awseks.DescribeClusterInput{Name: aws.String(clusterName)})
		if err != nil {
			var notFound *ekstypes.ResourceNotFoundException
			if errors.As(err, &notFound) {
				return nil
			}
			return fmt.Errorf("failed to describe EKS cluster %s: %w", clusterName, err)
		}
		if aws.ToBool(output.Cluster.DeletionProtection) == protect {
			return nil
		}
		// a cluster in any state but one that settles on its own is asked
		// right away, for EKS to say why it can't change it
		if !eksClusterSettling(output.Cluster.Status) {
			break
		}
		if err := waitEksDeletionProtectionPoll(ctx, fmt.Sprintf("EKS cluster %s to be active", clusterName)); err != nil {
			return err
		}
	}

	updated, err := client.UpdateClusterConfig(ctx, &awseks.UpdateClusterConfigInput{
		Name:               aws.String(clusterName),
		DeletionProtection: aws.Bool(protect),
	})
	if err != nil {
		return fmt.Errorf("failed to set deletion protection to %t on EKS cluster %s: %w", protect, clusterName, err)
	}
	if updated.Update == nil || updated.Update.Id == nil {
		return nil
	}

	for {
		described, err := client.DescribeUpdate(ctx, &awseks.DescribeUpdateInput{
			Name:     aws.String(clusterName),
			UpdateId: updated.Update.Id,
		})
		if err != nil {
			return fmt.Errorf("failed to check the deletion protection update of EKS cluster %s: %w", clusterName, err)
		}
		switch described.Update.Status {
		case ekstypes.UpdateStatusSuccessful:
			return nil
		case ekstypes.UpdateStatusFailed, ekstypes.UpdateStatusCancelled:
			return fmt.Errorf(
				"the update setting deletion protection to %t on EKS cluster %s ended %s: %s",
				protect, clusterName, described.Update.Status, eksUpdateErrors(described.Update.Errors),
			)
		}
		if err := waitEksDeletionProtectionPoll(ctx, fmt.Sprintf("the deletion protection update of EKS cluster %s", clusterName)); err != nil {
			return err
		}
	}
}

// eksClusterSettling reports whether a cluster's status changes on its own,
// to active or to failed.
func eksClusterSettling(status ekstypes.ClusterStatus) bool {
	switch status {
	case ekstypes.ClusterStatusCreating, ekstypes.ClusterStatusUpdating, ekstypes.ClusterStatusPending:
		return true
	}

	return false
}

// waitEksDeletionProtectionPoll waits one poll interval, or fails once ctx
// is done.
func waitEksDeletionProtectionPoll(ctx context.Context, waitingFor string) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("timed out waiting for %s: %w", waitingFor, ctx.Err())
	case <-time.After(eksDeletionProtectionPollInterval):
		return nil
	}
}

// eksUpdateErrors describes the errors EKS reports on an update.
func eksUpdateErrors(updateErrors []ekstypes.ErrorDetail) string {
	if len(updateErrors) == 0 {
		return "no error reported"
	}
	descriptions := make([]string, 0, len(updateErrors))
	for _, updateError := range updateErrors {
		descriptions = append(descriptions, fmt.Sprintf("%s: %s", updateError.ErrorCode, aws.ToString(updateError.ErrorMessage)))
	}

	return fmt.Sprint(descriptions)
}
