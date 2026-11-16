package v0

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// fakeApi serves the objects a Replace looks up, each under its collection's
// path whether asked for by name or by ID, and records the body of the PUT
// the Replace sends.
type fakeApi struct {
	t       *testing.T
	objects map[string]interface{}
	sent    []byte
}

// serve starts the fake API and returns the endpoint a config takes.
func (f *fakeApi) serve() string {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			for path, object := range f.objects {
				if r.URL.Path == path || strings.HasPrefix(r.URL.Path, path+"/") {
					respondWithObject(f.t, w, object)
					return
				}
			}
			http.Error(w, "no object at "+r.URL.Path, http.StatusNotFound)

		case http.MethodPut:
			body, err := io.ReadAll(r.Body)
			require.NoError(f.t, err)
			f.sent = body

			// echo what was sent, rather than what the test wants to see
			var replaced map[string]interface{}
			require.NoError(f.t, json.Unmarshal(body, &replaced))
			replaced["ID"] = 1
			replaced["CreatedAt"] = time.Now()
			respondWithObject(f.t, w, replaced)

		default:
			http.Error(w, "unexpected "+r.Method, http.StatusMethodNotAllowed)
		}
	}))
	f.t.Cleanup(srv.Close)

	return strings.TrimPrefix(srv.URL, "http://")
}

// decodeSent decodes the PUT the Replace sent into object.
func (f *fakeApi) decodeSent(object interface{}) {
	f.t.Helper()
	require.NotNil(f.t, f.sent, "the replace sent no PUT")
	require.NoError(f.t, json.Unmarshal(f.sent, object))
}

// sentFields returns the top-level fields of the PUT the Replace sent.
func (f *fakeApi) sentFields() map[string]interface{} {
	f.t.Helper()
	var fields map[string]interface{}
	f.decodeSent(&fields)

	return fields
}

// TestReplaceKeepsWhatOnlyTheServerWrites covers the terraform instance's
// state document and outputs, which the controller writes: a replace that
// edits the instance has to send them back as they are. The state document
// is encrypted, and the API returns it encrypted, so it goes back the same.
func TestReplaceKeepsWhatOnlyTheServerWrites(t *testing.T) {
	name := "infra"
	existing := api_v0.TerraformInstance{
		Common:                api_v0.Common{ID: util.Ptr(uint(1))},
		Instance:              api_v0.Instance{Name: &name, Status: util.Ptr("Created")},
		Reconciliation:        api_v0.Reconciliation{Reconciled: util.Ptr(true), CreationConfirmed: util.Ptr(time.Now().UTC().Truncate(time.Second))},
		AwsProviderID:         util.Ptr(uint(2)),
		TerraformDefinitionID: util.Ptr(uint(3)),
		VarsDocument:          util.Ptr("ciphertext-of-the-old-vars"),
		StateDocument:         util.Ptr("ciphertext-of-the-state"),
	}
	api := &fakeApi{t: t, objects: map[string]interface{}{
		api_v0.PathTerraformInstances:   existing,
		api_v0.PathTerraformDefinitions: api_v0.TerraformDefinition{Common: api_v0.Common{ID: util.Ptr(uint(3))}},
		api_v0.PathAwsProviders:         api_v0.AwsProvider{Common: api_v0.Common{ID: util.Ptr(uint(2))}},
	}}

	config := &TerraformInstanceConfig{TerraformInstance: TerraformInstanceValues{
		Name:                &name,
		AwsProvider:         &AwsProviderValues{Name: util.Ptr("aws")},
		TerraformDefinition: &TerraformDefinitionValues{Name: util.Ptr("definition")},
	}}
	_, err := config.Replace(&http.Client{}, api.serve(), name)
	require.NoError(t, err)

	var sent api_v0.TerraformInstance
	api.decodeSent(&sent)
	assert.Equal(t, existing.StateDocument, sent.StateDocument, "the controller's state document must be sent back as it is")
	assert.Equal(t, existing.Status, sent.Status)
	assert.Equal(t, existing.Reconciled, sent.Reconciled)
	assert.Equal(t, existing.CreationConfirmed, sent.CreationConfirmed, "every reconciliation field is kept, not only Reconciled")
	assert.Nil(t, sent.VarsDocument, "a config naming no vars document clears the old one rather than keeping it")
}

