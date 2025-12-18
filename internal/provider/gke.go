package provider

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	container "cloud.google.com/go/container/apiv1"
	containerpb "cloud.google.com/go/container/apiv1/containerpb"
	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp"
	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/compute"
	gkecontainer "github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/container"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optdestroy"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optup"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	gcpiam "google.golang.org/api/iam/v1"
	gcpoption "google.golang.org/api/option"
	"gopkg.in/ini.v1"
	"gorm.io/datatypes"

	kube "github.com/threeport/threeport/pkg/kube/v0"
)

// GCP OAuth2 configuration for Application Default Credentials
// These are the public client credentials used by gcloud CLI for user authentication
// See: https://cloud.google.com/sdk/docs/authorizing
const (
	gcpOAuthClientID     = "764086051850-6qr4p6gpi6hn506pt8ejuq83di341hur.apps.googleusercontent.com"
	gcpOAuthClientSecret = "d-FL95Q19q7MQmFpd7hHD0Ty"
)

// gcpOAuthScopes defines the scopes needed for GKE operations
var gcpOAuthScopes = []string{
	"https://www.googleapis.com/auth/cloud-platform",
	"https://www.googleapis.com/auth/userinfo.email",
}

// adcCredentials represents the structure of the Application Default Credentials file
type adcCredentials struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RefreshToken string `json:"refresh_token"`
	Type         string `json:"type"`
}

// KubernetesRuntimeInfraGKE represents the infrastructure for a threeport-managed
// GKE (Google Kubernetes Engine) cluster.
type KubernetesRuntimeInfraGKE struct {
	// The unique name of the kubernetes runtime instance managed by threeport.
	RuntimeInstanceName string

	// Kubernetes version of the GKE cluster.
	Version string

	// The Google Cloud project ID where the cluster infra is provisioned.
	ProjectID string

	// The Google Cloud region where the cluster infra is provisioned.
	Region string

	// The number of nodes initially created for the worker node pool.
	WorkerNodeInitialCount int32

	// The path to the Pulumi state directory
	stateDir string

	// The email address of the GCP service account created for Threeport.
	// This service account is used with Workload Identity to allow
	// Threeport controllers to manage GCP resources.
	ServiceAccountEmail string
}

