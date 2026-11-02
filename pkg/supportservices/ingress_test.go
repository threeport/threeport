package supportservices

import (
	"errors"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	api "github.com/threeport/threeport/pkg/api/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// fakeCaller records the calls the shared install and release make.
type fakeCaller struct {
	ready      bool
	gone       bool
	readyErr   error
	claimErr   error
	cleared    bool
	clearErr   error
	deleteErr  error
	installErr error
	recordErr  error
	deploys    int
	installID  uint
	claimed    []uint
	deleted    []uint
	recorded   *uint
}

// IsComponentReconciled returns the readiness the test arranged.
func (f *fakeCaller) IsComponentReconciled(uint) (bool, bool, error) {
	// return the arranged readiness
	return f.ready, f.gone, f.readyErr
}

// SetCallerKubernetesWorkloadInstanceID records the workload id the caller stored.
func (f *fakeCaller) SetCallerKubernetesWorkloadInstanceID(id *uint) error {
	// store the id
	f.recorded = id
	return f.recordErr
}

// ClaimRuntimeKubernetesWorkloadInstanceID records the workload id and returns the arranged error.
func (f *fakeCaller) ClaimRuntimeKubernetesWorkloadInstanceID(_ uint, workloadID uint) error {
	// record the claim
	f.claimed = append(f.claimed, workloadID)
	return f.claimErr
}

// ClearRuntimeKubernetesWorkloadInstanceID returns the clear result the test arranged.
func (f *fakeCaller) ClearRuntimeKubernetesWorkloadInstanceID(uint, uint) (bool, error) {
	// return the arranged clear result
	return f.cleared, f.clearErr
}

// GetReconciler returns no reconciler.
func (f *fakeCaller) GetReconciler() *controller.Reconciler {
	return nil
}

// Install records one install and returns the arranged id.
func (f *fakeCaller) Install(*api.KubernetesRuntimeInstance, *logr.Logger) (uint, error) {
	// record the install
	f.deploys++
	return f.installID, f.installErr
}

// DeleteComponentKubernetesWorkloadInstance records the workload id and returns the arranged error.
func (f *fakeCaller) DeleteComponentKubernetesWorkloadInstance(id uint) error {
	// record the delete
	f.deleted = append(f.deleted, id)
	return f.deleteErr
}

// runtimeWithSlot returns a runtime whose component slot holds id.
func runtimeWithSlot(id *uint) *api.KubernetesRuntimeInstance {
	// build the runtime
	return &api.KubernetesRuntimeInstance{
		Common:                     api.Common{ID: util.Ptr(uint(1))},
		IngressComponentInstanceID: id,
	}
}

// TestEnsureComponentRecordsAReconciledWorkload keeps the recorded workload.
func TestEnsureComponentRecordsAReconciledWorkload(t *testing.T) {
	// arrange a reconciled workload already in the slot
	caller := &fakeCaller{ready: true}
	runtime := runtimeWithSlot(util.Ptr(uint(4)))

	// ensure
	err := ensure(runtime, &runtime.IngressComponentInstanceID, "gateway component", caller, nil)
	if err != nil {
		t.Fatal(err)
	}
	if caller.deploys != 0 {
		t.Fatalf("deploys = %d, want 0", caller.deploys)
	}
	if caller.recorded == nil || *caller.recorded != 4 {
		t.Fatalf("recorded = %v, want 4", caller.recorded)
	}
}

// TestEnsureComponentWaitsForReconciliation leaves a live workload in place.
func TestEnsureComponentWaitsForReconciliation(t *testing.T) {
	// arrange a workload that exists and has not reconciled
	caller := &fakeCaller{}
	runtime := runtimeWithSlot(util.Ptr(uint(4)))

	// ensure
	err := ensure(runtime, &runtime.IngressComponentInstanceID, "gateway component", caller, nil)
	if err == nil || !strings.Contains(err.Error(), "not reconciled") {
		t.Fatalf("got %v, want not reconciled", err)
	}
	if caller.deploys != 0 {
		t.Fatalf("deploys = %d, want 0", caller.deploys)
	}
}

// TestEnsureComponentInstallsWhenTheSlotIsEmpty claims the new workload.
func TestEnsureComponentInstallsWhenTheSlotIsEmpty(t *testing.T) {
	// arrange an empty slot
	caller := &fakeCaller{installID: 8}
	runtime := runtimeWithSlot(nil)

	// ensure
	err := ensure(runtime, &runtime.IngressComponentInstanceID, "gateway component", caller, nil)
	if err == nil || !strings.Contains(err.Error(), "not reconciled") {
		t.Fatalf("got %v, want not reconciled", err)
	}
	if caller.deploys != 1 || len(caller.claimed) != 1 || caller.claimed[0] != 8 {
		t.Fatalf("deploys %d claims %v", caller.deploys, caller.claimed)
	}
	if runtime.IngressComponentInstanceID == nil || *runtime.IngressComponentInstanceID != 8 {
		t.Fatalf("slot = %v, want 8", runtime.IngressComponentInstanceID)
	}
}

// TestEnsureComponentDeletesALostClaim removes the workload the compare refused.
func TestEnsureComponentDeletesALostClaim(t *testing.T) {
	// arrange a claim the compare refuses
	caller := &fakeCaller{installID: 8, claimErr: errors.New("slot held")}
	runtime := runtimeWithSlot(nil)

	// ensure
	err := ensure(runtime, &runtime.IngressComponentInstanceID, "gateway component", caller, nil)
	if err == nil || !strings.Contains(err.Error(), "slot held") {
		t.Fatalf("got %v, want slot held", err)
	}
	if len(caller.deleted) != 1 || caller.deleted[0] != 8 {
		t.Fatalf("deleted %v, want [8]", caller.deleted)
	}
	if runtime.IngressComponentInstanceID != nil {
		t.Fatal("expected the slot to stay empty")
	}
}

// TestReleaseComponentDeletesTheLastWorkload clears the slot and deletes the workload.
func TestReleaseComponentDeletesTheLastWorkload(t *testing.T) {
	// arrange a clear the compare allows
	caller := &fakeCaller{cleared: true}
	runtime := runtimeWithSlot(util.Ptr(uint(4)))

	// release
	err := release(*runtime.ID, &runtime.IngressComponentInstanceID, "gateway component", caller)
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.deleted) != 1 || caller.deleted[0] != 4 {
		t.Fatalf("deleted %v, want [4]", caller.deleted)
	}
	if runtime.IngressComponentInstanceID != nil {
		t.Fatal("expected the slot to clear")
	}
}

