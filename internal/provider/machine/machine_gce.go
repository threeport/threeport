// Package machine provisions a Google Compute Engine VM for a Threeport
// machine runtime through Pulumi. The stack creates an SSH firewall rule
// and an instance with an ephemeral public IP, then captures hostname and
// NAT IP as outputs. SSH keys are generated in process. The public key is
// written to instance metadata. The private key is not a stack output.
package machine

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp"
	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/compute"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"golang.org/x/crypto/ssh"
	"gorm.io/datatypes"

	"github.com/threeport/threeport/internal/provider"
	gcpauth "github.com/threeport/threeport/pkg/auth/v0"
)

// compile-time check that GceMachineInfra satisfies InfraProvider.
// StreamableProvider and RefreshableProvider come from the embedded workspace.
var (
	_ provider.InfraProvider       = (*GceMachineInfra)(nil)
	_ provider.StreamableProvider  = (*GceMachineInfra)(nil)
	_ provider.RefreshableProvider = (*GceMachineInfra)(nil)
)

// gceNameRe is GCE's RFC1035 instance-name pattern, 1 to 59 characters.
// The SSH firewall name is {name}-ssh and is also capped at 63.
var gceNameRe = regexp.MustCompile(`^[a-z]([-a-z0-9]{0,57}[a-z0-9])?$`)

// GceMachineInfra is the Google Compute Engine backend for a machine runtime.
// Machine-runtime constructs this with NewGceMachineInfra() and calls DeployInfra().
type GceMachineInfra struct {
	provider.PulumiWorkspace

	// The Google Cloud project ID where the VM is provisioned
	ProjectID string

	// The Google Cloud region written to Pulumi stack config
	Region string

	// The Google Cloud zone where the VM is created
	Zone string

	// The GCE machine type, e.g. e2-medium
	MachineType string

	// The boot disk image, e.g. debian-cloud/debian-12
	ImageID string

	// The VPC network self-link or name the instance attaches to
	NetworkID string

	// The JSON key for a GCP service account when ADC is not already valid
	ServiceAccountCredentials string

	// The Linux user that receives the generated SSH public key
	SSHUser string

	// The CIDR ranges allowed to reach TCP 22; required, no open default
	SSHSourceRanges []string

	// The function that persists the generated SSH private key before Pulumi up
	PersistSSHKey func(privateKeyPEM string) error

	// The generated RSA private key in PEM form
	sshPrivateKeyPEM string

	// The generated SSH public key in authorized_keys form
	sshPublicKeyAuthorized string

	// The instance name exported from the stack
	hostname string

	// The ephemeral public IPv4 exported from the stack
	externalIP string
}

// NewGceMachineInfra returns a GCE machine provider for the named runtime instance.
func NewGceMachineInfra(name string, opts ...provider.PulumiWorkspaceOption) *GceMachineInfra {
	return &GceMachineInfra{
		PulumiWorkspace: *provider.NewPulumiWorkspace(name, "gce", opts...),
	}
}

// ensurePulumiProjectDefaults sets Pulumi project metadata when not provided by callers.
func (i *GceMachineInfra) ensurePulumiProjectDefaults() {
	// set project name and description when callers left them empty
	if i.ProjectName == "" {
		i.ProjectName = "gce"
	}
	if i.ProjectDescription == "" {
		i.ProjectDescription = "Google Compute Engine VM for Threeport"
	}
}

// gcpRegion returns Region, or the zone prefix when Region is empty.
func (i *GceMachineInfra) gcpRegion() string {
	if i.Region != "" {
		return i.Region
	}
	if idx := strings.LastIndex(i.Zone, "-"); idx > 0 {
		return i.Zone[:idx]
	}
	return ""
}

// syncStackConfigs updates stack config keys from the current ProjectID and Region.
func (i *GceMachineInfra) syncStackConfigs() {
	i.StackConfigs = map[string]string{
		"gcp:project": i.ProjectID,
		"gcp:region":  i.gcpRegion(),
	}
}

// sshSourceRanges returns the configured SSH CIDRs.
func (i *GceMachineInfra) sshSourceRanges() []string {
	return i.SSHSourceRanges
}

