package supportservices

import (
	"net/http"
	"strings"
	"testing"

	api "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// appendingEditor appends one extra document to the component manifest.
type appendingEditor struct{}

// GetFullyQualifiedType returns the test attacher type.
func (e *appendingEditor) GetFullyQualifiedType() string {
	return "appendingEditor"
}

// ObjectID returns no id.
func (e *appendingEditor) ObjectID() *uint {
	return nil
}

// EditManifest appends one extra document.
func (e *appendingEditor) EditManifest(base func() (string, error)) (string, error) {
	// append the extra document
	document, err := base()
	if err != nil {
		return "", err
	}
	return document + "\n---\nkind: Extra", nil
}

// noopGatewayUpdate satisfies the update argument when the attached API instance is not stored.
func noopGatewayUpdate(*http.Client, string, *api.GatewayInstance) (*api.GatewayInstance, error) {
	// skip the write
	return nil, nil
}

// TestEnsureComponentRejectsAMissingRuntime reports the component whose runtime is absent.
func TestEnsureComponentRejectsAMissingRuntime(t *testing.T) {
	gateway := &api.GatewayInstance{}
	cases := []struct {
		name string
		call func() error
		want string
	}{
		{
			name: "ingress",
			call: func() error {
				return EnsureIngressComponent(nil, nil, gateway, nil, noopGatewayUpdate, nil)
			},
			want: "ingress component runtime is missing",
		},
		{
			name: "editor",
			call: func() error {
				return EnsureIngressComponent(nil, nil, &appendingEditor{}, nil, nil, nil)
			},
			want: "ingress component runtime is missing",
		},
		{
			name: "certificates",
			call: func() error {
				return EnsureCertificatesComponent(nil, nil, gateway, nil, noopGatewayUpdate, nil)
			},
			want: "certificates component runtime is missing",
		},
		{
			name: "secrets",
			call: func() error {
				return EnsureSecretsComponent(nil, nil, &api.SecretInstance{}, nil, nil, nil, nil)
			},
			want: "secrets component runtime is missing",
		},
		{
			name: "database",
			call: func() error {
				return EnsureDatabaseComponent(nil, nil, gateway, nil, noopGatewayUpdate, nil)
			},
			want: "database component runtime is missing",
		},
		{
			name: "release ingress",
			call: func() error {
				return ReleaseIngressComponent(nil, nil, gateway)
			},
			want: "ingress component runtime is missing",
		},
		{
			name: "release certificates",
			call: func() error {
				return ReleaseCertificatesComponent(nil, nil, gateway)
			},
			want: "certificates component runtime is missing",
		},
		{
			name: "release secrets",
			call: func() error {
				return ReleaseSecretsComponent(nil, nil, &api.SecretInstance{})
			},
			want: "secrets component runtime is missing",
		},
		{
			name: "release database",
			call: func() error {
				return ReleaseDatabaseComponent(nil, nil, gateway)
			},
			want: "database component runtime is missing",
		},
	}
	// reject each component whose runtime is absent
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %v, want %q", err, tc.want)
			}
		})
	}
}

// TestEnsureDatabaseComponentReportsAMissingManifest stops before a workload exists.
func TestEnsureDatabaseComponentReportsAMissingManifest(t *testing.T) {
	// install with no manifest
	runtime := &api.KubernetesRuntimeInstance{Common: api.Common{ID: util.Ptr(uint(1))}}
	gateway := &api.GatewayInstance{}
	err := EnsureDatabaseComponent(nil, runtime, gateway, nil, noopGatewayUpdate, nil)
	if err == nil || err.Error() != "database component manifest is missing" {
		t.Fatalf("error %v, want the missing manifest", err)
	}
	if runtime.DatabaseComponentInstanceID != nil {
		t.Fatalf("slot = %v, want empty", runtime.DatabaseComponentInstanceID)
	}
}

// TestSecretsProviderCreates allows eks and kind to install the secrets component.
func TestSecretsProviderCreates(t *testing.T) {
	// allow eks and kind
	if !secretsProviderCreates(util.Ptr("eks")) || !secretsProviderCreates(util.Ptr("kind")) {
		t.Fatal("eks and kind create the secrets component")
	}
	if secretsProviderCreates(util.Ptr("gke")) || secretsProviderCreates(nil) {
		t.Fatal("gke and an empty provider skip the secrets component")
	}
}

// TestEnsureSecretsComponentSkipsOtherProviders leaves the secrets workload id empty for gke.
func TestEnsureSecretsComponentSkipsOtherProviders(t *testing.T) {
	// skip a provider that does not create the component
	name := "dev"
	runtime := &api.KubernetesRuntimeInstance{
		Common:   api.Common{ID: util.Ptr(uint(1))},
		Instance: api.Instance{Name: &name},
	}
	err := EnsureSecretsComponent(nil, runtime, &api.SecretInstance{}, nil, nil, util.Ptr("gke"), nil)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if runtime.SecretsComponentInstanceID != nil {
		t.Fatalf("slot = %v, want empty", runtime.SecretsComponentInstanceID)
	}
}

// TestReleaseComponentLeavesAnEmptySlot returns before deleting a workload.
func TestReleaseComponentLeavesAnEmptySlot(t *testing.T) {
	// release a runtime that stores no ingress workload id
	runtime := &api.KubernetesRuntimeInstance{Common: api.Common{ID: util.Ptr(uint(9))}}
	err := release(
		nil,
		*runtime.ID,
		&runtime.IngressComponentInstanceID,
		"ingress component",
		ingressColumn,
		"",
		nil,
	)
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if runtime.IngressComponentInstanceID != nil {
		t.Fatalf("slot = %v, want empty", runtime.IngressComponentInstanceID)
	}
}

// TestAttacherManifestUsesAnEditor reports an API instance that edits a support services operator component manifest.
func TestAttacherManifestUsesAnEditor(t *testing.T) {
	// a gateway instance stores the workload id and keeps the stock ingress manifest
	if attacherManifest(&api.GatewayInstance{}) != nil {
		t.Fatal("gateway instance supplied an editor")
	}

	// an attached instance that edits the manifest supplies the synced document
	if attacherManifest(&appendingEditor{}) == nil {
		t.Fatal("editor supplied no document")
	}
}

// TestComponentDocumentAppendsTheEditor joins the component manifest with the editor output.
func TestComponentDocumentAppendsTheEditor(t *testing.T) {
	// build the synced document
	document, err := componentDocument(&appendingEditor{}, func() (string, error) {
		return "kind: Component", nil
	})
	if err != nil {
		t.Fatalf("document: %v", err)
	}
	if document != "kind: Component\n---\nkind: Extra" {
		t.Fatalf("document %q, want the edited manifest", document)
	}
}
