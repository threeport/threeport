package v0

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	lib "github.com/threeport/threeport/pkg/api/lib/v0"
	auth "github.com/threeport/threeport/pkg/auth/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

const testAcmeWidgetFQT = "acme.corp/v0.Widget"

// testAcmeWidget is the owner row whose column the create sets.
type testAcmeWidget struct {
	ID         *uint `gorm:"primaryKey"`
	WorkloadID *uint
	DeletedAt  gorm.DeletedAt
}

func (testAcmeWidget) TableName() string { return "acme_corp_v0_widgets" }

func (c *testAcmeWidget) BeforeUpdate(tx *gorm.DB) error {
	return errOwnerUpdateRan
}

// errOwnerUpdateRan fails the test when the raw update takes the model path.
var errOwnerUpdateRan = errString("owner beforeUpdate ran")

type errString string

func (e errString) Error() string { return string(e) }

func setupOwnerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&testHolder{}, &testAcmeWidget{}, &AttachedObjectReference{}))
	return db
}

func TestTableForQualifiedType(t *testing.T) {
	core, err := tableForQualifiedType("threeport.io/v0.KubernetesWorkloadInstance")
	require.NoError(t, err)
	assert.Equal(t, "v0_kubernetes_workload_instances", core)

	module, err := tableForQualifiedType(testAcmeWidgetFQT)
	require.NoError(t, err)
	assert.Equal(t, "acme_corp_v0_widgets", module)

	_, err = tableForQualifiedType("not-a-type")
	require.Error(t, err)
}

func TestColumnForOwnerField(t *testing.T) {
	column, err := columnForOwnerField("KubernetesWorkloadInstanceID")
	require.NoError(t, err)
	assert.Equal(t, "kubernetes_workload_instance_id", column)

	_, err = columnForOwnerField("1Field")
	require.Error(t, err)

	_, err = columnForOwnerField("ID")
	require.Error(t, err)
	assert.ErrorContains(t, err, "primary key")
}

func TestRecordOwnerOnCreateSetsColumnAndReference(t *testing.T) {
	db := setupOwnerTestDB(t)
	owner := &testAcmeWidget{}
	require.NoError(t, db.Create(owner).Error)

	holder := &testHolder{
		Name: strPtr("owned"),
		owner: &OwnerRef{
			Type:  strPtr(testAcmeWidgetFQT),
			ID:    owner.ID,
			Field: strPtr("WorkloadID"),
		},
	}
	require.NoError(t, withThreeportOwner(db).Create(holder).Error)

	var stored testAcmeWidget
	require.NoError(t, db.First(&stored, *owner.ID).Error)
	require.NotNil(t, stored.WorkloadID)
	assert.Equal(t, *holder.ID, *stored.WorkloadID)

	var refs []AttachedObjectReference
	require.NoError(t, db.Where(
		"object_type = ? AND object_id = ? AND relationship = ?",
		testHolderFQT, *holder.ID, RelationshipOwns,
	).Find(&refs).Error)
	require.Len(t, refs, 1)
	assert.Equal(t, testAcmeWidgetFQT, *refs[0].AttachedObjectType)
	assert.Equal(t, *owner.ID, *refs[0].AttachedObjectID)
}