// validateRequiredFields reports every empty required field in one error.
func (i *GceMachineInfra) validateRequiredFields() error {
	// collect every empty required field
	var missing []string
	if i.RuntimeInstanceName == "" {
		missing = append(missing, "RuntimeInstanceName")
	} else if !gceNameRe.MatchString(i.RuntimeInstanceName) {
		return fmt.Errorf("RuntimeInstanceName %q is not a valid GCE instance name", i.RuntimeInstanceName)
	}
	if i.ProjectID == "" {
		missing = append(missing, "ProjectID")
	}
	if i.Zone == "" {
		missing = append(missing, "Zone")
	}
	if i.MachineType == "" {
		missing = append(missing, "MachineType")
	}
	if i.ImageID == "" {
		missing = append(missing, "ImageID")
	}
	if i.SSHUser == "" {
		missing = append(missing, "SSHUser")
	}
	if i.NetworkID == "" {
		missing = append(missing, "NetworkID")
	}
	if len(i.SSHSourceRanges) == 0 {
		missing = append(missing, "SSHSourceRanges")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required fields: %s", strings.Join(missing, ", "))
	}
	return nil
}

// DeployInfra creates the GCE VM and SSH firewall. It satisfies InfraProvider.
func (i *GceMachineInfra) DeployInfra() error {
	return i.createInfra()
}

// createInfra validates config, authenticates to GCP, generates SSH keys, and runs the stack.
func (i *GceMachineInfra) createInfra() error {
	// validate required fields
	if err := i.validateRequiredFields(); err != nil {
		return fmt.Errorf("invalid GCE machine configuration: %w", err)
	}

	// ensure GCP authentication is in place
	if err := gcpauth.EnsureGCPAuth(i.ServiceAccountCredentials); err != nil {
		return fmt.Errorf("failed to ensure GCP authentication: %w", err)
	}

	// generate SSH keys outside the Pulumi program so the program is deterministic
	if err := i.ensureSSHKeyPair(); err != nil {
		return fmt.Errorf("failed to ensure SSH key pair: %w", err)
	}

	// persist the private key before up so a retry can rehydrate it
	if err := i.persistGeneratedSSHKey(); err != nil {
		return fmt.Errorf("failed to persist ssh private key: %w", err)
	}

	// set Pulumi project defaults and stack config
	i.ensurePulumiProjectDefaults()
	i.syncStackConfigs()

	// set up Pulumi workspace and get stack
	stack, err := i.SetupStack(i.pulumiProgram())
	if err != nil {
		return fmt.Errorf("failed to set up Pulumi workspace: %w", err)
	}

	// deploy the stack
	upResult, err := i.RunUp(context.Background(), stack)
	if err != nil {
		return fmt.Errorf("failed to deploy stack: %w", err)
	}

	// capture hostname and external IP from stack outputs
	i.captureOutputs(upResult.Outputs)
	if i.hostname == "" || i.externalIP == "" {
		return errors.New("pulumi stack outputs missing hostname or externalIP")
	}

	return nil
}

// DestroyInfra tears down the GCE VM and SSH firewall. It satisfies InfraProvider.
func (i *GceMachineInfra) DestroyInfra() error {
	// ensure GCP authentication is in place
	if err := gcpauth.EnsureGCPAuth(i.ServiceAccountCredentials); err != nil {
		return fmt.Errorf("failed to ensure GCP authentication: %w", err)
	}

	// set Pulumi project defaults and stack config
	i.ensurePulumiProjectDefaults()
	i.syncStackConfigs()

	// destroy the Pulumi stack
	if err := i.DestroyStack(); err != nil {
		return fmt.Errorf("failed to destroy Pulumi stack: %w", err)
	}

	return nil
}

// GetStackState returns the current stack state. It fills project defaults first.
func (i *GceMachineInfra) GetStackState() (*datatypes.JSON, error) {
	// fill project defaults and stack config before reading state
	i.ensurePulumiProjectDefaults()
	i.syncStackConfigs()
	return i.PulumiWorkspace.GetStackState()
}

