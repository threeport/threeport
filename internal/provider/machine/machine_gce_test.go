package machine

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"golang.org/x/crypto/ssh"
	"gorm.io/datatypes"

	"github.com/threeport/threeport/internal/provider"
)

// requirePulumi skips the test when the pulumi CLI is not on PATH.
// SetStackState shells out through the local workspace, so that path cannot run without it.
func requirePulumi(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("pulumi"); err != nil {
		t.Skip("pulumi CLI not found on PATH; skipping pulumi-backed test")
	}
}

// recordedResource is one NewResource call captured by recordingMocks.
// It holds the type token, name, and mapped inputs.
type recordedResource struct {
	typeToken string
	name      string
	inputs    map[string]any
}

// recordingMocks is a pulumi mock that records each NewResource call.
type recordingMocks struct {
	mu        sync.Mutex
	resources []recordedResource
}

// NewResource records the resource and returns name-id plus args.Inputs.
func (m *recordingMocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	m.mu.Lock()
	m.resources = append(m.resources, recordedResource{
		typeToken: args.TypeToken,
		name:      args.Name,
		inputs:    args.Inputs.Mappable(),
	})
	m.mu.Unlock()
	return args.Name + "-id", args.Inputs, nil
}

// Call returns an empty property map.
func (m *recordingMocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	return resource.PropertyMap{}, nil
}

// byType returns recorded resources whose type token matches.
func (m *recordingMocks) byType(typeToken string) []recordedResource {
	var out []recordedResource
	for _, r := range m.resources {
		if r.typeToken == typeToken {
			out = append(out, r)
		}
	}
	return out
}

const (
	instanceTypeToken    = "gcp:compute/instance:Instance"
	firewallTypeToken    = "gcp:compute/firewall:Firewall"
	gcpProviderTypeToken = "pulumi:providers:gcp"
)

// newTestInfra returns a GceMachineInfra with test project, zone, and image fields set.
func newTestInfra(name string) *GceMachineInfra {
	return &GceMachineInfra{
		PulumiWorkspace: provider.PulumiWorkspace{
			RuntimeInstanceName: name,
			ProjectName:         "gce",
		},
		ProjectID:       "test-project",
		Region:          "us-central1",
		Zone:            "us-central1-a",
		MachineType:     "e2-medium",
		ImageID:         "debian-cloud/debian-12",
		NetworkID:       "default",
		SSHUser:         "threeport",
		SSHSourceRanges: []string{"10.0.0.0/8"},
	}
}

// TestGenerateSSHKeyPair_ValidFormats covers PEM PKCS1 private and authorized-key public material that sign and verify.
func TestGenerateSSHKeyPair_ValidFormats(t *testing.T) {
	priv, pub, err := generateSSHKeyPair()
	if err != nil {
		t.Fatalf("generateSSHKeyPair returned error: %v", err)
	}
	if priv == "" || pub == "" {
		t.Fatalf("expected non-empty key material, got priv=%q pub=%q", priv, pub)
	}

	// parse private key as PEM PKCS1
	block, _ := pem.Decode([]byte(priv))
	if block == nil {
		t.Fatal("private key did not decode as PEM")
	}
	rsaKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		t.Fatalf("private key did not parse as PKCS1: %v", err)
	}

	// parse public key as authorized-key
	pubKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(pub))
	if err != nil {
		t.Fatalf("public key did not parse as authorized key: %v", err)
	}

	// sign with private key and verify with public key
	signer, err := ssh.NewSignerFromKey(rsaKey)
	if err != nil {
		t.Fatalf("failed to build signer from private key: %v", err)
	}
	msg := []byte("threeport gce ssh round trip")
	sig, err := signer.Sign(nil, msg)
	if err != nil {
		t.Fatalf("failed to sign message: %v", err)
	}
	if err := pubKey.Verify(msg, sig); err != nil {
		t.Fatalf("public key failed to verify signature from private key: %v", err)
	}
}