// TestReleaseComponentKeepsAHeldSlot leaves the workload when the clear is refused.
func TestReleaseComponentKeepsAHeldSlot(t *testing.T) {
	// arrange a clear the compare refuses
	caller := &fakeCaller{}
	runtime := runtimeWithSlot(util.Ptr(uint(4)))

	// release
	err := release(*runtime.ID, &runtime.IngressComponentInstanceID, "gateway component", caller)
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.deleted) != 0 {
		t.Fatalf("deleted %v, want none", caller.deleted)
	}
	if runtime.IngressComponentInstanceID == nil || *runtime.IngressComponentInstanceID != 4 {
		t.Fatalf("slot = %v, want 4", runtime.IngressComponentInstanceID)
	}
}

// TestReleaseComponentRestoresTheSlotWhenDeleteFails puts the workload id back.
func TestReleaseComponentRestoresTheSlotWhenDeleteFails(t *testing.T) {
	// arrange a delete that fails after the clear
	caller := &fakeCaller{cleared: true, deleteErr: errors.New("delete failed")}
	runtime := runtimeWithSlot(util.Ptr(uint(4)))

	// release
	err := release(*runtime.ID, &runtime.IngressComponentInstanceID, "gateway component", caller)
	if err == nil || !strings.Contains(err.Error(), "delete failed") {
		t.Fatalf("got %v, want delete failed", err)
	}
	if len(caller.claimed) != 1 || caller.claimed[0] != 4 {
		t.Fatalf("claims %v, want [4]", caller.claimed)
	}
	if runtime.IngressComponentInstanceID == nil || *runtime.IngressComponentInstanceID != 4 {
		t.Fatalf("slot = %v, want 4", runtime.IngressComponentInstanceID)
	}
}

