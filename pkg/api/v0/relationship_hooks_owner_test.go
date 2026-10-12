package v0

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
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
	require.NoError(t, db.Create(holder).Error)

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
	other := uint(99)
	owner := &testAcmeWidget{WorkloadID: &other}
	require.NoError(t, db.Create(owner).Error)

	holder := &testHolder{
		Name: strPtr("conflict"),
		owner: &OwnerRef{
			Type:  strPtr(testAcmeWidgetFQT),
			ID:    owner.ID,
			Field: strPtr("WorkloadID"),
		},
	}
	err := db.Create(holder).Error
	require.Error(t, err)

	var n int64
	require.NoError(t, db.Model(&testHolder{}).Where("name = ?", "conflict").Count(&n).Error)
	assert.Equal(t, int64(0), n)

	var stored testAcmeWidget
	require.NoError(t, db.First(&stored, *owner.ID).Error)
	assert.Equal(t, other, *stored.WorkloadID)
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
	require.NoError(t, db.Create(holder).Error)
	assert.Equal(t, already, *holder.ID)
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
	require.Error(t, db.Create(holder).Error)
}
