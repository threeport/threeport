package provider

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awseks "github.com/aws/aws-sdk-go-v2/service/eks"
	ekstypes "github.com/aws/aws-sdk-go-v2/service/eks/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"

	tpaws "github.com/threeport/threeport/pkg/aws/v0"
)

var (
	// vpcCniRolloutTimeout bounds the wait for the VPC CNI plugin's pods to
	// be recreated on the role of their own.
	vpcCniRolloutTimeout = 10 * time.Minute

	// vpcCniRolloutPollInterval is how often the wait asks whether they are.
	vpcCniRolloutPollInterval = 5 * time.Second
)

// vpcCniRoleAnnotation is set on the aws-node pod template to the role the
// pods run as.  The service account's role only reaches pods created after
// it is set, so a new value recreates them, and a create resumed with the
// same role leaves them as they are.
const vpcCniRoleAnnotation = "threeport.io/vpc-cni-role-arn"

// eksAddonUpdater is the part of the EKS API binding the vpc-cni add-on to a
// role uses.
type eksAddonUpdater interface {
	awseks.DescribeAddonAPIClient
	UpdateAddon(context.Context, *awseks.UpdateAddonInput, ...func(*awseks.Options)) (*awseks.UpdateAddonOutput, error)
}

// iamPolicyDetacher is the part of the IAM API detachIamPolicy uses.
type iamPolicyDetacher interface {
	iam.ListAttachedRolePoliciesAPIClient
	DetachRolePolicy(context.Context, *iam.DetachRolePolicyInput, ...func(*iam.Options)) (*iam.DetachRolePolicyOutput, error)
}

// newVpcCniAddonClient, newVpcCniIamClient and newVpcCniKubeClient return the
// clients moving the VPC CNI plugin's policy uses.  A test stands in for them.
var (
	newVpcCniAddonClient = func(cfg aws.Config) eksAddonUpdater {
		return awseks.NewFromConfig(cfg)
	}
	newVpcCniIamClient = func(cfg aws.Config) iamPolicyDetacher {
		return iam.NewFromConfig(cfg)
	}
	newVpcCniKubeClient = func(i *KubernetesRuntimeInfraEKS) (kubernetes.Interface, error) {
		connection, err := i.GetConnection()
		if err != nil {
			return nil, err
		}

		return kubernetes.NewForConfig(&rest.Config{
			Host:        connection.APIEndpoint,
			BearerToken: connection.Token,
			TLSClientConfig: rest.TLSClientConfig{
				CAData: []byte(connection.CACertificate),
			},
		})
	}
)

// moveVpcCniPolicyToItsRole has the VPC CNI plugin run as a role of its own,
// roleArn, and then takes its policy off the node role, so a pod that reaches
// the nodes' metadata service gets no permission to change their network
// interfaces.  The policy only comes off once every aws-node pod runs as the
// role: a pod still on the node's credentials would lose them, and with them
// the addresses it hands out.
func (i *KubernetesRuntimeInfraEKS) moveVpcCniPolicyToItsRole(roleArn string) error {
	if i.AwsConfig == nil {
		return errors.New("cannot bind the vpc-cni add-on without an AWS config")
	}
	if roleArn == "" {
		return errors.New("no VPC CNI role to bind the vpc-cni add-on to")
	}

	if err := bindVpcCniAddon(newVpcCniAddonClient(*i.AwsConfig), i.RuntimeInstanceName, roleArn); err != nil {
		return err
	}

	kubeClient, err := newVpcCniKubeClient(i)
	if err != nil {
		return fmt.Errorf("failed to connect to the cluster to recreate the aws-node pods: %w", err)
	}
	if err := recreateAwsNodePods(kubeClient, roleArn); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), ebsStorageAddonRequestTimeout)
	defer cancel()

	return detachIamPolicy(ctx,
		newVpcCniIamClient(*i.AwsConfig),
		tpaws.EksNodeRoleName(i.RuntimeInstanceName),
		tpaws.EksCniPolicyArn,
	)
}

