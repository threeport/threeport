package terraform

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/go-logr/logr"

	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// partialApplyConfig creates one resource and then fails on a second one
// while var.fail is true.  terraform_data is built into terraform, so the
// config needs no provider download.
const partialApplyConfig = `
variable "fail" {
  type = bool
}

resource "terraform_data" "first" {}

resource "terraform_data" "second" {
  depends_on = [terraform_data.first]

  provisioner "local-exec" {
    command = var.fail ? "exit 1" : "true"
  }
}
`

// stateSavingAPI stands in for the Threeport API and records the state
// documents sent to it.
type stateSavingAPI struct {
	mu     sync.Mutex
	states []string
}

func (a *stateSavingAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var instance v0.TerraformInstance
	_ = json.Unmarshal(body, &instance)

	a.mu.Lock()
	if instance.StateDocument != nil {
		a.states = append(a.states, *instance.StateDocument)
	}
	a.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(apiserver_lib.Response{
		Data:   []apiserver_lib.Object{instance},
		Status: apiserver_lib.Status{Code: http.StatusOK},
	})
}

// newTestConfig returns a terraform instance config for a replica whose
// terraform directory is dir, saving state to the given API.
func newTestConfig(t *testing.T, dir string, apiAddr string, fail bool, savedState *string) *TerraformInstanceConfig {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, "terraform.tf"), []byte(partialApplyConfig), 0644); err != nil {
		t.Fatalf("failed to write terraform config: %v", err)
	}
	initOut, err := exec.Command("terraform", "-chdir="+dir, "init", "-no-color").CombinedOutput()
	if err != nil {
		t.Fatalf("terraform init failed: %v\n%s", err, initOut)
	}

	vars := "fail = false\n"
	if fail {
		vars = "fail = true\n"
	}
	log := logr.Discard()

	return &TerraformInstanceConfig{
		r: &controller.Reconciler{
			APIClient: http.DefaultClient,
			APIServer: apiAddr,
		},
		terraformInstance: &v0.TerraformInstance{
			Common:        v0.Common{ID: util.Ptr(uint(1))},
			VarsDocument:  &vars,
			StateDocument: savedState,
		},
		log: &log,
		awsConfig: &aws.Config{
			Credentials: credentials.NewStaticCredentialsProvider("test", "test", ""),
		},
		tfDirName: dir,
	}
}

// resourceIDs returns the ID terraform recorded for each resource in a state
// document.
func resourceIDs(t *testing.T, state string) map[string]string {
	t.Helper()

	var parsed struct {
		Resources []struct {
			Name      string `json:"name"`
			Instances []struct {
				Attributes struct {
					ID string `json:"id"`
				} `json:"attributes"`
			} `json:"instances"`
		} `json:"resources"`
	}
	if err := json.Unmarshal([]byte(state), &parsed); err != nil {
		t.Fatalf("failed to parse terraform state: %v", err)
	}

	ids := map[string]string{}
	for _, resource := range parsed.Resources {
		for _, instance := range resource.Instances {
			ids[resource.Name] = instance.Attributes.ID
		}
	}

	return ids
}

// TestRetryOnAnotherReplicaKeepsPartiallyCreatedResources covers an apply
// that creates one resource and then fails, retried by a replica that never
// saw the first attempt's terraform directory.  The retry must pick up the
// resource the failed attempt created instead of creating it again.
func TestRetryOnAnotherReplicaKeepsPartiallyCreatedResources(t *testing.T) {
	if _, err := exec.LookPath("terraform"); err != nil {
		t.Skip("terraform is not installed")
	}

	api := &stateSavingAPI{}
	server := httptest.NewServer(api)
	defer server.Close()
	apiAddr := strings.TrimPrefix(server.URL, "http://")

	// first replica: the apply creates the first resource, then fails
	firstReplica := newTestConfig(t, t.TempDir(), apiAddr, true, nil)
	if err := firstReplica.createTerraformInstance(); err == nil {
		t.Fatal("expected the first apply to fail")
	}
	if err := firstReplica.savePartialTerraformState(); err != nil {
		t.Fatalf("failed to save partial state: %v", err)
	}
	if len(api.states) != 1 {
		t.Fatalf("expected the partial state to be saved once, got %d saves", len(api.states))
	}
	partialID := resourceIDs(t, api.states[0])["first"]
	if partialID == "" {
		t.Fatal("the partial state does not record the resource the failed apply created")
	}

	// second replica: a fresh directory and the state saved in the API
	secondReplica := newTestConfig(t, t.TempDir(), apiAddr, false, &api.states[0])
	if err := secondReplica.createTerraformInstance(); err != nil {
		t.Fatalf("the retry failed: %v", err)
	}

	if got := resourceIDs(t, secondReplica.tfState)["first"]; got != partialID {
		t.Errorf("the retry created the first resource again: id %s, want %s", got, partialID)
	}
}