// TestEnsureSSHKeyPair_IdempotentAcrossCalls asserts a second call keeps the first pair and a seeded public key is left alone.
func TestEnsureSSHKeyPair_IdempotentAcrossCalls(t *testing.T) {
	i := newTestInfra("idempotent")

	// generate on first call
	if err := i.ensureSSHKeyPair(); err != nil {
		t.Fatalf("first ensureSSHKeyPair: %v", err)
	}
	firstPriv := i.sshPrivateKeyPEM
	firstPub := i.sshPublicKeyAuthorized
	if firstPriv == "" || firstPub == "" {
		t.Fatal("expected key material after first call")
	}

	// keep the same pair on a second call
	if err := i.ensureSSHKeyPair(); err != nil {
		t.Fatalf("second ensureSSHKeyPair: %v", err)
	}
	if i.sshPrivateKeyPEM != firstPriv || i.sshPublicKeyAuthorized != firstPub {
		t.Fatal("second ensureSSHKeyPair overwrote existing key material")
	}

	// skip generation when a public key is already set
	seeded := newTestInfra("seeded")
	seeded.sshPublicKeyAuthorized = "ssh-rsa AAAAseeded threeport"
	if err := seeded.ensureSSHKeyPair(); err != nil {
		t.Fatalf("ensureSSHKeyPair on seeded: %v", err)
	}
	if seeded.sshPublicKeyAuthorized != "ssh-rsa AAAAseeded threeport" {
		t.Fatal("ensureSSHKeyPair overwrote a pre-seeded public key")
	}
	if seeded.sshPrivateKeyPEM != "" {
		t.Fatal("ensureSSHKeyPair generated a private key when the public key was already set")
	}
}

// TestPulumiProgram_CreatesInstanceAndFirewall covers one instance and one SSH firewall from the program.
func TestPulumiProgram_CreatesInstanceAndFirewall(t *testing.T) {
	i := newTestInfra("create-test")
	if err := i.ensureSSHKeyPair(); err != nil {
		t.Fatalf("ensureSSHKeyPair: %v", err)
	}

	// run the program against recording mocks
	mocks := &recordingMocks{}
	if err := pulumi.RunErr(i.pulumiProgram(), pulumi.WithMocks("gce", "test-stack", mocks)); err != nil {
		t.Fatalf("RunErr: %v", err)
	}

	// assert one instance and one firewall
	instances := mocks.byType(instanceTypeToken)
	if len(instances) != 1 {
		t.Fatalf("expected exactly 1 instance, got %d", len(instances))
	}
	firewalls := mocks.byType(firewallTypeToken)
	if len(firewalls) != 1 {
		t.Fatalf("expected exactly 1 firewall, got %d", len(firewalls))
	}

	// check instance machine type and zone
	inst := instances[0]
	if got := inst.inputs["machineType"]; got != "e2-medium" {
		t.Errorf("instance machineType = %v, want e2-medium", got)
	}
	if got := inst.inputs["zone"]; got != "us-central1-a" {
		t.Errorf("instance zone = %v, want us-central1-a", got)
	}

	// check firewall source ranges and tcp/22
	if got := firewallSourceRanges(t, firewalls[0]); !equalStringSlices(got, i.sshSourceRanges()) {
		t.Errorf("firewall sourceRanges = %v, want %v", got, i.sshSourceRanges())
	}
	if !firewallAllowsTCP22(t, firewalls[0]) {
		t.Errorf("firewall does not allow tcp/22: %v", firewalls[0].inputs["allows"])
	}
	if got := stringSliceInput(t, firewalls[0].inputs["targetTags"]); !equalStringSlices(got, []string{i.RuntimeInstanceName}) {
		t.Errorf("firewall targetTags = %v, want [%s]", got, i.RuntimeInstanceName)
	}
	if got := stringSliceInput(t, inst.inputs["tags"]); !equalStringSlices(got, []string{i.RuntimeInstanceName}) {
		t.Errorf("instance tags = %v, want [%s]", got, i.RuntimeInstanceName)
	}
	wantLabels := provider.GcpResourceLabels(i.RuntimeInstanceName)
	if got := stringMapInput(t, inst.inputs["labels"]); !equalStringMaps(got, wantLabels) {
		t.Errorf("instance labels = %v, want %v", got, wantLabels)
	}
	if got, ok := firewalls[0].inputs["description"].(string); !ok || !strings.Contains(got, provider.GcpOwnershipDescription(i.RuntimeInstanceName)) {
		t.Errorf("firewall description = %v, want it to contain ownership pair", firewalls[0].inputs["description"])
	}
}