// TestRecordOwnerOnCreateRejectsOwnedRow covers a create that names an owner another object owns.
// An external caller is rejected and a control-plane caller is allowed.
func TestRecordOwnerOnCreateRejectsOwnedRow(t *testing.T) {
	db := setupOwnerTestDB(t)
	owner := &testAcmeWidget{}
	require.NoError(t, db.Create(owner).Error)
	holderType := "acme.corp/v0.Parent"
	holderID := uint(7)
	require.NoError(t, db.Create(&AttachedObjectReference{
		ObjectID:           owner.ID,
		ObjectType:         strPtr(testAcmeWidgetFQT),
		AttachedObjectID:   &holderID,
		AttachedObjectType: &holderType,
		Relationship:       relPtr(RelationshipOwns),
	}).Error)

	// an external create names that owned row
	external := db.WithContext(lib.WithCaller(context.Background(), lib.CallerIdentity{
		CommonName:   "acme-corp-threeport-module",
		Organization: "acme.corp",
	}))
	err := external.Create(&testHolder{
		Name: strPtr("blocked"),
		owner: &OwnerRef{
			Type:  strPtr(testAcmeWidgetFQT),
			ID:    owner.ID,
			Field: strPtr("WorkloadID"),
		},
	}).Error
	require.Error(t, err)
	assert.Contains(t, err.Error(), ErrMsgExternalUpdateBlocked)

	// the create rolled back, so the owner column is still empty
	var stored testAcmeWidget
	require.NoError(t, db.First(&stored, *owner.ID).Error)
	assert.Nil(t, stored.WorkloadID)

	// a control-plane caller in this organization writes the same owner
	allowed := db.WithContext(lib.WithCaller(context.Background(), lib.CallerIdentity{
		CommonName:         "workload-controller",
		Organization:       "acme.corp",
		OrganizationalUnit: "controlplane",
	}))
	require.NoError(t, allowed.Create(&testHolder{
		Name: strPtr("allowed"),
		owner: &OwnerRef{
			Type:  strPtr(testAcmeWidgetFQT),
			ID:    owner.ID,
			Field: strPtr("WorkloadID"),
		},
	}).Error)
	require.NoError(t, db.First(&stored, *owner.ID).Error)
	require.NotNil(t, stored.WorkloadID)
}

func relPtr(r Relationship) *Relationship { return &r }

// withThreeportOwner returns a session for the tptctl client certificate.
// That certificate has no organization.
func withThreeportOwner(db *gorm.DB) *gorm.DB {
	return db.WithContext(lib.WithCaller(context.Background(), lib.CallerIdentity{
		CommonName: "threeport-owner",
	}))
}

// TestRecordOwnerOnCreateMatchesCallerOrganization covers the certificate organization.
// A different organization is rejected and the same organization is allowed.
func TestRecordOwnerOnCreateMatchesCallerOrganization(t *testing.T) {
	db := setupOwnerTestDB(t)
	owner := &testAcmeWidget{}
	require.NoError(t, db.Create(owner).Error)
	ref := &OwnerRef{
		Type:  strPtr(testAcmeWidgetFQT),
		ID:    owner.ID,
		Field: strPtr("WorkloadID"),
	}

	// a caller from another namespace cannot name this owner
	other := db.WithContext(lib.WithCaller(context.Background(), lib.CallerIdentity{
		CommonName:   "other-threeport-module",
		Organization: "other.corp",
	}))
	err := other.Create(&testHolder{Name: strPtr("other"), owner: ref}).Error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "outside caller organization")
	var stored testAcmeWidget
	require.NoError(t, db.First(&stored, *owner.ID).Error)
	assert.Nil(t, stored.WorkloadID)

	// the module that owns the type can set the column
	same := db.WithContext(lib.WithCaller(context.Background(), lib.CallerIdentity{
		CommonName:   "acme-corp-threeport-module",
		Organization: "acme.corp",
	}))
	require.NoError(t, same.Create(&testHolder{Name: strPtr("same-org"), owner: ref}).Error)
	require.NoError(t, db.First(&stored, *owner.ID).Error)
	require.NotNil(t, stored.WorkloadID)
}

// TestRecordOwnerOnCreateAllowsThreeportOwner covers a caller with no organization.
// The threeport-owner certificate may set the column. Another common name may not.
func TestRecordOwnerOnCreateAllowsThreeportOwner(t *testing.T) {
	db := setupOwnerTestDB(t)
	owner := &testAcmeWidget{}
	require.NoError(t, db.Create(owner).Error)
	ref := &OwnerRef{
		Type:  strPtr(testAcmeWidgetFQT),
		ID:    owner.ID,
		Field: strPtr("WorkloadID"),
	}

	// another certificate with no organization cannot name the owner
	blank := db.WithContext(lib.WithCaller(context.Background(), lib.CallerIdentity{
		CommonName: "other-client",
	}))
	err := blank.Create(&testHolder{Name: strPtr("blank"), owner: ref}).Error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "caller organization")
	var stored testAcmeWidget
	require.NoError(t, db.First(&stored, *owner.ID).Error)
	assert.Nil(t, stored.WorkloadID)

	// the tptctl certificate sets the column
	require.NoError(t, withThreeportOwner(db).Create(&testHolder{Name: strPtr("cli"), owner: ref}).Error)
	require.NoError(t, db.First(&stored, *owner.ID).Error)
	require.NotNil(t, stored.WorkloadID)
}