// bindVpcCniAddon sets the role the cluster's vpc-cni add-on runs as, and
// waits for the add-on to be active.  An add-on already on the role is left
// as it is.
func bindVpcCniAddon(eksClient eksAddonUpdater, clusterName, roleArn string) error {
	ctx, cancel := context.WithTimeout(context.Background(), ebsStorageAddonRequestTimeout)
	defer cancel()
	addon := &awseks.DescribeAddonInput{
		AddonName:   aws.String(tpaws.EksVpcCniAddonName),
		ClusterName: aws.String(clusterName),
	}

	current, err := eksClient.DescribeAddon(ctx, addon)
	if err != nil {
		return fmt.Errorf("failed to describe the vpc-cni add-on: %w", err)
	}
	if aws.ToString(current.Addon.ServiceAccountRoleArn) == roleArn {
		return nil
	}

	// EKS refuses to update an add-on it is still creating or updating
	if status := current.Addon.Status; status == ekstypes.AddonStatusCreating || status == ekstypes.AddonStatusUpdating {
		if err := waitForAddonActive(eksClient, addon); err != nil {
			return fmt.Errorf("failed waiting for the vpc-cni add-on to be active before binding it to its role: %w", err)
		}
	}

	updateCtx, cancelUpdate := context.WithTimeout(context.Background(), ebsStorageAddonRequestTimeout)
	defer cancelUpdate()
	if _, err := eksClient.UpdateAddon(updateCtx, &awseks.UpdateAddonInput{
		AddonName:             addon.AddonName,
		ClusterName:           addon.ClusterName,
		ServiceAccountRoleArn: aws.String(roleArn),
		ResolveConflicts:      ekstypes.ResolveConflictsOverwrite,
	}); err != nil {
		return fmt.Errorf("failed to bind the vpc-cni add-on to role %s: %w", roleArn, err)
	}
	if err := waitForAddonActive(eksClient, addon); err != nil {
		return fmt.Errorf("failed waiting for the vpc-cni add-on to be active on its role: %w", err)
	}

	return nil
}

// recreateAwsNodePods recreates the aws-node pods, unless they were already
// recreated for roleArn, and waits for every node to run a new one.  The role
// set on their service account only reaches pods created after it.
func recreateAwsNodePods(kubeClient kubernetes.Interface, roleArn string) error {
	daemonSets := kubeClient.AppsV1().DaemonSets(tpaws.EksVpcCniServiceAccountNamespace)
	name := tpaws.EksVpcCniServiceAccountName

	ctx, cancel := context.WithTimeout(context.Background(), vpcCniRolloutTimeout)
	defer cancel()

	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		daemonSet, err := daemonSets.Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if daemonSet.Spec.Template.Annotations[vpcCniRoleAnnotation] == roleArn {
			return nil
		}
		if daemonSet.Spec.Template.Annotations == nil {
			daemonSet.Spec.Template.Annotations = map[string]string{}
		}
		daemonSet.Spec.Template.Annotations[vpcCniRoleAnnotation] = roleArn
		_, err = daemonSets.Update(ctx, daemonSet, metav1.UpdateOptions{})
		return err
	}); err != nil {
		return fmt.Errorf("failed to recreate the aws-node pods on role %s: %w", roleArn, err)
	}

	if err := wait.PollUntilContextCancel(ctx, vpcCniRolloutPollInterval, true, func(ctx context.Context) (bool, error) {
		daemonSet, err := daemonSets.Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, nil
		}
		status := daemonSet.Status

		return status.ObservedGeneration >= daemonSet.Generation &&
			status.UpdatedNumberScheduled == status.DesiredNumberScheduled &&
			status.NumberAvailable == status.DesiredNumberScheduled, nil
	}); err != nil {
		return fmt.Errorf("failed waiting for every node to run an aws-node pod on role %s: %w", roleArn, err)
	}

	return nil
}

// detachIamPolicy detaches a managed policy from a role, if it is attached.
func detachIamPolicy(ctx context.Context, iamClient iamPolicyDetacher, roleName, policyArn string) error {
	attachedPolicies, err := listAttachedPolicies(ctx, iamClient, roleName)
	if err != nil {
		return err
	}
	for _, attachedPolicy := range attachedPolicies {
		if aws.ToString(attachedPolicy.PolicyArn) != policyArn {
			continue
		}
		_, err := iamClient.DetachRolePolicy(ctx, &iam.DetachRolePolicyInput{
			RoleName:  aws.String(roleName),
			PolicyArn: aws.String(policyArn),
		})
		var notFound *types.NoSuchEntityException
		if err != nil && !errors.As(err, &notFound) {
			return fmt.Errorf("failed to detach policy %s from role %s: %w", policyArn, roleName, err)
		}
	}

	return nil
}