// TestPulumiProgram_InjectsSSHKeyMetadata asserts ssh-keys metadata is user:pubkey and holds no private key.
func TestPulumiProgram_InjectsSSHKeyMetadata(t *testing.T) {
	i := newTestInfra("metadata-test")
	if err := i.ensureSSHKeyPair(); err != nil {
		t.Fatalf("ensureSSHKeyPair: %v", err)
	}

	mocks := &recordingMocks{}
	if err := pulumi.RunErr(i.pulumiProgram(), pulumi.WithMocks("gce", "test-stack", mocks)); err != nil {
		t.Fatalf("RunErr: %v", err)
	}

	instances := mocks.byType(instanceTypeToken)
	if len(instances) != 1 {
		t.Fatalf("expected exactly 1 instance, got %d", len(instances))
	}

	// check ssh-keys prefix, public key, and no private key
	sshKeys := instanceSSHKeysMetadata(t, instances[0])
	wantPrefix := i.SSHUser + ":"
	if !strings.HasPrefix(sshKeys, wantPrefix) {
		t.Errorf("ssh-keys metadata = %q, want prefix %q", sshKeys, wantPrefix)
	}
	if !strings.Contains(sshKeys, strings.TrimSpace(i.sshPublicKeyAuthorized)) {
		t.Errorf("ssh-keys metadata does not contain the generated public key")
	}
	if strings.Contains(sshKeys, "PRIVATE KEY") {
		t.Errorf("ssh-keys metadata contains a private key: %q", sshKeys)
	}
}

// TestPulumiProgram_DoesNotExportPrivateKey rejects a private key in any mocked resource input.
func TestPulumiProgram_DoesNotExportPrivateKey(t *testing.T) {
	i := newTestInfra("no-export-test")
	if err := i.ensureSSHKeyPair(); err != nil {
		t.Fatalf("ensureSSHKeyPair: %v", err)
	}
	if i.sshPrivateKeyPEM == "" {
		t.Fatal("expected a generated private key on the receiver")
	}

	mocks := &recordingMocks{}
	if err := pulumi.RunErr(i.pulumiProgram(), pulumi.WithMocks("gce", "test-stack", mocks)); err != nil {
		t.Fatalf("RunErr: %v", err)
	}

	for _, r := range mocks.resources {
		if containsPrivateKey(r.inputs, i.sshPrivateKeyPEM) {
			t.Fatalf("resource %s (%s) inputs contain the private key", r.name, r.typeToken)
		}
	}
}

// TestCaptureOutputs_MapsHostnameAndIP covers hostname and externalIP from stack outputs and empty maps.
func TestCaptureOutputs_MapsHostnameAndIP(t *testing.T) {
	i := newTestInfra("capture-test")

	outputs := auto.OutputMap{
		"hostname":   auto.OutputValue{Value: "vm-host"},
		"externalIP": auto.OutputValue{Value: "203.0.113.7"},
	}
	i.captureOutputs(outputs)
	if i.hostname != "vm-host" {
		t.Errorf("hostname = %q, want vm-host", i.hostname)
	}
	if i.externalIP != "203.0.113.7" {
		t.Errorf("externalIP = %q, want 203.0.113.7", i.externalIP)
	}

	// leave fields empty when the output map is empty
	empty := newTestInfra("capture-empty")
	empty.captureOutputs(auto.OutputMap{})
	if empty.hostname != "" || empty.externalIP != "" {
		t.Errorf("expected empty fields, got hostname=%q externalIP=%q", empty.hostname, empty.externalIP)
	}
}