// TestReplaceSetsTheVarsTheConfigNames covers the other half: a vars
// document the config names is read and sent.
func TestReplaceSetsTheVarsTheConfigNames(t *testing.T) {
	name := "infra"
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "vars.tfvars"), []byte(`region = "us-east-1"`), 0600))
	api := &fakeApi{t: t, objects: map[string]interface{}{
		api_v0.PathTerraformInstances: api_v0.TerraformInstance{
			Common:   api_v0.Common{ID: util.Ptr(uint(1))},
			Instance: api_v0.Instance{Name: &name},
		},
		api_v0.PathTerraformDefinitions: api_v0.TerraformDefinition{Common: api_v0.Common{ID: util.Ptr(uint(3))}},
		api_v0.PathAwsProviders:         api_v0.AwsProvider{Common: api_v0.Common{ID: util.Ptr(uint(2))}},
	}}

	config := &TerraformInstanceConfig{TerraformInstance: TerraformInstanceValues{
		Name:                &name,
		AwsProvider:         &AwsProviderValues{Name: util.Ptr("aws")},
		TerraformDefinition: &TerraformDefinitionValues{Name: util.Ptr("definition")},
		VarsDocument:        util.Ptr("vars.tfvars"),
		TerraformConfigPath: util.Ptr(filepath.Join(dir, "terraform.yaml")),
	}}
	_, err := config.Replace(&http.Client{}, api.serve(), name)
	require.NoError(t, err)

	var sent api_v0.TerraformInstance
	api.decodeSent(&sent)
	assert.Equal(t, util.Ptr(`region = "us-east-1"`), sent.VarsDocument)
}

// TestReplaceKeepsARuntimeConnected covers the connection a controller
// records on a kubernetes runtime instance, which a replace used to clear,
// and whether it hosts the control plane, which the installer records and a
// config that does not mention it is not changing.
func TestReplaceKeepsARuntimeConnected(t *testing.T) {
	name := "runtime"
	existing := api_v0.KubernetesRuntimeInstance{
		Common:                        api_v0.Common{ID: util.Ptr(uint(1))},
		Instance:                      api_v0.Instance{Name: &name},
		KubernetesRuntimeDefinitionID: util.Ptr(uint(2)),
		ThreeportControlPlaneHost:     util.Ptr(true),
		APIEndpoint:                   util.Ptr("https://10.0.0.1:6443"),
		CACertificate:                 util.Ptr("ca"),
		ConnectionToken:               util.Ptr("ciphertext-of-the-token"),
	}
	api := &fakeApi{t: t, objects: map[string]interface{}{
		api_v0.PathKubernetesRuntimeInstances:   existing,
		api_v0.PathKubernetesRuntimeDefinitions: api_v0.KubernetesRuntimeDefinition{Common: api_v0.Common{ID: util.Ptr(uint(2))}},
	}}

	config := &KubernetesRuntimeInstanceConfig{KubernetesRuntimeInstance: KubernetesRuntimeInstanceValues{
		Name:                        &name,
		Location:                    util.Ptr("us-east-1"),
		KubernetesRuntimeDefinition: &KubernetesRuntimeDefinitionValues{Name: util.Ptr("definition")},
	}}
	_, err := config.Replace(&http.Client{}, api.serve(), name)
	require.NoError(t, err)

	var sent api_v0.KubernetesRuntimeInstance
	api.decodeSent(&sent)
	assert.Equal(t, existing.APIEndpoint, sent.APIEndpoint)
	assert.Equal(t, existing.CACertificate, sent.CACertificate)
	assert.Equal(t, existing.ConnectionToken, sent.ConnectionToken)
	assert.Equal(t, util.Ptr("us-east-1"), sent.Location)
	assert.Equal(t, util.Ptr(true), sent.ThreeportControlPlaneHost, "a config not mentioning the control plane host does not unmark it")
}