// TestEnsureDatabaseComponentClaimsTheSlot stores the installed id on the database column.
func TestEnsureDatabaseComponentClaimsTheSlot(t *testing.T) {
	// install into an empty database slot
	runtime := &api.KubernetesRuntimeInstance{Common: api.Common{ID: util.Ptr(uint(1))}}
	caller := &fakeCaller{installID: 11}

	// ensure
	err := EnsureDatabaseComponent(runtime, caller, nil)
	if err == nil || !strings.Contains(err.Error(), "not reconciled") {
		t.Fatalf("got %v, want not reconciled", err)
	}
	if runtime.DatabaseComponentInstanceID == nil || *runtime.DatabaseComponentInstanceID != 11 {
		t.Fatalf("slot = %v, want 11", runtime.DatabaseComponentInstanceID)
	}
}

// TestReleaseDatabaseComponentClearsTheSlot deletes the workload and nils the column.
func TestReleaseDatabaseComponentClearsTheSlot(t *testing.T) {
	// release the only holder
	runtime := &api.KubernetesRuntimeInstance{
		Common:                      api.Common{ID: util.Ptr(uint(1))},
		DatabaseComponentInstanceID: util.Ptr(uint(11)),
	}
	caller := &fakeCaller{cleared: true}

	// release
	if err := ReleaseDatabaseComponent(runtime, caller); err != nil {
		t.Fatal(err)
	}
	if runtime.DatabaseComponentInstanceID != nil {
		t.Fatalf("slot = %v, want nil", runtime.DatabaseComponentInstanceID)
	}
	if len(caller.deleted) != 1 || caller.deleted[0] != 11 {
		t.Fatalf("deleted %v, want [11]", caller.deleted)
	}
}

// TestEnsureSecretsComponentClaimsTheSlot stores the installed id on the secrets column.
func TestEnsureSecretsComponentClaimsTheSlot(t *testing.T) {
	// install into an empty secrets slot
	runtime := &api.KubernetesRuntimeInstance{Common: api.Common{ID: util.Ptr(uint(1))}}
	caller := &fakeCaller{installID: 12}

	// ensure
	err := EnsureSecretsComponent(runtime, caller, nil)
	if err == nil || !strings.Contains(err.Error(), "not reconciled") {
		t.Fatalf("got %v, want not reconciled", err)
	}
	if runtime.SecretsComponentInstanceID == nil || *runtime.SecretsComponentInstanceID != 12 {
		t.Fatalf("slot = %v, want 12", runtime.SecretsComponentInstanceID)
	}
}

// TestReleaseSecretsComponentClearsTheSlot deletes the workload and nils the column.
func TestReleaseSecretsComponentClearsTheSlot(t *testing.T) {
	// release the only holder
	runtime := &api.KubernetesRuntimeInstance{
		Common:                     api.Common{ID: util.Ptr(uint(1))},
		SecretsComponentInstanceID: util.Ptr(uint(12)),
	}
	caller := &fakeCaller{cleared: true}

	// release
	if err := ReleaseSecretsComponent(runtime, caller); err != nil {
		t.Fatal(err)
	}
	if runtime.SecretsComponentInstanceID != nil {
		t.Fatalf("slot = %v, want nil", runtime.SecretsComponentInstanceID)
	}
	if len(caller.deleted) != 1 || caller.deleted[0] != 12 {
		t.Fatalf("deleted %v, want [12]", caller.deleted)
	}
}

