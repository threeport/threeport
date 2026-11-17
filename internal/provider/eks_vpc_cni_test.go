package provider

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awseks "github.com/aws/aws-sdk-go-v2/service/eks"
	ekstypes "github.com/aws/aws-sdk-go-v2/service/eks/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	tpaws "github.com/threeport/threeport/pkg/aws/v0"
)

// vpcCniEvents records, in order, what moving the VPC CNI plugin's policy did.
type vpcCniEvents struct {
	mu     sync.Mutex
	events []string
}

func (e *vpcCniEvents) add(event string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = append(e.events, event)
}

func (e *vpcCniEvents) list() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.events...)
}

// fakeVpcCniAddon is a vpc-cni add-on on a role, reporting the statuses
// given, one per describe.
type fakeVpcCniAddon struct {
	mu       sync.Mutex
	roleArn  string
	statuses []ekstypes.AddonStatus
	reads    int
	updates  []*awseks.UpdateAddonInput
	events   *vpcCniEvents
}

func (f *fakeVpcCniAddon) DescribeAddon(_ context.Context, input *awseks.DescribeAddonInput, _ ...func(*awseks.Options)) (*awseks.DescribeAddonOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	status := ekstypes.AddonStatusActive
	if len(f.statuses) > 0 {
		status = f.statuses[min(f.reads, len(f.statuses)-1)]
	}
	f.reads++

	return &awseks.DescribeAddonOutput{Addon: &ekstypes.Addon{
		AddonName:             input.AddonName,
		Status:                status,
		ServiceAccountRoleArn: aws.String(f.roleArn),
	}}, nil
}

func (f *fakeVpcCniAddon) UpdateAddon(_ context.Context, input *awseks.UpdateAddonInput, _ ...func(*awseks.Options)) (*awseks.UpdateAddonOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updates = append(f.updates, input)
	f.roleArn = aws.ToString(input.ServiceAccountRoleArn)
	if f.events != nil {
		f.events.add("bind " + f.roleArn)
	}

	return &awseks.UpdateAddonOutput{}, nil
}

// fakeNodeRole is a role with the managed policies given attached.
type fakeNodeRole struct {
	mu       sync.Mutex
	attached []string
	detached []string
	events   *vpcCniEvents
}

func (f *fakeNodeRole) ListAttachedRolePolicies(context.Context, *iam.ListAttachedRolePoliciesInput, ...func(*iam.Options)) (*iam.ListAttachedRolePoliciesOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var policies []types.AttachedPolicy
	for _, arn := range f.attached {
		policies = append(policies, types.AttachedPolicy{PolicyArn: aws.String(arn)})
	}

	return &iam.ListAttachedRolePoliciesOutput{AttachedPolicies: policies}, nil
}

func (f *fakeNodeRole) DetachRolePolicy(_ context.Context, input *iam.DetachRolePolicyInput, _ ...func(*iam.Options)) (*iam.DetachRolePolicyOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.detached = append(f.detached, aws.ToString(input.RoleName)+" "+aws.ToString(input.PolicyArn))
	if f.events != nil {
		f.events.add("detach " + aws.ToString(input.PolicyArn))
	}

	return &iam.DetachRolePolicyOutput{}, nil
}

// awsNodeDaemonSet returns the aws-node DaemonSet, rolled out on nodes nodes.
func awsNodeDaemonSet(nodes int32, annotations map[string]string) *appsv1.DaemonSet {
	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "aws-node", Namespace: "kube-system", Generation: 1},
		Spec:       appsv1.DaemonSetSpec{Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Annotations: annotations}}},
		Status: appsv1.DaemonSetStatus{
			ObservedGeneration:     1,
			DesiredNumberScheduled: nodes,
			UpdatedNumberScheduled: nodes,
			NumberAvailable:        nodes,
		},
	}
}

// rollingAwsNode makes the fake cluster's aws-node DaemonSet behave as a real
// one does when its pod template changes: its generation goes up, and the
// pods are recreated one poll at a time.
func rollingAwsNode(clientset *fake.Clientset, events *vpcCniEvents) {
	var mu sync.Mutex
	rolledOut := int32(-1)
	clientset.PrependReactor("update", "daemonsets", func(action k8stesting.Action) (bool, runtime.Object, error) {
		daemonSet := action.(k8stesting.UpdateAction).GetObject().(*appsv1.DaemonSet).DeepCopy()
		daemonSet.Generation++
		daemonSet.Status.UpdatedNumberScheduled = 0
		mu.Lock()
		rolledOut = 0
		mu.Unlock()
		events.add("recreate aws-node")
		return false, nil, clientset.Tracker().Update(appsv1.SchemeGroupVersion.WithResource("daemonsets"), daemonSet, "kube-system")
	})
	clientset.PrependReactor("get", "daemonsets", func(k8stesting.Action) (bool, runtime.Object, error) {
		object, err := clientset.Tracker().Get(appsv1.SchemeGroupVersion.WithResource("daemonsets"), "kube-system", "aws-node")
		if err != nil {
			return true, nil, err
		}
		daemonSet := object.(*appsv1.DaemonSet).DeepCopy()
		mu.Lock()
		defer mu.Unlock()
		if rolledOut >= 0 && rolledOut < daemonSet.Status.DesiredNumberScheduled {
			rolledOut++
			daemonSet.Status.ObservedGeneration = daemonSet.Generation
			daemonSet.Status.UpdatedNumberScheduled = rolledOut
			if rolledOut == daemonSet.Status.DesiredNumberScheduled {
				events.add("aws-node rolled out")
			}
			_ = clientset.Tracker().Update(appsv1.SchemeGroupVersion.WithResource("daemonsets"), daemonSet, "kube-system")
		}
		return true, daemonSet, nil
	})
}