// TestRecordOwnerOnCreateAllowsAuthOffCaller covers the identity CaptureCaller writes when authentication is off.
// That caller may set the column. A controller certificate with an organization may not cross namespaces.
func TestRecordOwnerOnCreateAllowsAuthOffCaller(t *testing.T) {
	db := setupOwnerTestDB(t)
	owner := &testAcmeWidget{}
	require.NoError(t, db.Create(owner).Error)
	ref := &OwnerRef{
		Type:  strPtr(testAcmeWidgetFQT),
		ID:    owner.ID,
		Field: strPtr("WorkloadID"),
	}

	// a controller certificate stays inside its organization
	controller := db.WithContext(lib.WithCaller(context.Background(), lib.CallerIdentity{
		CommonName:         "workload-controller",
		Organization:       "threeport.io",
		OrganizationalUnit: auth.OUControlPlane,
	}))
	err := controller.Create(&testHolder{Name: strPtr("controller"), owner: ref}).Error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "outside caller organization")
	var stored testAcmeWidget
	require.NoError(t, db.First(&stored, *owner.ID).Error)
	assert.Nil(t, stored.WorkloadID)

	// the authentication-off identity sets the column
	authOff := db.WithContext(lib.WithCaller(context.Background(), lib.CallerIdentity{
		OrganizationalUnit: auth.OUControlPlane,
	}))
	require.NoError(t, authOff.Create(&testHolder{Name: strPtr("auth-off"), owner: ref}).Error)
	require.NoError(t, db.First(&stored, *owner.ID).Error)
	require.NotNil(t, stored.WorkloadID)
}

func TestRecordOwnerOnCreateSkipsWhenUnset(t *testing.T) {
	db := setupOwnerTestDB(t)
	holder := &testHolder{Name: strPtr("plain")}
	require.NoError(t, db.Create(holder).Error)

	var n int64
	require.NoError(t, db.Model(&AttachedObjectReference{}).
		Where("object_type = ? AND object_id = ?", testHolderFQT, *holder.ID).
		Count(&n).Error)
	assert.Equal(t, int64(0), n)
}

func TestRecordOwnerOnCreateRollsBackWhenColumnHoldsAnotherID(t *testing.T) {
	db := setupOwnerTestDB(t)
	live := &testHolder{Name: strPtr("live")}
	require.NoError(t, db.Create(live).Error)
	owner := &testAcmeWidget{WorkloadID: live.ID}
	require.NoError(t, db.Create(owner).Error)

	holder := &testHolder{
		Name: strPtr("conflict"),
		owner: &OwnerRef{
			Type:  strPtr(testAcmeWidgetFQT),
			ID:    owner.ID,
			Field: strPtr("WorkloadID"),
		},
	}
	err := withThreeportOwner(db).Create(holder).Error
	require.Error(t, err)

	var n int64
	require.NoError(t, db.Model(&testHolder{}).Where("name = ?", "conflict").Count(&n).Error)
	assert.Equal(t, int64(0), n)

	var stored testAcmeWidget
	require.NoError(t, db.First(&stored, *owner.ID).Error)
	assert.Equal(t, *live.ID, *stored.WorkloadID)
}