// TestCreateOutputs_SurfacesHostnameIPKey covers hostname, external IP, and PEM private key from CreateOutputs.
func TestCreateOutputs_SurfacesHostnameIPKey(t *testing.T) {
	i := newTestInfra("outputs-test")
	if err := i.ensureSSHKeyPair(); err != nil {
		t.Fatalf("ensureSSHKeyPair: %v", err)
	}
	i.captureOutputs(auto.OutputMap{
		"hostname":   auto.OutputValue{Value: "vm-host"},
		"externalIP": auto.OutputValue{Value: "203.0.113.7"},
	})

	hostname, externalIP, sshPrivateKey := i.CreateOutputs()
	if hostname != "vm-host" {
		t.Errorf("CreateOutputs hostname = %q, want vm-host", hostname)
	}
	if externalIP != "203.0.113.7" {
		t.Errorf("CreateOutputs externalIP = %q, want 203.0.113.7", externalIP)
	}
	if sshPrivateKey != i.sshPrivateKeyPEM {
		t.Error("CreateOutputs did not return the generated private key")
	}
	if !strings.Contains(sshPrivateKey, "PRIVATE KEY") {
		t.Errorf("CreateOutputs private key is not a PEM: %q", sshPrivateKey)
	}
}

// TestGceInfra_SatisfiesStreamableRefreshable asserts the state file path under the injected root.
func TestGceInfra_SatisfiesStreamableRefreshable(t *testing.T) {
	root := t.TempDir()
	i := NewGceMachineInfra("x", provider.WithStateDirRoot(root))

	path, err := i.GetStateFilePath()
	if err != nil {
		t.Fatalf("GetStateFilePath: %v", err)
	}
	wantSuffix := filepath.Join(".pulumi", "stacks", "gce", "x.json")
	if !strings.HasSuffix(path, wantSuffix) {
		t.Errorf("state file path = %q, want suffix %q", path, wantSuffix)
	}
	if !strings.HasPrefix(path, root) {
		t.Errorf("state file path = %q, want it under temp root %q", path, root)
	}
}

// TestNewGceMachineInfra_BuildsWorkspaceWithStateDirRoot covers distinct state paths under one root for two names.
func TestNewGceMachineInfra_BuildsWorkspaceWithStateDirRoot(t *testing.T) {
	root := t.TempDir()
	a := NewGceMachineInfra("alpha", provider.WithStateDirRoot(root))
	b := NewGceMachineInfra("beta", provider.WithStateDirRoot(root))

	pathA, err := a.GetStateFilePath()
	if err != nil {
		t.Fatalf("GetStateFilePath alpha: %v", err)
	}
	pathB, err := b.GetStateFilePath()
	if err != nil {
		t.Fatalf("GetStateFilePath beta: %v", err)
	}
	if pathA == pathB {
		t.Errorf("expected distinct state paths, both resolved to %q", pathA)
	}
	if !strings.HasPrefix(pathA, root) || !strings.HasPrefix(pathB, root) {
		t.Errorf("expected both paths under root %q, got %q and %q", root, pathA, pathB)
	}
}

// TestSetStackState_AppliesProjectDefaults covers SetStackState filling the gce project name on a literal workspace.
func TestSetStackState_AppliesProjectDefaults(t *testing.T) {
	requirePulumi(t)
	root := t.TempDir()
	i := &GceMachineInfra{
		PulumiWorkspace: provider.PulumiWorkspace{
			RuntimeInstanceName: "defaults",
		},
	}
	provider.WithStateDirRoot(root)(&i.PulumiWorkspace)

	blob := datatypes.JSON([]byte(`{"version":3,"checkpoint":{"stack":"gce/defaults"}}`))
	if err := i.SetStackState(&blob); err != nil {
		t.Fatalf("SetStackState: %v", err)
	}

	path, err := i.GetStateFilePath()
	if err != nil {
		t.Fatalf("GetStateFilePath: %v", err)
	}
	if !strings.Contains(path, filepath.Join("stacks", "gce", "defaults.json")) {
		t.Errorf("state path = %q, want it under stacks/gce", path)
	}
}