// fastVpcCniWaits makes the waits poll quickly for a test.
func fastVpcCniWaits(t *testing.T) {
	t.Helper()
	interval, timeout, addonInterval := vpcCniRolloutPollInterval, vpcCniRolloutTimeout, ebsStorageAddonPollInterval
	t.Cleanup(func() {
		vpcCniRolloutPollInterval, vpcCniRolloutTimeout, ebsStorageAddonPollInterval = interval, timeout, addonInterval
	})
	vpcCniRolloutPollInterval = time.Millisecond
	vpcCniRolloutTimeout = 5 * time.Second
	ebsStorageAddonPollInterval = time.Second
}

// useVpcCniClients stands the clients in for a test.
func useVpcCniClients(t *testing.T, addon eksAddonUpdater, nodeRole iamPolicyDetacher, kubeClient kubernetes.Interface) {
	t.Helper()
	newAddon, newIam, newKube := newVpcCniAddonClient, newVpcCniIamClient, newVpcCniKubeClient
	t.Cleanup(func() { newVpcCniAddonClient, newVpcCniIamClient, newVpcCniKubeClient = newAddon, newIam, newKube })
	newVpcCniAddonClient = func(aws.Config) eksAddonUpdater { return addon }
	newVpcCniIamClient = func(aws.Config) iamPolicyDetacher { return nodeRole }
	newVpcCniKubeClient = func(*KubernetesRuntimeInfraEKS) (kubernetes.Interface, error) { return kubeClient, nil }
}

// TestTheVpcCniPolicyLeavesTheNodeRoleOnceAwsNodeRunsAsItsOwnRole covers a
// new cluster: the vpc-cni add-on is bound to its role, every aws-node pod is
// recreated so it runs as that role, and only then does the CNI policy come
// off the node role, leaving the node role's other policies.
func TestTheVpcCniPolicyLeavesTheNodeRoleOnceAwsNodeRunsAsItsOwnRole(t *testing.T) {
	fastVpcCniWaits(t)
	events := &vpcCniEvents{}
	infra := testEksInfra(2)
	roleArn := tpaws.EksVpcCniRoleArn(infra.AwsAccountID, infra.RuntimeInstanceName)

	addon := &fakeVpcCniAddon{events: events}
	nodeRole := &fakeNodeRole{events: events, attached: []string{
		tpaws.EksWorkerNodePolicyArn, tpaws.EksContainerRegistryPolicyArn, tpaws.EksCniPolicyArn,
	}}
	clientset := fake.NewClientset(awsNodeDaemonSet(3, nil))
	rollingAwsNode(clientset, events)
	useVpcCniClients(t, addon, nodeRole, clientset)

	require.NoError(t, infra.moveVpcCniPolicyToItsRole(roleArn))

	assert.Equal(t, []string{
		"bind " + roleArn,
		"recreate aws-node",
		"aws-node rolled out",
		"detach " + tpaws.EksCniPolicyArn,
	}, events.list())
	require.Len(t, addon.updates, 1)
	assert.Equal(t, "vpc-cni", aws.ToString(addon.updates[0].AddonName))
	assert.Equal(t, "eks-test", aws.ToString(addon.updates[0].ClusterName))
	assert.Equal(t, ekstypes.ResolveConflictsOverwrite, addon.updates[0].ResolveConflicts)
	assert.Equal(t, []string{"worker-role-eks-test " + tpaws.EksCniPolicyArn}, nodeRole.detached)

	daemonSet, err := clientset.AppsV1().DaemonSets("kube-system").Get(context.Background(), "aws-node", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, roleArn, daemonSet.Spec.Template.Annotations[vpcCniRoleAnnotation])
}

