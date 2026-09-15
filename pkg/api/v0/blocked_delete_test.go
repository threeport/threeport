package v0

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	lib "github.com/threeport/threeport/pkg/api/lib/v0"
	auth "github.com/threeport/threeport/pkg/auth/v0"
)

// TestFormatObjectPath covers the renderer that turns an AOR's raw
// FQT + id (and optional id->name map) into the
// "<api-namespace>/<kebab-kind>/<name-or-id>" form rendered into
// blocked-delete responses, events output, etc. Pure function; the
// table reads as a flat input -> output mapping.
func TestFormatObjectPath(t *testing.T) {
	cases := []struct {
		name    string
		rawType string
		id      uint
		names   map[uint]string
		want    string
	}{
		{
			name:    "name resolved",
			rawType: "threeport.io/v0.KubernetesWorkloadInstance",
			id:      42,
			names:   map[uint]string{42: "my-workload"},
			want:    "threeport.io/kubernetes-workload-instance/my-workload",
		},
		{
			name:    "missing name falls back to id",
			rawType: "threeport.io/v0.KubernetesWorkloadInstance",
			id:      42,
			names:   map[uint]string{},
			want:    "threeport.io/kubernetes-workload-instance/42",
		},
		{
			name:    "empty name in map falls back to id",
			rawType: "threeport.io/v0.KubernetesWorkloadInstance",
			id:      42,
			names:   map[uint]string{42: ""},
			want:    "threeport.io/kubernetes-workload-instance/42",
		},
		{
			name:    "nil names map renders id",
			rawType: "threeport.io/v0.KubernetesWorkloadInstance",
			id:      42,
			names:   nil,
			want:    "threeport.io/kubernetes-workload-instance/42",
		},
		{
			name:    "module namespace renders with kebab kind",
			rawType: "example.com/v0.GadgetInstance",
			id:      7,
			names:   map[uint]string{7: "gadget-seven"},
			want:    "example.com/gadget-instance/gadget-seven",
		},
		{
			name:    "malformed FQT falls back to rawType",
			rawType: "Widget",
			id:      99,
			names:   map[uint]string{99: "ignored"},
			want:    "Widget/99",
		},
		{
			name:    "empty FQT also falls back",
			rawType: "",
			id:      1,
			want:    "/1",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatObjectPath(tc.rawType, tc.id, tc.names)
			assert.Equal(t, tc.want, got)
		})
	}
}

// makeRef is a tiny constructor for AttachedObjectReference test
// fixtures - the message renderer reads ObjectType/ObjectID and
// AttachedObjectType/AttachedObjectID, nothing else.
func makeRef(baseType string, baseID uint, attacherType string, attacherID uint) AttachedObjectReference {
	bt, at := baseType, attacherType
	bid, aid := baseID, attacherID
	return AttachedObjectReference{
		ObjectType:         &bt,
		ObjectID:           &bid,
		AttachedObjectType: &at,
		AttachedObjectID:   &aid,
	}
}