// Create installs a Kubernetes cluster using Google Cloud GKE for threeport workloads.
func (i *KubernetesRuntimeInfraGKE) Create() (*kube.KubeConnectionInfo, error) {
	// ensure GCP authentication is in place
	if err := EnsureGCPAuth(); err != nil {
		return nil, fmt.Errorf("failed to ensure GCP authentication: %w", err)
	}

	// load GCP configuration to ensure ProjectID is set
	if err := i.loadGCPConfig(); err != nil {
		return nil, fmt.Errorf("failed to load GCP configuration: %w", err)
	}

	// create GCP service account for Threeport to manage GCP resources
	// This service account will be used with Workload Identity
	if err := i.createGCPServiceAccountAndCredentials(); err != nil {
		return nil, fmt.Errorf("failed to create GCP service account: %w", err)
	}

	// set up Pulumi workspace and get stack
	stack, err := i.setupPulumiWorkspace(func(ctx *pulumi.Context) error {

		// create GCP provider with explicit configuration
		gcpProvider, err := gcp.NewProvider(ctx, "gcp-provider", &gcp.ProviderArgs{
			Project: pulumi.String(i.ProjectID),
			Region:  pulumi.String(i.Region),
		})
		if err != nil {
			return fmt.Errorf("failed to create GCP provider: %w", err)
		}

		// create VPC network for the cluster
		network, err := compute.NewNetwork(ctx, fmt.Sprintf("%s-vpc", i.RuntimeInstanceName), &compute.NetworkArgs{
			Name:                  pulumi.String(fmt.Sprintf("%s-vpc", i.RuntimeInstanceName)),
			AutoCreateSubnetworks: pulumi.Bool(false),
			Description:           pulumi.String(fmt.Sprintf("VPC network for Threeport GKE cluster %s", i.RuntimeInstanceName)),
		}, pulumi.Provider(gcpProvider))
		if err != nil {
			return fmt.Errorf("failed to create VPC network: %w", err)
		}

		// create subnet for the cluster
		subnet, err := compute.NewSubnetwork(ctx, fmt.Sprintf("%s-subnet", i.RuntimeInstanceName), &compute.SubnetworkArgs{
			Name:        pulumi.String(fmt.Sprintf("%s-subnet", i.RuntimeInstanceName)),
			IpCidrRange: pulumi.String("10.0.0.0/16"),
			Region:      pulumi.String(i.Region),
			Network:     network.ID(),
			SecondaryIpRanges: compute.SubnetworkSecondaryIpRangeArray{
				&compute.SubnetworkSecondaryIpRangeArgs{
					RangeName:   pulumi.String("pods"),
					IpCidrRange: pulumi.String("10.1.0.0/16"),
				},
				&compute.SubnetworkSecondaryIpRangeArgs{
					RangeName:   pulumi.String("services"),
					IpCidrRange: pulumi.String("10.2.0.0/20"),
				},
			},
		}, pulumi.Provider(gcpProvider),
			pulumi.DependsOn([]pulumi.Resource{network}))
		if err != nil {
			return fmt.Errorf("failed to create subnet: %w", err)
		}

		// TODO: Add firewall rules for more restrictive network access
		// For now, GKE will use default firewall rules

		// Create Cloud Router for Cloud NAT (required for private nodes to access internet)
		router, err := compute.NewRouter(ctx, fmt.Sprintf("%s-router", i.RuntimeInstanceName), &compute.RouterArgs{
			Name:    pulumi.String(fmt.Sprintf("%s-router", i.RuntimeInstanceName)),
			Network: network.ID(),
			Region:  pulumi.String(i.Region),
		}, pulumi.Provider(gcpProvider),
			pulumi.DependsOn([]pulumi.Resource{network}))
		if err != nil {
			return fmt.Errorf("failed to create Cloud Router: %w", err)
		}

		// Create Cloud NAT for outbound internet access from private nodes
		_, err = compute.NewRouterNat(ctx, fmt.Sprintf("%s-nat", i.RuntimeInstanceName), &compute.RouterNatArgs{
			Name:                          pulumi.String(fmt.Sprintf("%s-nat", i.RuntimeInstanceName)),
			Router:                        router.Name,
			Region:                        pulumi.String(i.Region),
			NatIpAllocateOption:           pulumi.String("AUTO_ONLY"),
			SourceSubnetworkIpRangesToNat: pulumi.String("ALL_SUBNETWORKS_ALL_IP_RANGES"),
			LogConfig: &compute.RouterNatLogConfigArgs{
				Enable: pulumi.Bool(true),
				Filter: pulumi.String("ERRORS_ONLY"),
			},
		}, pulumi.Provider(gcpProvider),
			pulumi.DependsOn([]pulumi.Resource{router}))
		if err != nil {
			return fmt.Errorf("failed to create Cloud NAT: %w", err)
		}

		// Create GKE cluster in Standard mode with separately managed node pools.
		// This pattern creates a cluster, removes the default node pool, and uses
		// explicitly defined node pools for more control over node configuration.
		cluster, err := gkecontainer.NewCluster(ctx, i.RuntimeInstanceName, &gkecontainer.ClusterArgs{
			Name:       pulumi.String(i.RuntimeInstanceName),
			Location:   pulumi.String(i.Region),
			Network:    network.Name,
			Subnetwork: subnet.Name,

			// Use VPC-native (alias IP) mode for better pod networking
			IpAllocationPolicy: &gkecontainer.ClusterIpAllocationPolicyArgs{
				ClusterSecondaryRangeName:  pulumi.String("pods"),
				ServicesSecondaryRangeName: pulumi.String("services"),
			},

			// InitialNodeCount is required by GKE API but the default node pool
			// is immediately removed so we can use separately managed node pools.
			InitialNodeCount:      pulumi.Int(1),
			RemoveDefaultNodePool: pulumi.Bool(true),

			// Private cluster configuration - nodes have no public IPs but
			// the API server remains publicly accessible.
			PrivateClusterConfig: &gkecontainer.ClusterPrivateClusterConfigArgs{
				EnablePrivateNodes:    pulumi.Bool(true),
				EnablePrivateEndpoint: pulumi.Bool(false), // Keep API server publicly accessible
				MasterIpv4CidrBlock:   pulumi.String("172.16.0.0/28"),
			},

			// Enable Kubernetes NetworkPolicy enforcement using Calico
			NetworkPolicy: &gkecontainer.ClusterNetworkPolicyArgs{
				Enabled:  pulumi.Bool(true),
				Provider: pulumi.String("CALICO"),
			},

			// Enable Workload Identity - GKE's recommended way to authenticate
			// workloads to other GCP services. This allows Kubernetes service
			// accounts to act as GCP service accounts.
			WorkloadIdentityConfig: &gkecontainer.ClusterWorkloadIdentityConfigArgs{
				WorkloadPool: pulumi.String(fmt.Sprintf("%s.svc.id.goog", i.ProjectID)),
			},

			// TODO: Cluster config options to consider:
			// - Master authorized networks.  This is a  GKE feature for restricting IP addresses that can access the Kubernetes API server.
			// - Binary authorization.  Ensure only trusted images can be deployed to the cluster.

			// Enable deletion protection in production
			// TODO: Make this configurable via 'tier' flag with tptctl up
			DeletionProtection: pulumi.Bool(false),
		}, pulumi.Provider(gcpProvider),
			pulumi.DependsOn([]pulumi.Resource{subnet}))
		if err != nil {
			return fmt.Errorf("failed to create GKE cluster: %w", err)
		}

		// create a separately managed node pool with Workload Identity enabled
		// This gives us more control over node configuration
		_, err = gkecontainer.NewNodePool(ctx, fmt.Sprintf("%s-nodepool", i.RuntimeInstanceName), &gkecontainer.NodePoolArgs{
			Name:     pulumi.String(fmt.Sprintf("%s-nodepool", i.RuntimeInstanceName)),
			Cluster:  cluster.Name,
			Location: pulumi.String(i.Region),

			NodeCount: pulumi.Int(int(i.WorkerNodeInitialCount)),

			NodeConfig: &gkecontainer.NodePoolNodeConfigArgs{
				// TODO: Make machine type configurable, perhaps via the 'tier' flag with tptctl up
				MachineType: pulumi.String("e2-medium"),

				// TODO: Make disk size and type configurable, perhaps via the 'tier' flag with tptctl up
				DiskSizeGb: pulumi.Int(50),
				DiskType:   pulumi.String("pd-standard"),

				// OAuth scopes for the node - with Workload Identity, nodes need
				// minimal scopes as workloads use their own service accounts
				OauthScopes: pulumi.StringArray{
					pulumi.String("https://www.googleapis.com/auth/cloud-platform"),
				},

				Labels: pulumi.StringMap{
					kube.ThreeportManagedByLabelKey: pulumi.String(kube.ThreeportManagedByLabelValue),
				},

				// Enable Workload Identity on this node pool
				// GKE_METADATA enables the GKE metadata server for Workload Identity
				WorkloadMetadataConfig: &gkecontainer.NodePoolNodeConfigWorkloadMetadataConfigArgs{
					Mode: pulumi.String("GKE_METADATA"),
				},

				// TODO: Add taints, metadata, and other node config options as needed
			},

			Autoscaling: &gkecontainer.NodePoolAutoscalingArgs{
				MinNodeCount:   pulumi.Int(1),
				MaxNodeCount:   pulumi.Int(10),       // TODO: Make this configurable, perhaps via the 'tier' flag with tptctl up
				LocationPolicy: pulumi.String("ANY"), // Allow nodes to be placed in any zone
			},

			Management: &gkecontainer.NodePoolManagementArgs{
				AutoRepair:  pulumi.Bool(true),
				AutoUpgrade: pulumi.Bool(true),
			},
		}, pulumi.Provider(gcpProvider),
			pulumi.DependsOn([]pulumi.Resource{cluster}))
		if err != nil {
			return fmt.Errorf("failed to create node pool: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to set up Pulumi workspace: %w", err)
	}

	// create a context for the automation API
	ctx := context.Background()

	// deploy the stack
	_, err = stack.Up(ctx, optup.ProgressStreams(os.Stdout))
	if err != nil {
		return nil, fmt.Errorf("failed to deploy stack: %w", err)
	}

	// configure Workload Identity binding after cluster is created
	// This allows Kubernetes service accounts to impersonate the GCP service account
	if err := i.configureWorkloadIdentityBindingPostCreate(); err != nil {
		return nil, fmt.Errorf("failed to configure Workload Identity binding: %w", err)
	}

	return i.GetConnection()
}

// configureWorkloadIdentityBindingPostCreate sets up the Workload Identity binding
// after the GKE cluster has been created.
func (i *KubernetesRuntimeInfraGKE) configureWorkloadIdentityBindingPostCreate() error {
	ctx := context.Background()

	// Create IAM service client
	iamService, err := gcpiam.NewService(ctx, gcpoption.WithScopes(gcpiam.CloudPlatformScope))
	if err != nil {
		return fmt.Errorf("failed to create IAM service client: %w", err)
	}

	return i.configureWorkloadIdentityBinding(iamService)
}

// Delete deletes a GKE cluster and the threeport control plane with it.
func (i *KubernetesRuntimeInfraGKE) Delete() error {
	// ensure GCP authentication is in place
	if err := EnsureGCPAuth(); err != nil {
		return fmt.Errorf("failed to ensure GCP authentication: %w", err)
	}

	// set up Pulumi workspace and get stack
	stack, err := i.setupPulumiWorkspace(func(ctx *pulumi.Context) error {
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to set up Pulumi workspace: %w", err)
	}

	// create a context for the automation API
	ctx := context.Background()

	// destroy the stack
	_, err = stack.Destroy(ctx, optdestroy.ProgressStreams(os.Stdout))
	if err != nil {
		return fmt.Errorf("failed to destroy stack: %w", err)
	}

	// remove the state directory after successful destruction
	if err := os.RemoveAll(i.stateDir); err != nil {
		return fmt.Errorf("failed to remove state directory: %w", err)
	}

	// clean up GCP resources (service account, IAM bindings)
	if err := i.DeleteGCPResources(); err != nil {
		// Log warning but don't fail - the cluster is already deleted
		fmt.Printf("Warning: failed to clean up GCP resources: %v\n", err)
	}

	return nil
}

// GetConnection returns the connection information for the GKE cluster.
func (i *KubernetesRuntimeInfraGKE) GetConnection() (*kube.KubeConnectionInfo, error) {
	// ensure GCP authentication is in place
	if err := EnsureGCPAuth(); err != nil {
		return nil, fmt.Errorf("failed to ensure GCP authentication: %w", err)
	}

	// load GCP configuration from gcloud CLI config or environment variables
	if err := i.loadGCPConfig(); err != nil {
		return nil, fmt.Errorf("failed to load GCP configuration: %w", err)
	}

	ctx := context.Background()

	// create GKE cluster manager client
	// This uses Application Default Credentials (ADC)
	clusterManagerClient, err := container.NewClusterManagerClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create cluster manager client: %w", err)
	}
	defer clusterManagerClient.Close()

	// construct the cluster name in the format required by GKE API
	// Format: projects/{project}/locations/{location}/clusters/{cluster}
	clusterName := fmt.Sprintf(
		"projects/%s/locations/%s/clusters/%s",
		i.ProjectID, i.Region, i.RuntimeInstanceName,
	)

	// get cluster details
	cluster, err := clusterManagerClient.GetCluster(ctx, &containerpb.GetClusterRequest{
		Name: clusterName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster details: %w", err)
	}

	// decode the CA certificate
	caCert, err := base64.StdEncoding.DecodeString(cluster.MasterAuth.ClusterCaCertificate)
	if err != nil {
		return nil, fmt.Errorf("failed to decode CA certificate: %w", err)
	}

	// get an access token for authentication
	// TODO: Implement token refresh mechanism for long-running operations
	// The token has a limited lifetime (typically 1 hour)
	tokenSource, err := google.DefaultTokenSource(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return nil, fmt.Errorf("failed to get token source: %w", err)
	}

	token, err := tokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("failed to get access token: %w", err)
	}

	// construct the API endpoint
	// GKE provides the endpoint without the https:// prefix
	apiEndpoint := fmt.Sprintf("https://%s", cluster.Endpoint)

	// create connection info
	kubeConnInfo := &kube.KubeConnectionInfo{
		APIEndpoint:     apiEndpoint,
		CACertificate:   string(caCert),
		Token:           token.AccessToken,
		TokenExpiration: token.Expiry,
	}

	return kubeConnInfo, nil
}

// EnsureGCPAuth checks for valid GCP Application Default Credentials and initiates
// the OAuth flow if credentials are missing or invalid. This allows users to
// authenticate without manually running `gcloud auth application-default login`.
func EnsureGCPAuth() error {
	ctx := context.Background()

	// first, check if valid credentials already exist
	if hasValidGCPCredentials(ctx) {
		return nil
	}

	fmt.Println("GCP credentials not found or expired. Initiating authentication...")

	// perform the OAuth flow
	if err := performGCPOAuthFlow(ctx); err != nil {
		return fmt.Errorf("failed to authenticate with GCP: %w", err)
	}

	fmt.Println("GCP authentication successful!")
	return nil
}

// LoadConfigFromStack loads the GCP project ID and region from the existing
// Pulumi stack configuration. This is useful when deleting a cluster where
// we only have the runtime instance name but need the project and region
// to connect to GCP.
func (i *KubernetesRuntimeInfraGKE) LoadConfigFromStack() error {
	// set up state directory
	if err := i.setStateDir(); err != nil {
		return fmt.Errorf("failed to set state directory: %w", err)
	}

	// set environment variables for Pulumi configuration
	if err := i.setPulumiEnvVars(); err != nil {
		return fmt.Errorf("failed to set Pulumi environment variables: %w", err)
	}

	ctx := context.Background()

	// create a new workspace with local state backend
	workspace, err := auto.NewLocalWorkspace(
		ctx,
		auto.Program(func(ctx *pulumi.Context) error { return nil }),
		auto.WorkDir(i.stateDir),
	)
	if err != nil {
		return fmt.Errorf("failed to create workspace: %w", err)
	}

	// select the existing stack
	stack, err := auto.SelectStack(ctx, i.getStackName(), workspace)
	if err != nil {
		return fmt.Errorf("failed to select stack: %w", err)
	}

	// get the project configuration
	projectConfig, err := stack.GetConfig(ctx, "gcp:project")
	if err != nil {
		return fmt.Errorf("failed to get gcp:project config: %w", err)
	}
	i.ProjectID = projectConfig.Value

	// get the region configuration
	regionConfig, err := stack.GetConfig(ctx, "gcp:region")
	if err != nil {
		return fmt.Errorf("failed to get gcp:region config: %w", err)
	}
	i.Region = regionConfig.Value

	return nil
}

// GetStackState returns the state of the GKE stack as a JSON object
func (i *KubernetesRuntimeInfraGKE) GetStackState() (*datatypes.JSON, error) {

	// set up state directory
	if err := i.setStateDir(); err != nil {
		return nil, fmt.Errorf("failed to set state directory: %w", err)
	}

	// set environment variables for Pulumi configuration
	if err := i.setPulumiEnvVars(); err != nil {
		return nil, fmt.Errorf("failed to set Pulumi environment variables: %w", err)
	}

	ctx := context.Background()

	// create a new workspace with local state backend
	workspace, err := auto.NewLocalWorkspace(
		ctx,
		auto.WorkDir(i.stateDir),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create workspace: %w", err)
	}

	// load stack from workspace
	stack, err := auto.SelectStack(ctx, i.getStackName(), workspace)
	if err != nil {
		return nil, fmt.Errorf("failed to select stack: %w", err)
	}

	// get the stack's state
	state, err := stack.Export(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to export stack state: %w", err)
	}

	// convert state to JSON
	stateJSON, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal state to JSON: %w", err)
	}

	jsonState := datatypes.JSON(stateJSON)
	return &jsonState, nil
}

// SetStackState sets the state of the GKE stack from a JSON object
func (i *KubernetesRuntimeInfraGKE) SetStackState(state *datatypes.JSON) error {

	// set up state directory
	if err := i.setStateDir(); err != nil {
		return fmt.Errorf("failed to set state directory: %w", err)
	}

	// set environment variables for Pulumi configuration
	if err := i.setPulumiEnvVars(); err != nil {
		return fmt.Errorf("failed to set Pulumi environment variables: %w", err)
	}

	ctx := context.Background()

	// create a new workspace with local state backend
	workspace, err := auto.NewLocalWorkspace(
		ctx,
		auto.WorkDir(i.stateDir),
	)
	if err != nil {
		return fmt.Errorf("failed to create workspace: %w", err)
	}

	// create/select stack
	stack, err := auto.UpsertStack(ctx, i.getStackName(), workspace)
	if err != nil {
		return fmt.Errorf("failed to create/select stack: %w", err)
	}

	// unmarshal state
	var pulumiState apitype.UntypedDeployment
	err = json.Unmarshal(*state, &pulumiState)
	if err != nil {
		return fmt.Errorf("failed to unmarshal state from JSON: %w", err)
	}

	// set the stack's state and persist to disk
	err = stack.Import(ctx, pulumiState)
	if err != nil {
		return fmt.Errorf("failed to import stack state: %w", err)
	}

	return nil
}

// hasValidGCPCredentials checks if valid Application Default Credentials exist
func hasValidGCPCredentials(ctx context.Context) bool {
	// try to get a token using the default credentials
	tokenSource, err := google.DefaultTokenSource(ctx, gcpOAuthScopes...)
	if err != nil {
		return false
	}

	// try to get a token to verify the credentials are valid
	token, err := tokenSource.Token()
	if err != nil {
		return false
	}

	// check if the token is valid and not expired
	return token.Valid()
}

// performGCPOAuthFlow performs the browser-based OAuth flow for GCP authentication
func performGCPOAuthFlow(ctx context.Context) error {
	// create a random state for CSRF protection
	state, err := generateRandomState()
	if err != nil {
		return fmt.Errorf("failed to generate state: %w", err)
	}

	// find an available port for the callback server
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return fmt.Errorf("failed to create listener: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	redirectURL := fmt.Sprintf("http://localhost:%d/callback", port)

	// create OAuth2 config
	oauth2Config := &oauth2.Config{
		ClientID:     gcpOAuthClientID,
		ClientSecret: gcpOAuthClientSecret,
		Endpoint:     google.Endpoint,
		RedirectURL:  redirectURL,
		Scopes:       gcpOAuthScopes,
	}

	// channel to receive the authorization code
	codeChan := make(chan string, 1)
	errChan := make(chan error, 1)

	// create HTTP server to handle the callback
	server := &http.Server{}
	http.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		// verify state parameter
		if r.URL.Query().Get("state") != state {
			errChan <- fmt.Errorf("invalid state parameter")
			http.Error(w, "Invalid state parameter", http.StatusBadRequest)
			return
		}

		// check for errors
		if errMsg := r.URL.Query().Get("error"); errMsg != "" {
			errChan <- fmt.Errorf("OAuth error: %s - %s", errMsg, r.URL.Query().Get("error_description"))
			http.Error(w, "Authentication failed", http.StatusBadRequest)
			return
		}

		// get the authorization code
		code := r.URL.Query().Get("code")
		if code == "" {
			errChan <- fmt.Errorf("no authorization code received")
			http.Error(w, "No authorization code received", http.StatusBadRequest)
			return
		}

		// send success response to browser
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<html><body><h1>Authentication Successful!</h1><p>You can close this window and return to the terminal.</p></body></html>`)

		codeChan <- code
	})

	// start the server
	go func() {
		if err := server.Serve(listener); err != http.ErrServerClosed {
			errChan <- fmt.Errorf("callback server error: %w", err)
		}
	}()

	// generate the authorization URL
	authURL := oauth2Config.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)

	fmt.Println("\nOpening browser for GCP authentication...")
	fmt.Println("If the browser doesn't open automatically, please visit:")
	fmt.Println(authURL)
	fmt.Println()

	// try to open the browser
	if err := openBrowser(authURL); err != nil {
		fmt.Println("Failed to open browser automatically. Please open the URL above manually.")
	}

	// wait for the authorization code or error
	var code string
	select {
	case code = <-codeChan:
		// success
	case err := <-errChan:
		server.Shutdown(ctx)
		return err
	case <-time.After(5 * time.Minute):
		server.Shutdown(ctx)
		return fmt.Errorf("authentication timed out after 5 minutes")
	}

	// shutdown the callback server
	server.Shutdown(ctx)

	// exchange the authorization code for tokens
	token, err := oauth2Config.Exchange(ctx, code)
	if err != nil {
		return fmt.Errorf("failed to exchange authorization code: %w", err)
	}

	// save the credentials to the ADC file
	if err := saveADCCredentials(token); err != nil {
		return fmt.Errorf("failed to save credentials: %w", err)
	}

	return nil
}

// saveADCCredentials saves the OAuth2 token as Application Default Credentials
func saveADCCredentials(token *oauth2.Token) error {
	// get the ADC file path
	adcPath, err := getADCPath()
	if err != nil {
		return err
	}

	// ensure the directory exists
	adcDir := filepath.Dir(adcPath)
	if err := os.MkdirAll(adcDir, 0700); err != nil {
		return fmt.Errorf("failed to create ADC directory: %w", err)
	}

	// create the credentials structure
	creds := adcCredentials{
		ClientID:     gcpOAuthClientID,
		ClientSecret: gcpOAuthClientSecret,
		RefreshToken: token.RefreshToken,
		Type:         "authorized_user",
	}

	// marshal to JSON
	credsJSON, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal credentials: %w", err)
	}

	// write to file
	if err := os.WriteFile(adcPath, credsJSON, 0600); err != nil {
		return fmt.Errorf("failed to write ADC file: %w", err)
	}

	return nil
}

// getADCPath returns the path to the Application Default Credentials file
func getADCPath() (string, error) {
	// check for GOOGLE_APPLICATION_CREDENTIALS environment variable
	if path := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"); path != "" {
		return path, nil
	}

	// use the default location
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	// on Windows, the path is different
	if runtime.GOOS == "windows" {
		return filepath.Join(homeDir, "AppData", "Roaming", "gcloud", "application_default_credentials.json"), nil
	}

	return filepath.Join(homeDir, ".config", "gcloud", "application_default_credentials.json"), nil
}

// generateRandomState generates a random state string for CSRF protection
func generateRandomState() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// openBrowser opens the specified URL in the default browser
func openBrowser(url string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		// try xdg-open first, then fall back to other options
		if _, err := exec.LookPath("xdg-open"); err == nil {
			cmd = exec.Command("xdg-open", url)
		} else if _, err := exec.LookPath("gnome-open"); err == nil {
			cmd = exec.Command("gnome-open", url)
		} else if _, err := exec.LookPath("kde-open"); err == nil {
			cmd = exec.Command("kde-open", url)
		} else {
			return fmt.Errorf("no browser opener found")
		}
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", strings.ReplaceAll(url, "&", "^&"))
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	return cmd.Start()
}

// setupPulumiWorkspace sets up the Pulumi workspace and environment for GKE operations
func (i *KubernetesRuntimeInfraGKE) setupPulumiWorkspace(program pulumi.RunFunc) (auto.Stack, error) {
	// set up state directory
	if err := i.setStateDir(); err != nil {
		return auto.Stack{}, fmt.Errorf("failed to set state directory: %w", err)
	}

	// set environment variables for Pulumi configuration
	if err := i.setPulumiEnvVars(); err != nil {
		return auto.Stack{}, fmt.Errorf("failed to set Pulumi environment variables: %w", err)
	}

	// load GCP configuration from gcloud CLI config or environment variables
	if err := i.loadGCPConfig(); err != nil {
		return auto.Stack{}, fmt.Errorf("failed to load GCP configuration: %w", err)
	}

	// create Pulumi.yaml project file
	pulumiYaml := `name: gke
runtime: go
description: Google Kubernetes Engine (GKE) cluster for Threeport
`
	pulumiYamlPath := filepath.Join(i.stateDir, "Pulumi.yaml")
	if err := os.WriteFile(pulumiYamlPath, []byte(pulumiYaml), 0644); err != nil {
		return auto.Stack{}, fmt.Errorf("failed to create Pulumi.yaml: %w", err)
	}

	ctx := context.Background()

	// create a new workspace with local state backend
	workspace, err := auto.NewLocalWorkspace(
		ctx,
		auto.Program(program),
		auto.WorkDir(i.stateDir),
	)
	if err != nil {
		return auto.Stack{}, fmt.Errorf("failed to create workspace: %w", err)
	}

	// create or select a stack with fully qualified name
	stack, err := auto.UpsertStack(ctx, i.getStackName(), workspace)
	if err != nil {
		return auto.Stack{}, fmt.Errorf("failed to create/select stack: %w", err)
	}

	// set up stack configuration
	err = stack.SetConfig(ctx, "gcp:project", auto.ConfigValue{Value: i.ProjectID})
	if err != nil {
		return auto.Stack{}, fmt.Errorf("failed to set project config: %w", err)
	}

	err = stack.SetConfig(ctx, "gcp:region", auto.ConfigValue{Value: i.Region})
	if err != nil {
		return auto.Stack{}, fmt.Errorf("failed to set region config: %w", err)
	}

	return stack, nil
}

// setPulumiEnvVars sets the environment variables for Pulumi
func (i *KubernetesRuntimeInfraGKE) setPulumiEnvVars() error {
	os.Setenv("PULUMI_BACKEND_URL", "file://"+i.stateDir)
	os.Setenv("PULUMI_HOME", i.stateDir)
	os.Setenv("PULUMI_ORGANIZATION", "organization") // TODO: make this configurable
	os.Setenv("PULUMI_PROJECT", "gke")
	os.Setenv("PULUMI_CONFIG_PASSPHRASE", "threeport")

	// set plugin path to the default location
	userHomeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}
	defaultPluginPath := filepath.Join(userHomeDir, ".pulumi", "plugins")
	os.Setenv("PULUMI_PLUGIN_PATH", defaultPluginPath)

	return nil
}

// setStateDir sets the state directory for the GKE stack
func (i *KubernetesRuntimeInfraGKE) setStateDir() error {
	pulumiStateDir, err := GetPulumiStateDir()
	if err != nil {
		return fmt.Errorf("failed to get pulumi state directory: %w", err)
	}

	i.stateDir = filepath.Join(pulumiStateDir, i.RuntimeInstanceName)

	// ensure state directory exists
	if err := os.MkdirAll(i.stateDir, 0755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	return nil
}

// getStackName returns the name of the GKE stack
func (i *KubernetesRuntimeInfraGKE) getStackName() string {
	return fmt.Sprintf("organization/gke/%s", i.RuntimeInstanceName)
}

// loadGCPConfig reads the GCP configuration from gcloud CLI config files
// or environment variables and populates KubernetesRuntimeInfraGKE struct fields.
// It follows a similar pattern to loadOCIConfig in OKE.
func (i *KubernetesRuntimeInfraGKE) loadGCPConfig() error {
	// first check environment variables
	projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if projectID == "" {
		projectID = os.Getenv("CLOUDSDK_CORE_PROJECT")
	}
	if projectID == "" {
		projectID = os.Getenv("GCLOUD_PROJECT")
	}

	region := os.Getenv("CLOUDSDK_COMPUTE_REGION")
	if region == "" {
		region = os.Getenv("GOOGLE_REGION")
	}

	// if environment variables provided the values, use them
	if projectID != "" && i.ProjectID == "" {
		i.ProjectID = projectID
	}
	if region != "" && i.Region == "" {
		i.Region = region
	}

	// if we already have both values, return early
	if i.ProjectID != "" && i.Region != "" {
		return nil
	}

	// try to read from gcloud configuration files
	if err := i.loadGCPConfigFromFile(); err != nil {
		// if we still don't have required values, return error
		if i.ProjectID == "" {
			return fmt.Errorf("GCP project ID not found in environment variables or gcloud config: %w", err)
		}
		if i.Region == "" {
			return fmt.Errorf("GCP region not found in environment variables or gcloud config: %w", err)
		}
	}

	return nil
}

// loadGCPConfigFromFile reads the gcloud CLI configuration files
// to get project ID and region settings.
func (i *KubernetesRuntimeInfraGKE) loadGCPConfigFromFile() error {
	// get user's home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	// gcloud config directory path
	var gcloudConfigDir string
	if runtime.GOOS == "windows" {
		gcloudConfigDir = filepath.Join(homeDir, "AppData", "Roaming", "gcloud")
	} else {
		gcloudConfigDir = filepath.Join(homeDir, ".config", "gcloud")
	}

	// try to determine the active configuration
	activeConfig := "default"
	activeConfigPath := filepath.Join(gcloudConfigDir, "active_config")
	if activeConfigData, err := os.ReadFile(activeConfigPath); err == nil {
		activeConfig = strings.TrimSpace(string(activeConfigData))
	}

	// try to read the configuration file for the active configuration
	configFilePath := filepath.Join(gcloudConfigDir, "configurations", fmt.Sprintf("config_%s", activeConfig))

	// check if config file exists
	if _, err := os.Stat(configFilePath); os.IsNotExist(err) {
		// fall back to properties file
		configFilePath = filepath.Join(gcloudConfigDir, "properties")
		if _, err := os.Stat(configFilePath); os.IsNotExist(err) {
			return fmt.Errorf("gcloud config file not found at %s or properties file", configFilePath)
		}
	}

	// load the configuration file (INI format)
	cfg, err := ini.Load(configFilePath)
	if err != nil {
		return fmt.Errorf("failed to read gcloud config file: %w", err)
	}

	// get project ID from [core] section
	if i.ProjectID == "" {
		projectID := cfg.Section("core").Key("project").String()
		if projectID != "" {
			i.ProjectID = projectID
		}
	}

	// get region from [compute] section
	if i.Region == "" {
		region := cfg.Section("compute").Key("region").String()
		if region != "" {
			i.Region = region
		}
	}

	return nil
}
