package supportservices

import (
	"errors"
	"strings"
	"testing"

	api "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// fakeIngress records the calls EnsureIngress and ReleaseIngress make.
type fakeIngress struct {
	ready     bool
	gone      bool
	readyErr  error
	claimErr  error
	cleared   bool
	clearErr  error
	deleteErr error
	deploys   int
	claimed   []uint
	deleted   []uint
	recorded  *uint
}

// Ready returns the readiness the test arranged.
func (f *fakeIngress) Ready(uint) (bool, bool, error) {
	// return the arranged readiness
	return f.ready, f.gone, f.readyErr
}

// SetWorkloadID records the workload id the caller stored.
func (f *fakeIngress) SetWorkloadID(id *uint) error {
	// store the id
	f.recorded = id
	return nil
}

// Claim records the workload id and returns the arranged error.
func (f *fakeIngress) Claim(_ uint, workloadID uint) error {
	// record the claim
	f.claimed = append(f.claimed, workloadID)
	return f.claimErr
}

// Clear returns the clear result the test arranged.
func (f *fakeIngress) Clear(uint, uint) (bool, error) {
	// return the arranged clear result
	return f.cleared, f.clearErr
}

// DeleteWorkload records the workload id and returns the arranged error.
func (f *fakeIngress) DeleteWorkload(id uint) error {
	// record the delete
	f.deleted = append(f.deleted, id)
	return f.deleteErr
}

// runtimeWithSlot returns a runtime whose controller slot holds id.
func runtimeWithSlot(id *uint) *api.KubernetesRuntimeInstance {
	// build the runtime
	return &api.KubernetesRuntimeInstance{
		Common:                      api.Common{ID: util.Ptr(uint(1))},
		GatewayControllerInstanceID: id,
	}
}

// TestEnsureIngressRecordsAReconciledWorkload keeps the recorded workload.
func TestEnsureIngressRecordsAReconciledWorkload(t *testing.T) {
	// arrange a reconciled workload already in the slot
	caller := &fakeIngress{ready: true}
	runtime := runtimeWithSlot(util.Ptr(uint(4)))

	// ensure
	err := Ensure(*runtime.ID, &runtime.GatewayControllerInstanceID, "gateway controller", caller, func() (uint, error) {
		caller.deploys++
		return 8, nil
	})
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

// TestEnsureIngressWaitsForReconciliation leaves a live workload in place.
func TestEnsureIngressWaitsForReconciliation(t *testing.T) {
	// arrange a workload that exists and has not reconciled
	caller := &fakeIngress{}
	runtime := runtimeWithSlot(util.Ptr(uint(4)))

	// ensure
	err := Ensure(*runtime.ID, &runtime.GatewayControllerInstanceID, "gateway controller", caller, func() (uint, error) {
		caller.deploys++
		return 8, nil
	})
	if err == nil || !strings.Contains(err.Error(), "not reconciled") {
		t.Fatalf("got %v, want not reconciled", err)
	}
	if caller.deploys != 0 {
		t.Fatalf("deploys = %d, want 0", caller.deploys)
	}
}

// TestEnsureIngressInstallsWhenTheSlotIsEmpty claims the new workload.
func TestEnsureIngressInstallsWhenTheSlotIsEmpty(t *testing.T) {
	// arrange an empty slot
	caller := &fakeIngress{}
	runtime := runtimeWithSlot(nil)

	// ensure
	err := Ensure(*runtime.ID, &runtime.GatewayControllerInstanceID, "gateway controller", caller, func() (uint, error) {
		caller.deploys++
		return 8, nil
	})
	if err == nil || !strings.Contains(err.Error(), "not reconciled") {
		t.Fatalf("got %v, want not reconciled", err)
	}
	if caller.deploys != 1 || len(caller.claimed) != 1 || caller.claimed[0] != 8 {
		t.Fatalf("deploys %d claims %v", caller.deploys, caller.claimed)
	}
	if runtime.GatewayControllerInstanceID == nil || *runtime.GatewayControllerInstanceID != 8 {
		t.Fatalf("slot = %v, want 8", runtime.GatewayControllerInstanceID)
	}
}

// TestEnsureIngressDeletesALostClaim removes the workload the compare refused.
func TestEnsureIngressDeletesALostClaim(t *testing.T) {
	// arrange a claim the compare refuses
	caller := &fakeIngress{claimErr: errors.New("slot held")}
	runtime := runtimeWithSlot(nil)

	// ensure
	err := Ensure(*runtime.ID, &runtime.GatewayControllerInstanceID, "gateway controller", caller, func() (uint, error) {
		caller.deploys++
		return 8, nil
	})
	if err == nil || !strings.Contains(err.Error(), "slot held") {
		t.Fatalf("got %v, want slot held", err)
	}
	if len(caller.deleted) != 1 || caller.deleted[0] != 8 {
		t.Fatalf("deleted %v, want [8]", caller.deleted)
	}
	if runtime.GatewayControllerInstanceID != nil {
		t.Fatal("expected the slot to stay empty")
	}
}

// TestReleaseIngressDeletesTheLastWorkload clears the slot and deletes the workload.
func TestReleaseIngressDeletesTheLastWorkload(t *testing.T) {
	// arrange a clear the compare allows
	caller := &fakeIngress{cleared: true}
	runtime := runtimeWithSlot(util.Ptr(uint(4)))

	// release
	err := Release(*runtime.ID, &runtime.GatewayControllerInstanceID, "gateway controller", caller)
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.deleted) != 1 || caller.deleted[0] != 4 {
		t.Fatalf("deleted %v, want [4]", caller.deleted)
	}
	if runtime.GatewayControllerInstanceID != nil {
		t.Fatal("expected the slot to clear")
	}
}

// TestReleaseIngressKeepsAHeldSlot leaves the workload when the clear is refused.
func TestReleaseIngressKeepsAHeldSlot(t *testing.T) {
	// arrange a clear the compare refuses
	caller := &fakeIngress{}
	runtime := runtimeWithSlot(util.Ptr(uint(4)))

	// release
	err := Release(*runtime.ID, &runtime.GatewayControllerInstanceID, "gateway controller", caller)
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.deleted) != 0 {
		t.Fatalf("deleted %v, want none", caller.deleted)
	}
	if runtime.GatewayControllerInstanceID == nil || *runtime.GatewayControllerInstanceID != 4 {
		t.Fatalf("slot = %v, want 4", runtime.GatewayControllerInstanceID)
	}
}