// TestFormatBlockedDelete covers the 409 message body. The function
// is called twice per error: once at error.Error() (no names map, all
// id-only paths), once at handler-level after the AOR list resolves
// names. Both paths matter so the table covers them and a few mixed
// scenarios.
func TestFormatBlockedDelete(t *testing.T) {
	const baseType = "threeport.io/v0.KubernetesWorkloadDefinition"
	const baseID = uint(5)
	const attacherType = "threeport.io/v0.KubernetesWorkloadInstance"

	cases := []struct {
		name        string
		refs        []AttachedObjectReference
		namesByType map[string]map[uint]string
		wantHas     []string // substrings the rendered message must contain
		wantCount   int
	}{
		{
			name: "single blocking ref - id only",
			refs: []AttachedObjectReference{
				makeRef(baseType, baseID, attacherType, 11),
			},
			wantHas: []string{
				"threeport.io/kubernetes-workload-definition/5",
				"cannot be deleted while 1 object(s) still reference it",
				"threeport.io/kubernetes-workload-instance/11",
				"Remove dependents first.",
			},
			wantCount: 1,
		},
		{
			name: "multiple blocking refs - id only",
			refs: []AttachedObjectReference{
				makeRef(baseType, baseID, attacherType, 11),
				makeRef(baseType, baseID, attacherType, 12),
				makeRef(baseType, baseID, attacherType, 13),
			},
			wantHas: []string{
				"cannot be deleted while 3 object(s)",
				"threeport.io/kubernetes-workload-instance/11",
				"threeport.io/kubernetes-workload-instance/12",
				"threeport.io/kubernetes-workload-instance/13",
			},
			wantCount: 3,
		},
		{
			name: "names resolved for base and attachers",
			refs: []AttachedObjectReference{
				makeRef(baseType, baseID, attacherType, 11),
			},
			namesByType: map[string]map[uint]string{
				baseType:     {baseID: "my-def"},
				attacherType: {11: "my-inst"},
			},
			wantHas: []string{
				"threeport.io/kubernetes-workload-definition/my-def",
				"threeport.io/kubernetes-workload-instance/my-inst",
			},
			wantCount: 1,
		},
		{
			name: "mixed name resolution - base resolved, attacher not",
			refs: []AttachedObjectReference{
				makeRef(baseType, baseID, attacherType, 11),
			},
			namesByType: map[string]map[uint]string{
				baseType: {baseID: "my-def"},
			},
			wantHas: []string{
				"threeport.io/kubernetes-workload-definition/my-def",
				"threeport.io/kubernetes-workload-instance/11", // id-only fallback for the attacher
			},
			wantCount: 1,
		},
		{
			name: "attachers spanning different types each render their own kebab kind",
			refs: []AttachedObjectReference{
				makeRef(baseType, baseID, "threeport.io/v0.KubernetesWorkloadInstance", 11),
				makeRef(baseType, baseID, "threeport.io/v0.GatewayInstance", 22),
			},
			wantHas: []string{
				"threeport.io/kubernetes-workload-instance/11",
				"threeport.io/gateway-instance/22",
			},
			wantCount: 2,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := &BlockedDeleteError{AttachedRefs: tc.refs}
			got := FormatBlockedDelete(err, tc.namesByType)
			for _, want := range tc.wantHas {
				assert.Contains(t, got, want, "rendered message should contain %q", want)
			}
			// Each attacher emits one bullet line; counting the leading
			// "  - " sequence pins both the formatting and the count.
			assert.Equal(t, tc.wantCount, strings.Count(got, "  - "), "one bullet per attacher")
		})
	}
}

// TestBlockedDeleteError_DefaultError verifies the Error() method
// renders an id-only message - the BlockedDeleteError type is
// returned from the hook without name resolution; the handler layer
// is responsible for upgrading the message before sending the 409.
func TestBlockedDeleteError_DefaultError(t *testing.T) {
	err := &BlockedDeleteError{
		AttachedRefs: []AttachedObjectReference{
			makeRef("threeport.io/v0.KubernetesWorkloadDefinition", 5, "threeport.io/v0.KubernetesWorkloadInstance", 11),
		},
	}
	msg := err.Error()
	assert.Contains(t, msg, "threeport.io/kubernetes-workload-definition/5", "base path should be id-only at error.Error() level")
	assert.Contains(t, msg, "threeport.io/kubernetes-workload-instance/11", "attacher path should be id-only at error.Error() level")
	assert.NotContains(t, msg, "my-", "Error() does not get a names map; no resolved name should leak")
}

// blockedDeleteBaseType and blockedDeleteAttacherType are the object and
// attacher FQTs the policy tests plant AORs with; only the type strings and
// ids matter to the blocking-policy functions.
const (
	blockedDeleteBaseType     = "threeport.io/v0.MachineRuntimeInstance"
	blockedDeleteBaseID       = uint(5)
	blockedDeleteAttacherType = "threeport.io/v0.GcpGceMachineRuntimeInstance"
)