// TestEnsureComponentReinstallsWhenTheWorkloadIsGone installs again when the recorded row is gone.
func TestEnsureComponentReinstallsWhenTheWorkloadIsGone(t *testing.T) {
	// arrange a slot that names a deleted workload row
	caller := &fakeCaller{gone: true, installID: 9}
	runtime := runtimeWithSlot(util.Ptr(uint(4)))

	// ensure
	err := ensure(runtime, &runtime.IngressComponentInstanceID, "gateway component", caller, nil)
	if err == nil || !strings.Contains(err.Error(), "not reconciled") {
		t.Fatalf("got %v, want not reconciled", err)
	}
	if caller.deploys != 1 {
		t.Fatalf("deploys = %d, want 1", caller.deploys)
	}
	if runtime.IngressComponentInstanceID == nil || *runtime.IngressComponentInstanceID != 9 {
		t.Fatalf("slot = %v, want 9", runtime.IngressComponentInstanceID)
	}
}

// TestEnsureComponentReturnsTheDeleteError reports the delete after the claim fails.
func TestEnsureComponentReturnsTheDeleteError(t *testing.T) {
	// arrange a lost claim whose workload also fails to delete
	caller := &fakeCaller{
		installID: 8,
		claimErr:  errors.New("slot held"),
		deleteErr: errors.New("delete failed"),
	}
	runtime := runtimeWithSlot(nil)

	// ensure
	err := ensure(runtime, &runtime.IngressComponentInstanceID, "gateway component", caller, nil)
	if err == nil || !strings.Contains(err.Error(), "delete failed") {
		t.Fatalf("got %v, want delete failed", err)
	}
	if runtime.IngressComponentInstanceID != nil {
		t.Fatal("expected the slot to stay empty")
	}
}

// TestEnsureComponentRejectsAMissingRuntime covers a nil runtime and a runtime with no id.
func TestEnsureComponentRejectsAMissingRuntime(t *testing.T) {
	// reject a nil runtime
	err := EnsureIngressComponent(nil, &fakeCaller{}, nil)
	if err == nil || !strings.Contains(err.Error(), "ingress component runtime is missing") {
		t.Fatalf("gateway: %v", err)
	}
	err = EnsureSecretsComponent(nil, &fakeCaller{}, nil)
	if err == nil || !strings.Contains(err.Error(), "secrets component runtime is missing") {
		t.Fatalf("secrets: %v", err)
	}
	err = EnsureDatabaseComponent(nil, &fakeCaller{}, nil)
	if err == nil || !strings.Contains(err.Error(), "database component runtime is missing") {
		t.Fatalf("database: %v", err)
	}
	err = EnsureCertificatesComponent(nil, &fakeCaller{}, nil)
	if err == nil || !strings.Contains(err.Error(), "certificates component runtime is missing") {
		t.Fatalf("dns: %v", err)
	}

	// reject a runtime that has no id
	runtime := &api.KubernetesRuntimeInstance{}
	err = EnsureIngressComponent(runtime, &fakeCaller{}, nil)
	if err == nil || !strings.Contains(err.Error(), "ingress component runtime is missing") {
		t.Fatalf("gateway id: %v", err)
	}
	err = EnsureSecretsComponent(runtime, &fakeCaller{}, nil)
	if err == nil || !strings.Contains(err.Error(), "secrets component runtime is missing") {
		t.Fatalf("secrets id: %v", err)
	}
	err = EnsureDatabaseComponent(runtime, &fakeCaller{}, nil)
	if err == nil || !strings.Contains(err.Error(), "database component runtime is missing") {
		t.Fatalf("database id: %v", err)
	}
	err = EnsureCertificatesComponent(runtime, &fakeCaller{}, nil)
	if err == nil || !strings.Contains(err.Error(), "certificates component runtime is missing") {
		t.Fatalf("dns id: %v", err)
	}
	err = ReleaseIngressComponent(runtime, &fakeCaller{})
	if err == nil || !strings.Contains(err.Error(), "ingress component runtime is missing") {
		t.Fatalf("gateway release id: %v", err)
	}
	err = ReleaseSecretsComponent(runtime, &fakeCaller{})
	if err == nil || !strings.Contains(err.Error(), "secrets component runtime is missing") {
		t.Fatalf("secrets release id: %v", err)
	}
	err = ReleaseDatabaseComponent(runtime, &fakeCaller{})
	if err == nil || !strings.Contains(err.Error(), "database component runtime is missing") {
		t.Fatalf("database release id: %v", err)
	}
	err = ReleaseCertificatesComponent(runtime, &fakeCaller{})
	if err == nil || !strings.Contains(err.Error(), "certificates component runtime is missing") {
		t.Fatalf("dns release id: %v", err)
	}
	err = ReleaseIngressComponent(nil, &fakeCaller{})
	if err == nil || !strings.Contains(err.Error(), "ingress component runtime is missing") {
		t.Fatalf("gateway release: %v", err)
	}
	err = ReleaseSecretsComponent(nil, &fakeCaller{})
	if err == nil || !strings.Contains(err.Error(), "secrets component runtime is missing") {
		t.Fatalf("secrets release: %v", err)
	}
	err = ReleaseDatabaseComponent(nil, &fakeCaller{})
	if err == nil || !strings.Contains(err.Error(), "database component runtime is missing") {
		t.Fatalf("database release: %v", err)
	}
	err = ReleaseCertificatesComponent(nil, &fakeCaller{})
	if err == nil || !strings.Contains(err.Error(), "certificates component runtime is missing") {
		t.Fatalf("dns release: %v", err)
	}
}