// TestRecordOwnerOnCreateReplacesStaleChild covers a saved id whose child is gone or scheduled for deletion.
func TestRecordOwnerOnCreateReplacesStaleChild(t *testing.T) {
	db := setupOwnerTestDB(t)
	gone := &testHolder{Name: strPtr("gone")}
	require.NoError(t, db.Create(gone).Error)
	require.NoError(t, db.Delete(gone).Error)
	owner := &testAcmeWidget{WorkloadID: gone.ID}
	require.NoError(t, db.Create(owner).Error)

	// create a holder while the saved child is deleted
	holder := &testHolder{
		Name: strPtr("retry"),
		owner: &OwnerRef{
			Type:  strPtr(testAcmeWidgetFQT),
			ID:    owner.ID,
			Field: strPtr("WorkloadID"),
		},
	}
	require.NoError(t, withThreeportOwner(db).Create(holder).Error)
	var stored testAcmeWidget
	require.NoError(t, db.First(&stored, *owner.ID).Error)
	require.NotNil(t, stored.WorkloadID)
	assert.Equal(t, *holder.ID, *stored.WorkloadID)

	// a deletion-scheduled child lets the next create set the column
	scheduled := &testHolder{Name: strPtr("scheduled")}
	require.NoError(t, db.Create(scheduled).Error)
	require.NoError(t, db.Exec(
		"UPDATE test_holders SET deletion_scheduled = ? WHERE id = ?",
		time.Now(), *scheduled.ID,
	).Error)
	require.NoError(t, db.Exec(
		"UPDATE acme_corp_v0_widgets SET workload_id = ? WHERE id = ?",
		*scheduled.ID, *owner.ID,
	).Error)
	again := &testHolder{
		Name: strPtr("retry-scheduled"),
		owner: &OwnerRef{
			Type:  strPtr(testAcmeWidgetFQT),
			ID:    owner.ID,
			Field: strPtr("WorkloadID"),
		},
	}
	require.NoError(t, withThreeportOwner(db).Create(again).Error)
	require.NoError(t, db.First(&stored, *owner.ID).Error)
	assert.Equal(t, *again.ID, *stored.WorkloadID)
}

func TestRecordOwnerOnCreateAllowsColumnAlreadySetToThisID(t *testing.T) {
	db := setupOwnerTestDB(t)
	// the next testHolder id is 1 because this table is empty
	already := uint(1)
	owner := &testAcmeWidget{WorkloadID: &already}
	require.NoError(t, db.Create(owner).Error)

	holder := &testHolder{
		Name: strPtr("same"),
		owner: &OwnerRef{
			Type:  strPtr(testAcmeWidgetFQT),
			ID:    owner.ID,
			Field: strPtr("WorkloadID"),
		},
	}
	require.NoError(t, withThreeportOwner(db).Create(holder).Error)
	assert.Equal(t, already, *holder.ID)
}

func TestWorkloadOwnedByModuleTypeWritesTheRowsTogether(t *testing.T) {
	db := setupWorkloadOwnerDB(t)
	owner := &testAcmeWidget{}
	require.NoError(t, db.Create(owner).Error)
	before := rowCount(t, db)

	workload := &KubernetesWorkloadInstance{
		Common: Common{
			Owner: &OwnerRef{
				Type:  strPtr(testAcmeWidgetFQT),
				ID:    owner.ID,
				Field: strPtr("WorkloadID"),
			},
		},
		Instance:                       Instance{Name: strPtr("owned")},
		KubernetesRuntimeInstanceID:    uintPtr(1),
		KubernetesWorkloadDefinitionID: uintPtr(2),
	}
	require.NoError(t, withThreeportOwner(db).Create(workload).Error)

	// the workload, its two requires references, and the owns reference
	assert.Equal(t, before+4, rowCount(t, db))
	var stored testAcmeWidget
	require.NoError(t, db.First(&stored, *owner.ID).Error)
	require.NotNil(t, stored.WorkloadID)
	assert.Equal(t, *workload.ID, *stored.WorkloadID)

	var owns int64
	require.NoError(t, db.Model(&AttachedObjectReference{}).Where(
		"object_type = ? AND object_id = ? AND relationship = ? AND attached_object_type = ?",
		workload.GetFullyQualifiedType(), *workload.ID, RelationshipOwns, testAcmeWidgetFQT,
	).Count(&owns).Error)
	assert.Equal(t, int64(1), owns)
}