// setupBlockedDeleteTestDB returns an in-memory sqlite db with only the
// AttachedObjectReference table migrated. The blocking-policy functions read
// that table directly, so no other model needs to be present.
func setupBlockedDeleteTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&AttachedObjectReference{}))
	return db
}

// plantBlockingAOR inserts one incoming AOR of the given relationship pointing
// at the base object, so the policy functions see it as a potential blocker.
func plantBlockingAOR(t *testing.T, db *gorm.DB, rel Relationship, attacherID uint) {
	t.Helper()
	r := rel
	bt := blockedDeleteBaseType
	bid := blockedDeleteBaseID
	at := blockedDeleteAttacherType
	aid := attacherID
	require.NoError(t, db.Create(&AttachedObjectReference{
		ObjectType:         &bt,
		ObjectID:           &bid,
		AttachedObjectType: &at,
		AttachedObjectID:   &aid,
		Relationship:       &r,
	}).Error)
}

// withCallerOU returns a db session whose context carries a CallerIdentity with
// the given organizational unit, so the policy functions read the intended
// caller scope off the gorm statement context.
func withCallerOU(db *gorm.DB, ou string) *gorm.DB {
	ctx := lib.WithCaller(context.Background(), lib.CallerIdentity{OrganizationalUnit: ou})
	return db.WithContext(ctx)
}

// TestFindBlockingAttachedObjectReferences_ExternalCaller asserts that an
// external (non control-plane) caller is blocked by an incoming requires, owns,
// or marries reference. This documents that the blocking policy treats all
// three relationships as blockers for callers outside the control plane.
func TestFindBlockingAttachedObjectReferences_ExternalCaller(t *testing.T) {
	cases := []struct {
		name string
		rel  Relationship
	}{
		// each relationship that blocks an external caller's delete
		{name: "requires blocks external", rel: RelationshipRequires},
		{name: "owns blocks external", rel: RelationshipOwns},
		{name: "marries blocks external", rel: RelationshipMarries},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// plant one incoming reference of the relationship under test
			db := setupBlockedDeleteTestDB(t)
			plantBlockingAOR(t, db, tc.rel, 11)

			// query the policy for an external caller
			blocking, err := findBlockingAttachedObjectReferences(
				db, blockedDeleteBaseType, uintPtr(blockedDeleteBaseID), "external",
			)
			require.NoError(t, err)

			// the reference must be reported as a blocker
			require.Len(t, blocking, 1, "external caller should be blocked by %s", tc.rel)
			require.NotNil(t, blocking[0].Relationship)
			assert.Equal(t, tc.rel, *blocking[0].Relationship)
		})
	}
}

// TestFindBlockingAttachedObjectReferences_ControlPlaneCaller asserts the
// control-plane carve-out: a control-plane caller is still blocked by requires,
// but under the carve-out is NOT blocked by owns or marries. This documents the
// current policy that lets control-plane components tear down objects they own
// or are married to.
func TestFindBlockingAttachedObjectReferences_ControlPlaneCaller(t *testing.T) {
	cases := []struct {
		name        string
		rel         Relationship
		wantBlocked bool
	}{
		// requires blocks the control plane like any other caller
		{name: "requires blocks control-plane", rel: RelationshipRequires, wantBlocked: true},
		// owns and marries are carved out for the control plane
		{name: "owns does not block control-plane", rel: RelationshipOwns, wantBlocked: false},
		{name: "marries does not block control-plane", rel: RelationshipMarries, wantBlocked: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// plant one incoming reference of the relationship under test
			db := setupBlockedDeleteTestDB(t)
			plantBlockingAOR(t, db, tc.rel, 11)

			// query the policy for a control-plane caller
			blocking, err := findBlockingAttachedObjectReferences(
				db, blockedDeleteBaseType, uintPtr(blockedDeleteBaseID), auth.OUControlPlane,
			)
			require.NoError(t, err)

			// requires still blocks; owns and marries are carved out
			if tc.wantBlocked {
				require.Len(t, blocking, 1, "control-plane caller should be blocked by %s", tc.rel)
				require.NotNil(t, blocking[0].Relationship)
				assert.Equal(t, tc.rel, *blocking[0].Relationship)
				return
			}
			assert.Empty(t, blocking, "control-plane caller should not be blocked by %s", tc.rel)
		})
	}
}