// TestEnsureCertificatesComponentClaimsTheSlot stores the installed id on the dns column.
func TestEnsureCertificatesComponentClaimsTheSlot(t *testing.T) {
	// install into an empty dns slot
	runtime := &api.KubernetesRuntimeInstance{Common: api.Common{ID: util.Ptr(uint(1))}}
	caller := &fakeCaller{installID: 13}

	// ensure
	err := EnsureCertificatesComponent(runtime, caller, nil)
	if err == nil || !strings.Contains(err.Error(), "not reconciled") {
		t.Fatalf("got %v, want not reconciled", err)
	}
	if runtime.CertificatesComponentInstanceID == nil || *runtime.CertificatesComponentInstanceID != 13 {
		t.Fatalf("slot = %v, want 13", runtime.CertificatesComponentInstanceID)
	}
}

// TestReleaseCertificatesComponentClearsTheSlot deletes the workload and nils the column.
func TestReleaseCertificatesComponentClearsTheSlot(t *testing.T) {
	// release the only holder
	runtime := &api.KubernetesRuntimeInstance{
		Common:                          api.Common{ID: util.Ptr(uint(1))},
		CertificatesComponentInstanceID: util.Ptr(uint(13)),
	}
	caller := &fakeCaller{cleared: true}

	// release
	if err := ReleaseCertificatesComponent(runtime, caller); err != nil {
		t.Fatal(err)
	}
	if runtime.CertificatesComponentInstanceID != nil {
		t.Fatalf("slot = %v, want nil", runtime.CertificatesComponentInstanceID)
	}
	if len(caller.deleted) != 1 || caller.deleted[0] != 13 {
		t.Fatalf("deleted %v, want [13]", caller.deleted)
	}
}

// TestReleaseComponentLeavesAnEmptySlot does nothing when the slot is empty.
func TestReleaseComponentLeavesAnEmptySlot(t *testing.T) {
	// arrange an empty slot
	caller := &fakeCaller{}
	runtime := runtimeWithSlot(nil)

	// release
	err := release(*runtime.ID, &runtime.IngressComponentInstanceID, "gateway component", caller)
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.deleted) != 0 {
		t.Fatalf("deleted %v, want none", caller.deleted)
	}
}

// TestEnsureComponentReturnsAReadError returns the failure from reading the workload.
func TestEnsureComponentReturnsAReadError(t *testing.T) {
	// arrange a slot whose workload read fails
	caller := &fakeCaller{readyErr: errors.New("read failed")}
	runtime := runtimeWithSlot(util.Ptr(uint(4)))

	// ensure
	err := ensure(runtime, &runtime.IngressComponentInstanceID, "gateway component", caller, nil)
	if err == nil || !strings.Contains(err.Error(), "failed to read") {
		t.Fatalf("got %v, want failed to read", err)
	}
	if caller.deploys != 0 {
		t.Fatalf("deploys = %d, want 0", caller.deploys)
	}
}