// TestReplaceRemovesAGatewaysDomainName covers a gateway definition whose
// config no longer names a domain name: the replace has to send no domain,
// not keep the one the copy carries.
func TestReplaceRemovesAGatewaysDomainName(t *testing.T) {
	name := "gateway"
	api := &fakeApi{t: t, objects: map[string]interface{}{
		api_v0.PathGatewayDefinitions: api_v0.GatewayDefinition{
			Common:                         api_v0.Common{ID: util.Ptr(uint(1))},
			Definition:                     api_v0.Definition{Name: &name},
			DomainNameDefinitionID:         util.Ptr(uint(4)),
			KubernetesWorkloadDefinitionID: util.Ptr(uint(5)),
		},
	}}

	config := &GatewayDefinitionConfig{GatewayDefinition: GatewayDefinitionValues{
		Name:        &name,
		ServiceName: util.Ptr("web"),
		HttpPorts:   &[]GatewayHttpPortValues{{Port: util.Ptr(80), Path: util.Ptr("/"), HTTPSRedirect: util.Ptr(false)}},
	}}
	_, err := config.Replace(&http.Client{}, api.serve(), name)
	require.NoError(t, err)

	var sent api_v0.GatewayDefinition
	api.decodeSent(&sent)
	assert.Nil(t, sent.DomainNameDefinitionID, "a gateway whose config names no domain name loses the one it had")
	assert.Equal(t, util.Ptr(uint(5)), sent.KubernetesWorkloadDefinitionID, "the workload definition a controller set is kept")
}

// TestReplaceMovesASecretToAnotherKindOfWorkload covers a secret instance
// whose config now names a helm workload instead of a kubernetes one: the
// copy's kubernetes workload has to be cleared, or the secret would point at
// both.
func TestReplaceMovesASecretToAnotherKindOfWorkload(t *testing.T) {
	name := "secret"
	api := &fakeApi{t: t, objects: map[string]interface{}{
		api_v0.PathSecretInstances: api_v0.SecretInstance{
			Common:                       api_v0.Common{ID: util.Ptr(uint(1))},
			Instance:                     api_v0.Instance{Name: &name},
			KubernetesRuntimeInstanceID:  util.Ptr(uint(2)),
			SecretDefinitionID:           util.Ptr(uint(3)),
			KubernetesWorkloadInstanceID: util.Ptr(uint(4)),
		},
		api_v0.PathKubernetesRuntimeInstances: api_v0.KubernetesRuntimeInstance{Common: api_v0.Common{ID: util.Ptr(uint(2))}},
		api_v0.PathSecretDefinitions:          api_v0.SecretDefinition{Common: api_v0.Common{ID: util.Ptr(uint(3))}},
		api_v0.PathHelmWorkloadInstances:      api_v0.HelmWorkloadInstance{Common: api_v0.Common{ID: util.Ptr(uint(5))}},
	}}

	config := &SecretInstanceConfig{SecretInstance: SecretInstanceValues{
		Name:                      &name,
		SecretDefinition:          &SecretDefinitionValues{Name: util.Ptr("definition")},
		HelmWorkloadInstance:      &HelmWorkloadInstanceValues{Name: util.Ptr("helm")},
		KubernetesRuntimeInstance: &KubernetesRuntimeInstanceValues{Name: util.Ptr("runtime")},
	}}
	_, err := config.Replace(&http.Client{}, api.serve(), name)
	require.NoError(t, err)

	var sent api_v0.SecretInstance
	api.decodeSent(&sent)
	assert.Equal(t, util.Ptr(uint(5)), sent.HelmWorkloadInstanceID)
	assert.Nil(t, sent.KubernetesWorkloadInstanceID, "the workload the secret moved away from is cleared")
}