// TestStackStateRoundTrip_CheckpointFormat asserts checkpoint JSON writes and reads back unchanged with no leftover temp file.
func TestStackStateRoundTrip_CheckpointFormat(t *testing.T) {
	requirePulumi(t)
	root := t.TempDir()
	i := NewGceMachineInfra("roundtrip", provider.WithStateDirRoot(root))

	blob := datatypes.JSON([]byte(`{"version":3,"checkpoint":{"stack":"gce/roundtrip","latest":{}}}`))
	if err := i.SetStackState(&blob); err != nil {
		t.Fatalf("SetStackState: %v", err)
	}

	readBack, err := i.ReadStateFile()
	if err != nil {
		t.Fatalf("ReadStateFile: %v", err)
	}
	if readBack == nil {
		t.Fatal("ReadStateFile returned nil after SetStackState")
	}
	if string(*readBack) != string(blob) {
		t.Errorf("read-back state != written state\n got: %s\nwant: %s", *readBack, blob)
	}

	path, err := i.GetStateFilePath()
	if err != nil {
		t.Fatalf("GetStateFilePath: %v", err)
	}
	if exists(path + ".tmp") {
		t.Errorf("temp state file %q was not cleaned up", path+".tmp")
	}
}

// TestSSHSourceRanges_Required covers empty SSHSourceRanges failing validation.
func TestSSHSourceRanges_Required(t *testing.T) {
	i := newTestInfra("no-ranges")
	i.SSHSourceRanges = nil
	err := i.validateRequiredFields()
	if err == nil || !strings.Contains(err.Error(), "SSHSourceRanges") {
		t.Errorf("validateRequiredFields = %v, want SSHSourceRanges missing", err)
	}
}

// TestValidateRequiredFields_InvalidName covers a GCE name that GCP would reject.
func TestValidateRequiredFields_InvalidName(t *testing.T) {
	i := newTestInfra("Bad_Name")
	err := i.validateRequiredFields()
	if err == nil || !strings.Contains(err.Error(), "not a valid GCE instance name") {
		t.Errorf("validateRequiredFields = %v, want invalid GCE instance name", err)
	}
}

// TestValidateRequiredFields_NameLeavesRoomForFirewall covers the 59-char cap
// so {name}-ssh stays within GCE's 63-character firewall name limit.
func TestValidateRequiredFields_NameLeavesRoomForFirewall(t *testing.T) {
	ok := "a" + strings.Repeat("x", 57) + "z"
	i := newTestInfra(ok)
	if err := i.validateRequiredFields(); err != nil {
		t.Errorf("59-char name: %v", err)
	}
	tooLong := "a" + strings.Repeat("x", 58) + "z"
	i = newTestInfra(tooLong)
	err := i.validateRequiredFields()
	if err == nil || !strings.Contains(err.Error(), "not a valid GCE instance name") {
		t.Errorf("60-char name = %v, want invalid GCE instance name", err)
	}
}

// TestDeployInfra_MissingRequiredFields rejects each required field when it is empty.
func TestDeployInfra_MissingRequiredFields(t *testing.T) {
	base := func() *GceMachineInfra {
		return &GceMachineInfra{
			PulumiWorkspace: provider.PulumiWorkspace{RuntimeInstanceName: "validate"},
			ProjectID:       "p",
			Zone:            "z",
			MachineType:     "m",
			ImageID:         "img",
			SSHUser:         "u",
			NetworkID:       "default",
			SSHSourceRanges: []string{"10.0.0.0/8"},
		}
	}
	cases := []struct {
		name  string
		mut   func(*GceMachineInfra)
		field string
	}{
		{"missing runtime instance name", func(i *GceMachineInfra) { i.RuntimeInstanceName = "" }, "RuntimeInstanceName"},
		{"missing project id", func(i *GceMachineInfra) { i.ProjectID = "" }, "ProjectID"},
		{"missing zone", func(i *GceMachineInfra) { i.Zone = "" }, "Zone"},
		{"missing machine type", func(i *GceMachineInfra) { i.MachineType = "" }, "MachineType"},
		{"missing image id", func(i *GceMachineInfra) { i.ImageID = "" }, "ImageID"},
		{"missing ssh user", func(i *GceMachineInfra) { i.SSHUser = "" }, "SSHUser"},
		{"missing network id", func(i *GceMachineInfra) { i.NetworkID = "" }, "NetworkID"},
		{"missing ssh source ranges", func(i *GceMachineInfra) { i.SSHSourceRanges = nil }, "SSHSourceRanges"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			i := base()
			tc.mut(i)
			err := i.DeployInfra()
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tc.field)
			}
			if !strings.Contains(err.Error(), tc.field) {
				t.Errorf("error %q does not name the missing field %q", err.Error(), tc.field)
			}
		})
	}
}