// SetStackState restores stack state from JSON. It fills project defaults first.
func (i *GceMachineInfra) SetStackState(state *datatypes.JSON) error {
	// fill project defaults and stack config before writing state
	i.ensurePulumiProjectDefaults()
	i.syncStackConfigs()
	return i.PulumiWorkspace.SetStackState(state)
}

// pulumiProgram defines the Pulumi resources for the GCE VM stack.
func (i *GceMachineInfra) pulumiProgram() pulumi.RunFunc {
	return func(pctx *pulumi.Context) error {
		// create GCP provider
		gcpProvider, err := gcp.NewProvider(pctx, "gcp-provider", &gcp.ProviderArgs{
			Project: pulumi.String(i.ProjectID),
			Region:  pulumi.String(i.gcpRegion()),
		})
		if err != nil {
			return fmt.Errorf("failed to create GCP provider: %w", err)
		}

		// create SSH firewall rule scoped to this instance's network tag
		sshTag := i.RuntimeInstanceName
		sourceRanges := pulumi.ToStringArray(i.sshSourceRanges())
		_, err = compute.NewFirewall(pctx, fmt.Sprintf("%s-ssh", i.RuntimeInstanceName), &compute.FirewallArgs{
			Name:        pulumi.String(fmt.Sprintf("%s-ssh", i.RuntimeInstanceName)),
			Network:     pulumi.String(i.NetworkID),
			Description: pulumi.String(provider.GcpOwnershipDescription(i.RuntimeInstanceName)),
			Allows: compute.FirewallAllowArray{
				&compute.FirewallAllowArgs{
					Protocol: pulumi.String("tcp"),
					Ports:    pulumi.StringArray{pulumi.String("22")},
				},
			},
			SourceRanges: sourceRanges,
			TargetTags:   pulumi.StringArray{pulumi.String(sshTag)},
		}, pulumi.Provider(gcpProvider))
		if err != nil {
			return fmt.Errorf("failed to create SSH firewall rule: %w", err)
		}

		// create GCE instance with an ephemeral public IP
		instance, err := compute.NewInstance(pctx, i.RuntimeInstanceName, &compute.InstanceArgs{
			Name:        pulumi.String(i.RuntimeInstanceName),
			MachineType: pulumi.String(i.MachineType),
			Zone:        pulumi.String(i.Zone),
			BootDisk: &compute.InstanceBootDiskArgs{
				InitializeParams: &compute.InstanceBootDiskInitializeParamsArgs{
					Image: pulumi.String(i.ImageID),
				},
			},
			NetworkInterfaces: compute.InstanceNetworkInterfaceArray{
				&compute.InstanceNetworkInterfaceArgs{
					Network: pulumi.String(i.NetworkID),
					AccessConfigs: compute.InstanceNetworkInterfaceAccessConfigArray{
						&compute.InstanceNetworkInterfaceAccessConfigArgs{},
					},
				},
			},
			Metadata: pulumi.StringMap{
				"ssh-keys": pulumi.String(fmt.Sprintf(
					"%s:%s",
					i.SSHUser,
					// strip trailing newline from authorized_keys marshal
					strings.TrimSpace(i.sshPublicKeyAuthorized),
				)),
			},
			Tags:   pulumi.StringArray{pulumi.String(sshTag)},
			Labels: provider.GcpLabelsInput(i.RuntimeInstanceName),
		}, pulumi.Provider(gcpProvider))
		if err != nil {
			return fmt.Errorf("failed to create GCE instance: %w", err)
		}

		// export hostname and NAT IP; keep the private key off stack outputs
		pctx.Export("hostname", instance.Name)
		pctx.Export("externalIP", instance.NetworkInterfaces.
			Index(pulumi.Int(0)).
			AccessConfigs().
			Index(pulumi.Int(0)).
			NatIp())

		return nil
	}
}

// generateSSHKeyPair returns a PKCS1 PEM private key and an authorized_keys public key.
func generateSSHKeyPair() (privPEM, pubAuthorized string, err error) {
	// generate RSA key
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate RSA key: %w", err)
	}

	// encode private key to PEM
	privDER := x509.MarshalPKCS1PrivateKey(key)
	privPEMBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privDER,
	})
	if privPEMBytes == nil {
		return "", "", errors.New("failed to encode private key to PEM")
	}

	// marshal SSH public key
	pub, err := ssh.NewPublicKey(&key.PublicKey)
	if err != nil {
		return "", "", fmt.Errorf("failed to build SSH public key: %w", err)
	}
	pubAuthorizedBytes := ssh.MarshalAuthorizedKey(pub)

	return string(privPEMBytes), string(pubAuthorizedBytes), nil
}