// TestSavePartialStateWithoutStateFile covers an apply that failed before
// terraform wrote any state, which leaves nothing to save.
func TestSavePartialStateWithoutStateFile(t *testing.T) {
	api := &stateSavingAPI{}
	server := httptest.NewServer(api)
	defer server.Close()

	c := &TerraformInstanceConfig{
		r: &controller.Reconciler{
			APIClient: http.DefaultClient,
			APIServer: strings.TrimPrefix(server.URL, "http://"),
		},
		terraformInstance: &v0.TerraformInstance{Common: v0.Common{ID: util.Ptr(uint(1))}},
		tfDirName:         t.TempDir(),
		applyRan:          true,
	}
	if err := c.savePartialTerraformState(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(api.states) != 0 {
		t.Errorf("expected no save without a state file, got %d", len(api.states))
	}
}

// TestNoStateIsSavedWhenApplyDidNotRun covers a create that failed before
// terraform apply ran, for instance while restoring the saved state.  The
// state file on disk is then not an apply's result and may be a damaged
// restore, so it must not replace the state saved in the API.
func TestNoStateIsSavedWhenApplyDidNotRun(t *testing.T) {
	api := &stateSavingAPI{}
	server := httptest.NewServer(api)
	defer server.Close()

	c := &TerraformInstanceConfig{
		r: &controller.Reconciler{
			APIClient: http.DefaultClient,
			APIServer: strings.TrimPrefix(server.URL, "http://"),
		},
		terraformInstance: &v0.TerraformInstance{Common: v0.Common{ID: util.Ptr(uint(1))}},
		tfDirName:         t.TempDir(),
	}
	if err := os.WriteFile(c.tfStateFilepath(), []byte(`{"version": 4, "lin`), 0644); err != nil {
		t.Fatal(err)
	}

	if err := c.savePartialTerraformState(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(api.states) != 0 {
		t.Errorf("a state file no apply wrote was saved to the API: %v", api.states)
	}
}

// TestAFailedRestoreLeavesTheStateFileIntact covers a restore whose write
// fails part way: the state file already in the directory must be left as it
// was rather than truncated.
func TestAFailedRestoreLeavesTheStateFileIntact(t *testing.T) {
	dir := t.TempDir()
	c := &TerraformInstanceConfig{
		terraformInstance: &v0.TerraformInstance{StateDocument: util.Ptr(`{"lineage": "a", "serial": 3}`)},
		tfDirName:         dir,
	}
	existing := `{"lineage": "b", "serial": 1}`
	if err := os.WriteFile(c.tfStateFilepath(), []byte(existing), 0644); err != nil {
		t.Fatal(err)
	}

	// a directory nothing can be written into makes the restore fail
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0700)

	if err := c.restoreTerraformState(); err == nil {
		t.Fatal("expected the restore to fail")
	}
	onDisk, err := os.ReadFile(c.tfStateFilepath())
	if err != nil {
		t.Fatal(err)
	}
	if string(onDisk) != existing {
		t.Errorf("the failed restore changed the state file to %s", onDisk)
	}
}

// TestRestoreTerraformState covers which state a create attempt starts from.
func TestRestoreTerraformState(t *testing.T) {
	state := func(lineage string, serial int) string {
		out, _ := json.Marshal(map[string]interface{}{"lineage": lineage, "serial": serial})
		return string(out)
	}

	cases := []struct {
		name       string
		local      *string
		saved      *string
		wantOnDisk *string
	}{
		{
			name:       "no saved state leaves the directory alone",
			local:      nil,
			saved:      nil,
			wantOnDisk: nil,
		},
		{
			name:       "saved state is written to a fresh directory",
			local:      nil,
			saved:      util.Ptr(state("a", 3)),
			wantOnDisk: util.Ptr(state("a", 3)),
		},
		{
			name:       "saved state replaces an older local one",
			local:      util.Ptr(state("a", 2)),
			saved:      util.Ptr(state("a", 3)),
			wantOnDisk: util.Ptr(state("a", 3)),
		},
		{
			name:       "a newer local state whose save failed is kept",
			local:      util.Ptr(state("a", 4)),
			saved:      util.Ptr(state("a", 3)),
			wantOnDisk: util.Ptr(state("a", 4)),
		},
		{
			name:       "a local state of another lineage is replaced",
			local:      util.Ptr(state("b", 9)),
			saved:      util.Ptr(state("a", 3)),
			wantOnDisk: util.Ptr(state("a", 3)),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &TerraformInstanceConfig{
				terraformInstance: &v0.TerraformInstance{StateDocument: tc.saved},
				tfDirName:         t.TempDir(),
			}
			if tc.local != nil {
				if err := os.WriteFile(c.tfStateFilepath(), []byte(*tc.local), 0644); err != nil {
					t.Fatal(err)
				}
			}

			if err := c.restoreTerraformState(); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			onDisk, err := os.ReadFile(c.tfStateFilepath())
			if tc.wantOnDisk == nil {
				if err == nil {
					t.Errorf("expected no state file, found %s", onDisk)
				}
				return
			}
			if err != nil {
				t.Fatalf("expected a state file: %v", err)
			}
			if string(onDisk) != *tc.wantOnDisk {
				t.Errorf("state on disk = %s, want %s", onDisk, *tc.wantOnDisk)
			}
		})
	}
}