// TestDeployInfra_ReportsAllMissingFields asserts one error names every empty required field.
func TestDeployInfra_ReportsAllMissingFields(t *testing.T) {
	i := &GceMachineInfra{
		PulumiWorkspace: provider.PulumiWorkspace{RuntimeInstanceName: "validate"},
		ProjectID:       "p",
		ImageID:         "img",
	}
	err := i.DeployInfra()
	if err == nil {
		t.Fatal("expected error for multiple missing fields, got nil")
	}
	for _, field := range []string{"Zone", "MachineType", "SSHUser", "NetworkID", "SSHSourceRanges"} {
		if !strings.Contains(err.Error(), field) {
			t.Errorf("error %q does not name missing field %q", err.Error(), field)
		}
	}
}

// TestEnsureSSHKeyPair_RestoredPrivateKey covers deriving the public key from a stored PEM.
func TestEnsureSSHKeyPair_RestoredPrivateKey(t *testing.T) {
	priv, pub, err := generateSSHKeyPair()
	if err != nil {
		t.Fatalf("generateSSHKeyPair: %v", err)
	}
	i := &GceMachineInfra{sshPrivateKeyPEM: priv}
	if err := i.ensureSSHKeyPair(); err != nil {
		t.Fatalf("ensureSSHKeyPair: %v", err)
	}
	if i.sshPrivateKeyPEM != priv {
		t.Error("ensureSSHKeyPair replaced the restored private key")
	}
	if i.sshPublicKeyAuthorized != pub {
		t.Errorf("derived public key = %q, want %q", i.sshPublicKeyAuthorized, pub)
	}
}

// TestSeedSSHKeyPair_ReusesPersistedKey covers loading a PEM and keeping it
// across ensureSSHKeyPair.
func TestSeedSSHKeyPair_ReusesPersistedKey(t *testing.T) {
	priv, pub, err := generateSSHKeyPair()
	if err != nil {
		t.Fatalf("generateSSHKeyPair: %v", err)
	}
	i := &GceMachineInfra{}

	if err := i.SeedSSHKeyPair(priv); err != nil {
		t.Fatalf("SeedSSHKeyPair: %v", err)
	}
	if i.sshPrivateKeyPEM != priv {
		t.Error("SeedSSHKeyPair did not store the private key")
	}
	if i.sshPublicKeyAuthorized != pub {
		t.Errorf("seeded public key = %q, want %q", i.sshPublicKeyAuthorized, pub)
	}

	if err := i.ensureSSHKeyPair(); err != nil {
		t.Fatalf("ensureSSHKeyPair after seed: %v", err)
	}
	if i.sshPrivateKeyPEM != priv {
		t.Error("ensureSSHKeyPair replaced a seeded private key")
	}
}

// TestSeedSSHKeyPair_RejectsEmpty covers an empty PEM.
func TestSeedSSHKeyPair_RejectsEmpty(t *testing.T) {
	i := &GceMachineInfra{}
	if err := i.SeedSSHKeyPair(""); err == nil {
		t.Fatal("expected error for empty private key")
	}
}

// TestPersistGeneratedSSHKey_CallsHookBeforeUp covers writing the PEM when
// PersistSSHKey is set, and skipping when it is nil.
func TestPersistGeneratedSSHKey_CallsHookBeforeUp(t *testing.T) {
	priv, _, err := generateSSHKeyPair()
	if err != nil {
		t.Fatalf("generateSSHKeyPair: %v", err)
	}

	var got string
	i := &GceMachineInfra{
		sshPrivateKeyPEM: priv,
		PersistSSHKey: func(pem string) error {
			got = pem
			return nil
		},
	}
	if err := i.persistGeneratedSSHKey(); err != nil {
		t.Fatalf("persistGeneratedSSHKey: %v", err)
	}
	if got != priv {
		t.Error("PersistSSHKey was not called with the in-memory private key")
	}

	skipped := &GceMachineInfra{sshPrivateKeyPEM: priv}
	if err := skipped.persistGeneratedSSHKey(); err != nil {
		t.Fatalf("persistGeneratedSSHKey with nil hook: %v", err)
	}
}