// TestEnsureComponentReturnsAnInstallError returns the failure from installing the workload.
func TestEnsureComponentReturnsAnInstallError(t *testing.T) {
	// arrange an install that fails
	caller := &fakeCaller{installErr: errors.New("install failed")}
	runtime := runtimeWithSlot(nil)

	// ensure
	err := ensure(runtime, &runtime.IngressComponentInstanceID, "gateway component", caller, nil)
	if err == nil || !strings.Contains(err.Error(), "install failed") {
		t.Fatalf("got %v, want install failed", err)
	}
	if runtime.IngressComponentInstanceID != nil {
		t.Fatal("expected the slot to stay empty")
	}
}

// TestEnsureComponentReturnsARecordError returns the failure from recording the workload.
func TestEnsureComponentReturnsARecordError(t *testing.T) {
	// arrange a record that fails after the claim
	caller := &fakeCaller{installID: 8, recordErr: errors.New("record failed")}
	runtime := runtimeWithSlot(nil)

	// ensure
	err := ensure(runtime, &runtime.IngressComponentInstanceID, "gateway component", caller, nil)
	if err == nil || !strings.Contains(err.Error(), "failed to record") {
		t.Fatalf("got %v, want failed to record", err)
	}
	if runtime.IngressComponentInstanceID == nil || *runtime.IngressComponentInstanceID != 8 {
		t.Fatalf("slot = %v, want 8", runtime.IngressComponentInstanceID)
	}
}

// TestEnsureComponentRecordFailsWhenReady returns the record failure for a reconciled workload.
func TestEnsureComponentRecordFailsWhenReady(t *testing.T) {
	// arrange a reconciled workload whose record fails
	caller := &fakeCaller{ready: true, recordErr: errors.New("record failed")}
	runtime := runtimeWithSlot(util.Ptr(uint(4)))

	// ensure
	err := ensure(runtime, &runtime.IngressComponentInstanceID, "gateway component", caller, nil)
	if err == nil || !strings.Contains(err.Error(), "failed to record") {
		t.Fatalf("got %v, want failed to record", err)
	}
	if caller.deploys != 0 {
		t.Fatalf("deploys = %d, want 0", caller.deploys)
	}
}

// TestReleaseComponentReturnsAClearError returns the failure from clearing the slot.
func TestReleaseComponentReturnsAClearError(t *testing.T) {
	// arrange a clear that fails
	caller := &fakeCaller{clearErr: errors.New("clear failed")}
	runtime := runtimeWithSlot(util.Ptr(uint(4)))

	// release
	err := release(*runtime.ID, &runtime.IngressComponentInstanceID, "gateway component", caller)
	if err == nil || !strings.Contains(err.Error(), "failed to clear") {
		t.Fatalf("got %v, want failed to clear", err)
	}
	if len(caller.deleted) != 0 {
		t.Fatalf("deleted %v, want none", caller.deleted)
	}
}

// TestReleaseComponentJoinsTheRestoreError returns the delete failure and the restore failure.
func TestReleaseComponentJoinsTheRestoreError(t *testing.T) {
	// arrange a delete that fails and a restore that also fails
	caller := &fakeCaller{
		cleared:   true,
		deleteErr: errors.New("delete failed"),
		claimErr:  errors.New("slot held"),
	}
	runtime := runtimeWithSlot(util.Ptr(uint(4)))

	// release
	err := release(*runtime.ID, &runtime.IngressComponentInstanceID, "gateway component", caller)
	if err == nil ||
		!strings.Contains(err.Error(), "delete failed") ||
		!strings.Contains(err.Error(), "slot held") {
		t.Fatalf("got %v, want both failures", err)
	}
	if runtime.IngressComponentInstanceID == nil || *runtime.IngressComponentInstanceID != 4 {
		t.Fatalf("slot = %v, want 4", runtime.IngressComponentInstanceID)
	}
}