func TestWorkloadOwnedByRuntimeWritesTheRowsTogether(t *testing.T) {
	db := setupWorkloadOwnerDB(t)
	runtime := &KubernetesRuntimeInstance{
		Instance:                      Instance{Name: strPtr("runtime")},
		Location:                      strPtr("local"),
		KubernetesRuntimeDefinitionID: uintPtr(1),
	}
	require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).Create(runtime).Error)
	before := rowCount(t, db)

	workload := &KubernetesWorkloadInstance{
		Common: Common{
			Owner: &OwnerRef{
				Type:  strPtr(runtime.GetFullyQualifiedType()),
				ID:    runtime.ID,
				Field: strPtr("GatewayControllerInstanceID"),
			},
		},
		Instance:                       Instance{Name: strPtr("owned")},
		KubernetesRuntimeInstanceID:    runtime.ID,
		KubernetesWorkloadDefinitionID: uintPtr(2),
	}
	require.NoError(t, withThreeportOwner(db).Create(workload).Error)

	assert.Equal(t, before+4, rowCount(t, db))
	var stored KubernetesRuntimeInstance
	require.NoError(t, db.First(&stored, *runtime.ID).Error)
	require.NotNil(t, stored.GatewayControllerInstanceID)
	assert.Equal(t, *workload.ID, *stored.GatewayControllerInstanceID)
}

func TestWorkloadOwnerConflictRollsBackTheRows(t *testing.T) {
	db := setupWorkloadOwnerDB(t)
	live := &KubernetesWorkloadInstance{
		Instance:                       Instance{Name: strPtr("live")},
		KubernetesRuntimeInstanceID:    uintPtr(1),
		KubernetesWorkloadDefinitionID: uintPtr(2),
	}
	require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).Create(live).Error)
	owner := &testAcmeWidget{WorkloadID: live.ID}
	require.NoError(t, db.Create(owner).Error)
	before := rowCount(t, db)

	workload := &KubernetesWorkloadInstance{
		Common: Common{
			Owner: &OwnerRef{
				Type:  strPtr(testAcmeWidgetFQT),
				ID:    owner.ID,
				Field: strPtr("WorkloadID"),
			},
		},
		Instance:                       Instance{Name: strPtr("conflict")},
		KubernetesRuntimeInstanceID:    uintPtr(1),
		KubernetesWorkloadDefinitionID: uintPtr(2),
	}
	require.Error(t, withThreeportOwner(db).Create(workload).Error)
	assert.Equal(t, before, rowCount(t, db))
}

func setupWorkloadOwnerDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&KubernetesWorkloadInstance{},
		&KubernetesRuntimeInstance{},
		&AttachedObjectReference{},
		&testAcmeWidget{},
	))
	return db
}

func rowCount(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var workloads int64
	var runtimes int64
	var refs int64
	var owners int64
	require.NoError(t, db.Model(&KubernetesWorkloadInstance{}).Count(&workloads).Error)
	require.NoError(t, db.Model(&KubernetesRuntimeInstance{}).Count(&runtimes).Error)
	require.NoError(t, db.Model(&AttachedObjectReference{}).Count(&refs).Error)
	require.NoError(t, db.Model(&testAcmeWidget{}).Count(&owners).Error)
	return workloads + runtimes + refs + owners
}

func TestRecordOwnerOnCreateRejectsMissingColumn(t *testing.T) {
	db := setupOwnerTestDB(t)
	owner := &testAcmeWidget{}
	require.NoError(t, db.Create(owner).Error)
	holder := &testHolder{
		Name: strPtr("missing"),
		owner: &OwnerRef{
			Type:  strPtr(testAcmeWidgetFQT),
			ID:    owner.ID,
			Field: strPtr("NotAColumn"),
		},
	}
	err := withThreeportOwner(db).Create(holder).Error
	require.Error(t, err)
	var httpErr *util.HttpError
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, 400, httpErr.StatusCode)
}