// TestReleaseIngressRestoresTheSlotWhenDeleteFails puts the workload id back.
func TestReleaseIngressRestoresTheSlotWhenDeleteFails(t *testing.T) {
	// arrange a delete that fails after the clear
	caller := &fakeIngress{cleared: true, deleteErr: errors.New("delete failed")}
	runtime := runtimeWithSlot(util.Ptr(uint(4)))

	// release
	err := Release(*runtime.ID, &runtime.GatewayControllerInstanceID, "gateway controller", caller)
	if err == nil || !strings.Contains(err.Error(), "delete failed") {
		t.Fatalf("got %v, want delete failed", err)
	}
	if len(caller.claimed) != 1 || caller.claimed[0] != 4 {
		t.Fatalf("claims %v, want [4]", caller.claimed)
	}
	if runtime.GatewayControllerInstanceID == nil || *runtime.GatewayControllerInstanceID != 4 {
		t.Fatalf("slot = %v, want 4", runtime.GatewayControllerInstanceID)
	}
}

// TestEnsureClaimsTheDatabaseSlot stores the installed id on the database column.
func TestEnsureClaimsTheDatabaseSlot(t *testing.T) {
	// install into an empty database slot
	runtime := &api.KubernetesRuntimeInstance{Common: api.Common{ID: util.Ptr(uint(1))}}
	caller := &fakeIngress{}
	err := Ensure(*runtime.ID, &runtime.DatabaseControllerInstanceID, "database controller", caller, func() (uint, error) {
		return 11, nil
	})
	if err == nil || !strings.Contains(err.Error(), "not reconciled") {
		t.Fatalf("got %v, want not reconciled", err)
	}
	if runtime.DatabaseControllerInstanceID == nil || *runtime.DatabaseControllerInstanceID != 11 {
		t.Fatalf("slot = %v, want 11", runtime.DatabaseControllerInstanceID)
	}
}

// TestReleaseClearsTheDatabaseSlot deletes the workload and nils the column.
func TestReleaseClearsTheDatabaseSlot(t *testing.T) {
	// release the only holder
	runtime := &api.KubernetesRuntimeInstance{
		Common:                       api.Common{ID: util.Ptr(uint(1))},
		DatabaseControllerInstanceID: util.Ptr(uint(11)),
	}
	caller := &fakeIngress{cleared: true}
	if err := Release(*runtime.ID, &runtime.DatabaseControllerInstanceID, "database controller", caller); err != nil {
		t.Fatal(err)
	}
	if runtime.DatabaseControllerInstanceID != nil {
		t.Fatalf("slot = %v, want nil", runtime.DatabaseControllerInstanceID)
	}
	if len(caller.deleted) != 1 || caller.deleted[0] != 11 {
		t.Fatalf("deleted %v, want [11]", caller.deleted)
	}
}
