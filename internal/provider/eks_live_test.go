package provider

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	awseks "github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	tpaws "github.com/threeport/threeport/pkg/aws/v0"
)

// TestEksLiveCreateDelete provisions a real EKS cluster, connects to it, and
// tears it down.  It costs money and takes about half an hour, so it only runs
// when asked for explicitly.
func TestEksLiveCreateDelete(t *testing.T) {
	if os.Getenv("THREEPORT_EKS_LIVE") != "1" {
		t.Skip("set THREEPORT_EKS_LIVE=1 to run against a real AWS account")
	}

	ctx := context.Background()
	region := "us-east-1"
	clusterName := "tp-eks-live-test"

	awsConfig, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	require.NoError(t, err)

	callerIdentity, err := sts.NewFromConfig(awsConfig).GetCallerIdentity(
		ctx,
		&sts.GetCallerIdentityInput{},
	)
	require.NoError(t, err)
	t.Logf("account %s as %s", *callerIdentity.Account, *callerIdentity.Arn)

	// Leave ProviderCredentials empty so the Pulumi provider resolves
	// credentials from its own environment and refreshes them as it goes.
	// This is the branch a control plane running inside EKS takes, where the
	// pod is authenticated through IRSA.  It is also the only branch a local
	// run can exercise: the credentials an `aws login` session hands out last
	// fifteen minutes, so passing them as values would freeze them and fail
	// partway through exactly as assumed-role credentials did.
	infra := &KubernetesRuntimeInfraEKS{
		PulumiWorkspace: PulumiWorkspace{
			RuntimeInstanceName: clusterName,
		},
		AwsAccountID:                 *callerIdentity.Account,
		AwsConfig:                    &awsConfig,
		Region:                       region,
		ZoneCount:                    2,
		DefaultNodeGroupInstanceType: "t3.medium",
		DefaultNodeGroupInitialNodes: 2,
		DefaultNodeGroupMinNodes:     1,
		DefaultNodeGroupMaxNodes:     3,
	}

	// tear down no matter how the create goes, so a failure part way through
	// does not leave the account holding a cluster
	t.Cleanup(func() {
		t.Log("=== DELETE starting ===")
		deleteStart := time.Now()
		if err := infra.Delete(); err != nil {
			t.Errorf("delete failed after %s: %v", time.Since(deleteStart).Round(time.Second), err)
			return
		}
		t.Logf("=== DELETE done in %s ===", time.Since(deleteStart).Round(time.Second))
	})

	t.Log("=== CREATE starting ===")
	createStart := time.Now()
	kubeConnectionInfo, err := infra.Create()
	require.NoError(t, err, "create failed after %s", time.Since(createStart).Round(time.Second))
	t.Logf("=== CREATE done in %s ===", time.Since(createStart).Round(time.Second))

	require.NotEmpty(t, kubeConnectionInfo.APIEndpoint)
	require.NotEmpty(t, kubeConnectionInfo.CACertificate)
	require.NotEmpty(t, kubeConnectionInfo.Token)
	t.Logf("api endpoint: %s", kubeConnectionInfo.APIEndpoint)
	t.Logf("token expires: %s", kubeConnectionInfo.TokenExpiration)

	// the token is the part unit tests could only check the shape of - this is
	// the EKS API server actually accepting it
	kubeClient, err := kubernetes.NewForConfig(&rest.Config{
		Host:        kubeConnectionInfo.APIEndpoint,
		BearerToken: kubeConnectionInfo.Token,
		TLSClientConfig: rest.TLSClientConfig{
			CAData: []byte(kubeConnectionInfo.CACertificate),
		},
	})
	require.NoError(t, err)

	nodes, err := kubeClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	require.NoError(t, err, "failed to list nodes with the generated token")
	t.Logf("cluster reports %d nodes", len(nodes.Items))
	for _, node := range nodes.Items {
		t.Logf("  node %s", node.Name)
	}

	// the EBS CSI addon runs as the storage management role through IRSA
	pods, err := kubeClient.CoreV1().Pods("kube-system").List(ctx, metav1.ListOptions{})
	require.NoError(t, err)
	for _, pod := range pods.Items {
		t.Logf("  kube-system pod %s: %s", pod.Name, pod.Status.Phase)
	}
}