// TestReplaceTakesAMachineRuntimeConfigAsCreateDoes covers the fields the
// machine runtime instance's create took from the config and its replace
// ignored, clearing them: the host key, the definition, and an SSH key read
// from a file.
func TestReplaceTakesAMachineRuntimeConfigAsCreateDoes(t *testing.T) {
	name := "machine"
	keyFile := filepath.Join(t.TempDir(), "id_ed25519")
	require.NoError(t, os.WriteFile(keyFile, []byte("private-key\n"), 0600))
	api := &fakeApi{t: t, objects: map[string]interface{}{
		api_v0.PathMachineRuntimeInstances: api_v0.MachineRuntimeInstance{
			Common:                     api_v0.Common{ID: util.Ptr(uint(1))},
			Instance:                   api_v0.Instance{Name: &name, Status: util.Ptr("Ready")},
			MachineRuntimeDefinitionID: util.Ptr(uint(2)),
			HostKey:                    util.Ptr("old-host-key"),
			Region:                     util.Ptr("us-east-1"),
		},
		api_v0.PathMachineRuntimeDefinitions: api_v0.MachineRuntimeDefinition{Common: api_v0.Common{ID: util.Ptr(uint(2))}},
	}}

	config := &MachineRuntimeInstanceConfig{MachineRuntimeInstance: MachineRuntimeInstanceValues{
		Name:                     &name,
		Hostname:                 util.Ptr("10.0.0.5"),
		SSHUser:                  util.Ptr("ubuntu"),
		SSHKeyFile:               &keyFile,
		HostKey:                  util.Ptr("new-host-key"),
		MachineRuntimeDefinition: &MachineRuntimeDefinitionValues{Name: util.Ptr("definition")},
	}}
	_, err := config.Replace(&http.Client{}, api.serve(), name)
	require.NoError(t, err)

	var sent api_v0.MachineRuntimeInstance
	api.decodeSent(&sent)
	assert.Equal(t, util.Ptr("new-host-key"), sent.HostKey)
	assert.Equal(t, util.Ptr(uint(2)), sent.MachineRuntimeDefinitionID)
	assert.Equal(t, util.Ptr("private-key"), sent.SSHKey, "an SSH key given as a file is read, as create reads it")
	assert.Equal(t, util.Ptr("us-east-1"), sent.Region, "what the provisioner recorded is kept")
	assert.Equal(t, util.Ptr("Ready"), sent.Status)
}

// TestReplaceSendsNothingTheApiRefuses covers what the copy brings along
// from the fetched object: the replace client strips the ID and timestamps,
// and a deleted-at time is never set on a live object, so the PUT carries
// none of the GORM model fields the API refuses on a full replace.
func TestReplaceSendsNothingTheApiRefuses(t *testing.T) {
	name := "control-plane"
	createdAt := time.Now()
	api := &fakeApi{t: t, objects: map[string]interface{}{
		api_v0.PathControlPlaneInstances: api_v0.ControlPlaneInstance{
			Common:                   api_v0.Common{ID: util.Ptr(uint(1)), CreatedAt: &createdAt, UpdatedAt: &createdAt},
			Instance:                 api_v0.Instance{Name: &name},
			Namespace:                util.Ptr("threeport-control-plane"),
			Genesis:                  util.Ptr(true),
			ApiServerEndpoint:        util.Ptr("https://api.example.com"),
			ClientCert:               util.Ptr("cert"),
			ControlPlaneDefinitionID: util.Ptr(uint(2)),
		},
	}}

	config := &ControlPlaneInstanceConfig{ControlPlaneInstance: ControlPlaneInstanceValues{
		Name:                   &name,
		Namespace:              util.Ptr("threeport-control-plane"),
		ControlPlaneDefinition: &ControlPlaneDefinitionValues{Name: util.Ptr("definition")},
	}}
	_, err := config.Replace(&http.Client{}, api.serve(), name)
	require.NoError(t, err)

	fields := api.sentFields()
	for _, field := range []string{"ID", "CreatedAt", "UpdatedAt", "DeletedAt"} {
		assert.NotContains(t, fields, field, "the API refuses a replace carrying %s", field)
	}
	assert.Equal(t, "https://api.example.com", fields["ApiServerEndpoint"], "the control plane's connection details are kept")
	assert.Equal(t, true, fields["Genesis"])
}