// machineRuntimeInstanceProbe is a minimal object whose FQT and id let
// CheckBlockingAttachedObjectReferences resolve the type string and id it
// queries the AOR table with.
type machineRuntimeInstanceProbe struct {
	ID *uint `gorm:"primaryKey"`
}

func (p *machineRuntimeInstanceProbe) GetFullyQualifiedType() string {
	return blockedDeleteBaseType
}

// TestCheckBlockingAttachedObjectReferences_ExternalCaller asserts that the
// handler-facing wrapper returns a BlockedDeleteError to an external caller for
// an incoming requires, owns, or marries reference, reading the caller scope off
// the gorm statement context.
func TestCheckBlockingAttachedObjectReferences_ExternalCaller(t *testing.T) {
	cases := []struct {
		name string
		rel  Relationship
	}{
		// each relationship that blocks an external caller's delete
		{name: "requires blocks external", rel: RelationshipRequires},
		{name: "owns blocks external", rel: RelationshipOwns},
		{name: "marries blocks external", rel: RelationshipMarries},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// plant one incoming reference and probe the base object by id
			db := setupBlockedDeleteTestDB(t)
			plantBlockingAOR(t, db, tc.rel, 11)
			probe := &machineRuntimeInstanceProbe{ID: uintPtr(blockedDeleteBaseID)}

			// check the policy for an external caller carried on the context
			err := CheckBlockingAttachedObjectReferences(withCallerOU(db, "external"), probe)

			// the wrapper must surface a populated BlockedDeleteError
			require.Error(t, err, "external caller should be blocked by %s", tc.rel)
			blockedErr, ok := err.(*BlockedDeleteError)
			require.True(t, ok, "error should be a BlockedDeleteError")
			require.Len(t, blockedErr.AttachedRefs, 1)
		})
	}
}

// TestCheckBlockingAttachedObjectReferences_ControlPlaneCaller asserts the
// control-plane carve-out through the handler-facing wrapper: requires still
// yields a BlockedDeleteError, while owns and marries pass with no error.
func TestCheckBlockingAttachedObjectReferences_ControlPlaneCaller(t *testing.T) {
	cases := []struct {
		name        string
		rel         Relationship
		wantBlocked bool
	}{
		// requires blocks the control plane like any other caller
		{name: "requires blocks control-plane", rel: RelationshipRequires, wantBlocked: true},
		// owns and marries are carved out for the control plane
		{name: "owns does not block control-plane", rel: RelationshipOwns, wantBlocked: false},
		{name: "marries does not block control-plane", rel: RelationshipMarries, wantBlocked: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// plant one incoming reference and probe the base object by id
			db := setupBlockedDeleteTestDB(t)
			plantBlockingAOR(t, db, tc.rel, 11)
			probe := &machineRuntimeInstanceProbe{ID: uintPtr(blockedDeleteBaseID)}

			// check the policy for a control-plane caller carried on the context
			err := CheckBlockingAttachedObjectReferences(
				withCallerOU(db, auth.OUControlPlane), probe,
			)

			// requires still blocks; owns and marries pass with no error
			if tc.wantBlocked {
				require.Error(t, err, "control-plane caller should be blocked by %s", tc.rel)
				_, ok := err.(*BlockedDeleteError)
				require.True(t, ok, "error should be a BlockedDeleteError")
				return
			}
			require.NoError(t, err, "control-plane caller should not be blocked by %s", tc.rel)
		})
	}
}