// ensureSSHKeyPair keeps a restored private key or generates a new pair.
func (i *GceMachineInfra) ensureSSHKeyPair() error {
	// keep a restored private key; derive the public key when metadata is empty
	if i.sshPrivateKeyPEM != "" {
		if i.sshPublicKeyAuthorized == "" {
			pub, err := publicKeyFromPrivatePEM(i.sshPrivateKeyPEM)
			if err != nil {
				return err
			}
			i.sshPublicKeyAuthorized = pub
		}
		return nil
	}
	if i.sshPublicKeyAuthorized != "" {
		return nil
	}

	// generate a new pair
	priv, pub, err := generateSSHKeyPair()
	if err != nil {
		return err
	}
	i.sshPrivateKeyPEM = priv
	i.sshPublicKeyAuthorized = pub

	return nil
}

// publicKeyFromPrivatePEM returns the authorized_keys form of a PKCS1 PEM private key.
func publicKeyFromPrivatePEM(privPEM string) (string, error) {
	// decode PKCS1 PEM and marshal the public half
	block, _ := pem.Decode([]byte(privPEM))
	if block == nil {
		return "", errors.New("failed to decode private key PEM")
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("failed to parse private key: %w", err)
	}
	pub, err := ssh.NewPublicKey(&key.PublicKey)
	if err != nil {
		return "", fmt.Errorf("failed to build SSH public key: %w", err)
	}
	return string(ssh.MarshalAuthorizedKey(pub)), nil
}

// captureOutputs copies hostname and externalIP from a Pulumi up result.
func (i *GceMachineInfra) captureOutputs(outputs auto.OutputMap) {
	// copy hostname and NAT IP when the output values are strings
	if v, ok := outputs["hostname"]; ok {
		if s, ok := v.Value.(string); ok {
			i.hostname = s
		}
	}
	if v, ok := outputs["externalIP"]; ok {
		if s, ok := v.Value.(string); ok {
			i.externalIP = s
		}
	}
}

// CreateOutputs returns the instance hostname, public IP, and SSH private key.
// The private key is never written to Pulumi state.
func (i *GceMachineInfra) CreateOutputs() (hostname, externalIP, sshPrivateKey string) {
	return i.hostname, i.externalIP, i.sshPrivateKeyPEM
}

// SetCreateOutputs stores hostname, public IP, and SSH private key on the provider.
func (i *GceMachineInfra) SetCreateOutputs(hostname, externalIP, sshPrivateKey string) {
	// store hostname, public IP, and private key for a later deploy
	i.hostname = hostname
	i.externalIP = externalIP
	i.sshPrivateKeyPEM = sshPrivateKey
}

// SeedSSHKeyPair loads a persisted private key onto the provider and derives
// its authorized-keys public form so the next deploy reuses that key.
func (i *GceMachineInfra) SeedSSHKeyPair(sshPrivateKeyPEM string) error {
	if sshPrivateKeyPEM == "" {
		return fmt.Errorf("failed to seed ssh key pair: private key is empty")
	}

	// parse the persisted PEM and derive the public half
	signer, err := ssh.ParsePrivateKey([]byte(sshPrivateKeyPEM))
	if err != nil {
		return fmt.Errorf("failed to parse persisted SSH private key: %w", err)
	}
	i.sshPrivateKeyPEM = sshPrivateKeyPEM
	i.sshPublicKeyAuthorized = string(ssh.MarshalAuthorizedKey(signer.PublicKey()))
	return nil
}

// persistGeneratedSSHKey writes the in-memory private key when PersistSSHKey is set.
func (i *GceMachineInfra) persistGeneratedSSHKey() error {
	if i.PersistSSHKey == nil || i.sshPrivateKeyPEM == "" {
		return nil
	}
	return i.PersistSSHKey(i.sshPrivateKeyPEM)
}