// TestAResumedCreateLeavesAwsNodeAsItIs covers a create resumed after the
// add-on was bound and its pods recreated: CreateClusterIam has put the CNI
// policy back on the node role, and it comes off again, with the add-on and
// its pods left as they are.
func TestAResumedCreateLeavesAwsNodeAsItIs(t *testing.T) {
	fastVpcCniWaits(t)
	events := &vpcCniEvents{}
	infra := testEksInfra(2)
	roleArn := tpaws.EksVpcCniRoleArn(infra.AwsAccountID, infra.RuntimeInstanceName)

	addon := &fakeVpcCniAddon{events: events, roleArn: roleArn}
	nodeRole := &fakeNodeRole{events: events, attached: []string{tpaws.EksWorkerNodePolicyArn, tpaws.EksCniPolicyArn}}
	clientset := fake.NewClientset(awsNodeDaemonSet(3, map[string]string{vpcCniRoleAnnotation: roleArn}))
	rollingAwsNode(clientset, events)
	useVpcCniClients(t, addon, nodeRole, clientset)

	require.NoError(t, infra.moveVpcCniPolicyToItsRole(roleArn))
	assert.Equal(t, []string{"detach " + tpaws.EksCniPolicyArn}, events.list())

	// and a node role without the policy is left alone
	nodeRole.attached = []string{tpaws.EksWorkerNodePolicyArn}
	nodeRole.detached = nil
	require.NoError(t, infra.moveVpcCniPolicyToItsRole(roleArn))
	assert.Empty(t, nodeRole.detached)
}

// TestTheCniPolicyStaysWhenAwsNodeIsNotRecreated covers aws-node pods that
// don't all come up on the role in time: the policy stays on the node role,
// since the pods still on the node's credentials need it to hand out
// addresses.
func TestTheCniPolicyStaysWhenAwsNodeIsNotRecreated(t *testing.T) {
	fastVpcCniWaits(t)
	vpcCniRolloutTimeout = 50 * time.Millisecond
	infra := testEksInfra(2)
	roleArn := tpaws.EksVpcCniRoleArn(infra.AwsAccountID, infra.RuntimeInstanceName)

	stuck := awsNodeDaemonSet(3, nil)
	stuck.Status.NumberAvailable = 2
	nodeRole := &fakeNodeRole{attached: []string{tpaws.EksCniPolicyArn}}
	useVpcCniClients(t, &fakeVpcCniAddon{}, nodeRole, fake.NewClientset(stuck))

	err := infra.moveVpcCniPolicyToItsRole(roleArn)
	assert.ErrorContains(t, err, "failed waiting for every node to run an aws-node pod on role "+roleArn)
	assert.Empty(t, nodeRole.detached)

	// nor when the add-on can't be bound
	failing := &failingVpcCniAddon{}
	useVpcCniClients(t, failing, nodeRole, fake.NewClientset(awsNodeDaemonSet(3, nil)))
	err = infra.moveVpcCniPolicyToItsRole(roleArn)
	assert.ErrorContains(t, err, "failed to bind the vpc-cni add-on to role "+roleArn)
	assert.Empty(t, nodeRole.detached)
}

// failingVpcCniAddon is a vpc-cni add-on EKS refuses to update.
type failingVpcCniAddon struct{ fakeVpcCniAddon }

func (f *failingVpcCniAddon) UpdateAddon(context.Context, *awseks.UpdateAddonInput, ...func(*awseks.Options)) (*awseks.UpdateAddonOutput, error) {
	return nil, errors.New("InvalidParameterException")
}

// TestTheVpcCniAddonIsBoundOnceItIsActive covers an add-on EKS is still
// creating, as it can be right after the cluster's create: the binding waits
// for it to be active, since EKS refuses to update it before.
func TestTheVpcCniAddonIsBoundOnceItIsActive(t *testing.T) {
	fastVpcCniWaits(t)
	addon := &fakeVpcCniAddon{statuses: []ekstypes.AddonStatus{
		ekstypes.AddonStatusCreating,                             // the first look
		ekstypes.AddonStatusCreating, ekstypes.AddonStatusActive, // the wait before the update
		ekstypes.AddonStatusActive, // the wait after it
	}}

	require.NoError(t, bindVpcCniAddon(addon, "eks-test", "arn:aws:iam::123456789012:role/vpc-cni-role-eks-test"))
	require.Len(t, addon.updates, 1)
	assert.Equal(t, 4, addon.reads, "it waited for the add-on before and after the update")
}

// TestMoveVpcCniPolicyNeedsARole covers a call without the role: it is
// refused before anything changes.
func TestMoveVpcCniPolicyNeedsARole(t *testing.T) {
	nodeRole := &fakeNodeRole{attached: []string{tpaws.EksCniPolicyArn}}
	addon := &fakeVpcCniAddon{}
	useVpcCniClients(t, addon, nodeRole, fake.NewClientset())

	assert.ErrorContains(t, testEksInfra(2).moveVpcCniPolicyToItsRole(""), "no VPC CNI role")
	assert.Empty(t, addon.updates)
	assert.Empty(t, nodeRole.detached)
}