// TestEksLiveTeardown destroys a cluster left behind by an interrupted run,
// exercising the real teardown path rather than deleting resources by hand.
func TestEksLiveTeardown(t *testing.T) {
	if os.Getenv("THREEPORT_EKS_TEARDOWN") == "" {
		t.Skip("set THREEPORT_EKS_TEARDOWN=<cluster name> to tear down a real cluster")
	}
	clusterName := os.Getenv("THREEPORT_EKS_TEARDOWN")

	ctx := context.Background()
	region := "us-east-1"

	awsConfig, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	require.NoError(t, err)
	callerIdentity, err := sts.NewFromConfig(awsConfig).GetCallerIdentity(
		ctx,
		&sts.GetCallerIdentityInput{},
	)
	require.NoError(t, err)

	infra := &KubernetesRuntimeInfraEKS{
		PulumiWorkspace: PulumiWorkspace{RuntimeInstanceName: clusterName},
		AwsAccountID:    *callerIdentity.Account,
		AwsConfig:       &awsConfig,
		Region:          region,
	}

	t.Logf("=== DELETE %s starting ===", clusterName)
	deleteStart := time.Now()
	require.NoError(t, infra.Delete())
	t.Logf("=== DELETE done in %s ===", time.Since(deleteStart).Round(time.Second))
}

// TestEksLiveVerify inspects a cluster that is already running: the EBS CSI
// addon that proves IRSA works, and the nodes reachable with a generated
// token.
func TestEksLiveVerify(t *testing.T) {
	if os.Getenv("THREEPORT_EKS_VERIFY") == "" {
		t.Skip("set THREEPORT_EKS_VERIFY=<cluster name> to inspect a running cluster")
	}
	clusterName := os.Getenv("THREEPORT_EKS_VERIFY")

	ctx := context.Background()
	awsConfig, err := config.LoadDefaultConfig(ctx, config.WithRegion("us-east-1"))
	require.NoError(t, err)

	// the EBS CSI addon runs as the storage management role through the
	// cluster's OIDC provider, so an active addon is IRSA working end to end
	addon, err := awseks.NewFromConfig(awsConfig).DescribeAddon(ctx, &awseks.DescribeAddonInput{
		ClusterName: &clusterName,
		AddonName:   aws.String("aws-ebs-csi-driver"),
	})
	require.NoError(t, err, "the EBS CSI addon was not created")
	t.Logf("addon %s status=%s", *addon.Addon.AddonName, addon.Addon.Status)
	t.Logf("addon service account role: %s", aws.ToString(addon.Addon.ServiceAccountRoleArn))
	assert.Equal(t, "ACTIVE", string(addon.Addon.Status))

	// the cluster's admin is the role threeport provisioned it with, so the
	// connection has to be made as that role - a local user identity gets
	// Unauthorized, which is EKS working as intended
	callerIdentity, err := sts.NewFromConfig(awsConfig).GetCallerIdentity(
		ctx,
		&sts.GetCallerIdentityInput{},
	)
	require.NoError(t, err)
	resourceManagerConfig, err := tpaws.AssumeRole(
		awsConfig,
		GetResourceManagerRoleArn(
			strings.TrimPrefix(clusterName, "threeport-"),
			*callerIdentity.Account,
		),
		"us-east-1",
	)
	require.NoError(t, err)

	infra := &KubernetesRuntimeInfraEKS{
		PulumiWorkspace: PulumiWorkspace{RuntimeInstanceName: clusterName},
		AwsConfig:       resourceManagerConfig,
		Region:          "us-east-1",
	}
	kubeConnectionInfo, err := infra.GetConnection()
	require.NoError(t, err)

	kubeClient, err := kubernetes.NewForConfig(&rest.Config{
		Host:        kubeConnectionInfo.APIEndpoint,
		BearerToken: kubeConnectionInfo.Token,
		TLSClientConfig: rest.TLSClientConfig{
			CAData: []byte(kubeConnectionInfo.CACertificate),
		},
	})
	require.NoError(t, err)

	nodes, err := kubeClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	require.NoError(t, err)
	t.Logf("%d nodes", len(nodes.Items))

	pods, err := kubeClient.CoreV1().Pods("kube-system").List(ctx, metav1.ListOptions{})
	require.NoError(t, err)
	csiPods := 0
	for _, pod := range pods.Items {
		if strings.HasPrefix(pod.Name, "ebs-csi") {
			csiPods++
			t.Logf("  %s: %s", pod.Name, pod.Status.Phase)
		}
	}
	assert.Greater(t, csiPods, 0, "the EBS CSI driver has no pods running")
}