// TestSyncStackConfigs_RegionFromZone covers filling gcp:region from Zone.
func TestSyncStackConfigs_RegionFromZone(t *testing.T) {
	i := &GceMachineInfra{ProjectID: "p", Zone: "us-central1-a"}
	i.syncStackConfigs()
	if got := i.StackConfigs["gcp:region"]; got != "us-central1" {
		t.Errorf("gcp:region = %q, want us-central1", got)
	}
}

// exists reports whether path is present on disk.
func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// firewallSourceRanges returns the firewall sourceRanges input as strings.
func firewallSourceRanges(t *testing.T, r recordedResource) []string {
	t.Helper()
	raw, ok := r.inputs["sourceRanges"]
	if !ok {
		t.Fatal("firewall has no sourceRanges input")
	}
	return stringSliceInput(t, raw)
}

// stringSliceInput converts a Pulumi string-array input to []string.
func stringMapInput(t *testing.T, raw any) map[string]string {
	t.Helper()
	items, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("input is not a map: %T", raw)
	}
	out := make(map[string]string, len(items))
	for k, v := range items {
		s, ok := v.(string)
		if !ok {
			t.Fatalf("map value is not a string: %T", v)
		}
		out[k] = s
	}
	return out
}

func equalStringMaps(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func stringSliceInput(t *testing.T, raw any) []string {
	t.Helper()
	items, ok := raw.([]any)
	if !ok {
		t.Fatalf("input is not a slice: %T", raw)
	}
	out := make([]string, 0, len(items))
	for _, it := range items {
		s, ok := it.(string)
		if !ok {
			t.Fatalf("slice entry is not a string: %T", it)
		}
		out = append(out, s)
	}
	return out
}

// firewallAllowsTCP22 reports whether the firewall allows tcp port 22.
func firewallAllowsTCP22(t *testing.T, r recordedResource) bool {
	t.Helper()
	raw, ok := r.inputs["allows"]
	if !ok {
		return false
	}
	items, ok := raw.([]any)
	if !ok {
		t.Fatalf("allows is not a slice: %T", raw)
	}
	for _, it := range items {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		if m["protocol"] != "tcp" {
			continue
		}
		ports, ok := m["ports"].([]any)
		if !ok {
			continue
		}
		for _, p := range ports {
			if p == "22" {
				return true
			}
		}
	}
	return false
}

// instanceSSHKeysMetadata returns the instance ssh-keys metadata string.
func instanceSSHKeysMetadata(t *testing.T, r recordedResource) string {
	t.Helper()
	metaRaw, ok := r.inputs["metadata"]
	if !ok {
		t.Fatal("instance has no metadata input")
	}
	meta, ok := metaRaw.(map[string]any)
	if !ok {
		t.Fatalf("metadata is not a map: %T", metaRaw)
	}
	val, ok := meta["ssh-keys"]
	if !ok {
		t.Fatal("metadata has no ssh-keys entry")
	}
	s, ok := val.(string)
	if !ok {
		t.Fatalf("ssh-keys is not a string: %T", val)
	}
	return s
}

// containsPrivateKey reports whether inputs hold the PEM or a PRIVATE KEY marker.
func containsPrivateKey(inputs map[string]any, privPEM string) bool {
	return valueContains(inputs, privPEM) || valueContains(inputs, "PRIVATE KEY")
}

// valueContains walks strings, maps, and slices for needle.
func valueContains(v any, needle string) bool {
	switch t := v.(type) {
	case string:
		return strings.Contains(t, needle)
	case map[string]any:
		for _, e := range t {
			if valueContains(e, needle) {
				return true
			}
		}
	case []any:
		for _, e := range t {
			if valueContains(e, needle) {
				return true
			}
		}
	}
	return false
}

// equalStringSlices reports whether a and b have the same strings in the same order.
func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for idx := range a {
		if a[idx] != b[idx] {
			return false
		}
	}
	return true
}

