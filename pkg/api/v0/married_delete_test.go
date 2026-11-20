package v0

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	util "github.com/threeport/threeport/pkg/util/v0"
)

// marriedDeleteDB returns a database with a GKE runtime definition married
// to its kubernetes runtime definition, and a GKE runtime instance married
// to its kubernetes runtime instance, created through their hooks so their
// attached object references are written.
func marriedDeleteDB(t *testing.T) (
	db *gorm.DB,
	gkeDefinition *GcpGkeKubernetesRuntimeDefinition,
	gkeInstance *GcpGkeKubernetesRuntimeInstance,
) {
	t.Helper()

	db = setupKubernetesApiAccessDB(t)
	require.NoError(t, db.AutoMigrate(&GcpGkeKubernetesRuntimeInstance{}))

	// a definition first, so no married object shares its base's ID, as on
	// CockroachDB, where IDs are unique across tables
	require.NoError(t, db.Create(&KubernetesRuntimeDefinition{
		Definition:       Definition{Name: util.Ptr("filler")},
		InfraProvider:    util.Ptr(KubernetesRuntimeInfraProviderKind),
		HighAvailability: util.Ptr(false),
	}).Error)

	runtimeDefinition := &KubernetesRuntimeDefinition{
		Definition:       Definition{Name: util.Ptr("dev-01")},
		InfraProvider:    util.Ptr(KubernetesRuntimeInfraProviderGKE),
		HighAvailability: util.Ptr(false),
	}
	require.NoError(t, db.Create(runtimeDefinition).Error)
	gkeDefinition = &GcpGkeKubernetesRuntimeDefinition{
		Definition:                    Definition{Name: util.Ptr("dev-01")},
		ZoneCount:                     util.Ptr(1),
		DefaultNodeGroupInstanceType:  util.Ptr("e2-standard-4"),
		DefaultNodeGroupInitialSize:   util.Ptr(2),
		DefaultNodeGroupMinimumSize:   util.Ptr(0),
		DefaultNodeGroupMaximumSize:   util.Ptr(5),
		KubernetesRuntimeDefinitionID: runtimeDefinition.ID,
	}
	require.NoError(t, db.Create(gkeDefinition).Error)

	runtimeInstance := &KubernetesRuntimeInstance{
		Instance:                      Instance{Name: util.Ptr("dev-01")},
		Location:                      util.Ptr("NorthAmerica:NewYork"),
		KubernetesRuntimeDefinitionID: runtimeDefinition.ID,
	}
	require.NoError(t, db.Create(runtimeInstance).Error)
	gkeInstance = &GcpGkeKubernetesRuntimeInstance{
		Instance:                            Instance{Name: util.Ptr("dev-01")},
		GcpProviderID:                       util.Ptr(uint(1)),
		GcpGkeKubernetesRuntimeDefinitionID: gkeDefinition.ID,
		KubernetesRuntimeInstanceID:         runtimeInstance.ID,
	}
	require.NoError(t, db.Create(gkeInstance).Error)

	return db, gkeDefinition, gkeInstance
}

// exists reports whether the row of model's type with id is in db.
func exists(t *testing.T, db *gorm.DB, model any, id uint) bool {
	t.Helper()

	var count int64
	require.NoError(t, db.Model(model).Where("id = ?", id).Count(&count).Error)
	return count == 1
}

// marriesReferenceExists reports whether the marries reference from the
// object of attachedType with attachedID is in db.
func marriesReferenceExists(t *testing.T, db *gorm.DB, attachedType string, attachedID uint) bool {
	t.Helper()

	var count int64
	require.NoError(t, db.Model(&AttachedObjectReference{}).
		Where("attached_object_type = ? AND attached_object_id = ? AND relationship = ?", attachedType, attachedID, RelationshipMarries).
		Count(&count).Error)
	return count == 1
}

// TestADeleteRemovesTheBaseItMarriesInTheSameTransaction covers #768: a
// delete of an object with a marries foreign key deletes the base it names
// too, in one transaction.  A base that something else still depends on, or
// that refuses its delete, keeps the married object as well, so the same
// delete can be retried.
func TestADeleteRemovesTheBaseItMarriesInTheSameTransaction(t *testing.T) {
	t.Run("a definition takes its runtime definition with it", func(t *testing.T) {
		db, gkeDefinition, gkeInstance := marriedDeleteDB(t)
		require.NoError(t, db.Delete(gkeInstance).Error, "the GKE instance, which requires the definition, goes first")
		runtimeInstance := &KubernetesRuntimeInstance{}
		require.NoError(t, db.First(runtimeInstance, *gkeInstance.KubernetesRuntimeInstanceID).Error)
		require.NoError(t, db.Delete(runtimeInstance).Error)

		require.NoError(t, DeleteWithMarriedBases(db, gkeDefinition).Error)
		assert.False(t, exists(t, db, &GcpGkeKubernetesRuntimeDefinition{}, *gkeDefinition.ID))
		assert.False(t, exists(t, db, &KubernetesRuntimeDefinition{}, *gkeDefinition.KubernetesRuntimeDefinitionID))
		assert.False(t, marriesReferenceExists(t, db, gkeDefinition.GetFullyQualifiedType(), *gkeDefinition.ID))
	})

	t.Run("an instance takes its runtime instance with it", func(t *testing.T) {
		db, _, gkeInstance := marriedDeleteDB(t)

		require.NoError(t, DeleteWithMarriedBases(withControlPlaneCaller(db), gkeInstance).Error)
		assert.False(t, exists(t, db, &GcpGkeKubernetesRuntimeInstance{}, *gkeInstance.ID))
		assert.False(t, exists(t, db, &KubernetesRuntimeInstance{}, *gkeInstance.KubernetesRuntimeInstanceID))
	})

	t.Run("a request narrowed to the married object's ID", func(t *testing.T) {
		db, gkeDefinition, gkeInstance := marriedDeleteDB(t)
		require.NoError(t, db.Delete(gkeInstance).Error)
		runtimeInstance := &KubernetesRuntimeInstance{}
		require.NoError(t, db.First(runtimeInstance, *gkeInstance.KubernetesRuntimeInstanceID).Error)
		require.NoError(t, db.Delete(runtimeInstance).Error)

		require.NotEqual(t, *gkeDefinition.ID, *gkeDefinition.KubernetesRuntimeDefinitionID)

		// as ?ids= does, the request's scope narrows every read to the IDs it names
		narrowed := db.Scopes(func(scoped *gorm.DB) *gorm.DB {
			return scoped.Where("id IN ?", []uint{*gkeDefinition.ID})
		})
		require.NoError(t, DeleteWithMarriedBases(narrowed, gkeDefinition).Error)
		assert.False(t, exists(t, db, &GcpGkeKubernetesRuntimeDefinition{}, *gkeDefinition.ID))
		assert.False(t, exists(t, db, &KubernetesRuntimeDefinition{}, *gkeDefinition.KubernetesRuntimeDefinitionID),
			"the base is found and deleted although the request names only the married object")
	})

	t.Run("a request narrowed to the married object's ID, with a base something depends on", func(t *testing.T) {
		db, gkeDefinition, gkeInstance := marriedDeleteDB(t)
		require.NoError(t, db.Delete(gkeInstance).Error)

		narrowed := db.Scopes(func(scoped *gorm.DB) *gorm.DB {
			return scoped.Where("id IN ?", []uint{*gkeDefinition.ID})
		})
		err := DeleteWithMarriedBases(narrowed, gkeDefinition).Error
		require.ErrorAs(t, err, new(*BlockedDeleteError), "what depends on the base is found too")
		assert.True(t, exists(t, db, &GcpGkeKubernetesRuntimeDefinition{}, *gkeDefinition.ID))
	})

	t.Run("a base already gone", func(t *testing.T) {
		db, _, gkeInstance := marriedDeleteDB(t)
		require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).
			Where("id = ?", *gkeInstance.KubernetesRuntimeInstanceID).Delete(&KubernetesRuntimeInstance{}).Error)

		require.NoError(t, DeleteWithMarriedBases(withControlPlaneCaller(db), gkeInstance).Error)
		assert.False(t, exists(t, db, &GcpGkeKubernetesRuntimeInstance{}, *gkeInstance.ID))
	})

	t.Run("a base whose own delete is underway", func(t *testing.T) {
		db, _, gkeInstance := marriedDeleteDB(t)
		require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).Model(&KubernetesRuntimeInstance{}).
			Where("id = ?", *gkeInstance.KubernetesRuntimeInstanceID).
			Update("deletion_scheduled", time.Now().UTC()).Error)

		require.NoError(t, DeleteWithMarriedBases(withControlPlaneCaller(db), gkeInstance).Error)
		assert.False(t, exists(t, db, &GcpGkeKubernetesRuntimeInstance{}, *gkeInstance.ID))
		assert.True(t, exists(t, db, &KubernetesRuntimeInstance{}, *gkeInstance.KubernetesRuntimeInstanceID),
			"its reconciler finishes the delete it started")
	})

	t.Run("a base something else depends on", func(t *testing.T) {
		db, gkeDefinition, gkeInstance := marriedDeleteDB(t)
		require.NoError(t, db.Delete(gkeInstance).Error)

		// the kubernetes runtime instance still requires the runtime definition
		err := DeleteWithMarriedBases(db, gkeDefinition).Error
		var blocked *BlockedDeleteError
		require.ErrorAs(t, err, &blocked)
		assert.Equal(t, new(KubernetesRuntimeDefinition).GetFullyQualifiedType(), *blocked.AttachedRefs[0].ObjectType)
		assert.True(t, exists(t, db, &GcpGkeKubernetesRuntimeDefinition{}, *gkeDefinition.ID), "the married definition is kept")
		assert.True(t, marriesReferenceExists(t, db, gkeDefinition.GetFullyQualifiedType(), *gkeDefinition.ID),
			"with the reference that names its runtime definition")
		assert.True(t, exists(t, db, &KubernetesRuntimeDefinition{}, *gkeDefinition.KubernetesRuntimeDefinitionID))
	})

	t.Run("a base that refuses its delete", func(t *testing.T) {
		db, gkeDefinition, gkeInstance := marriedDeleteDB(t)
		require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).Model(gkeDefinition).
			Update("deletion_protection", true).Error)

		err := DeleteWithMarriedBases(withControlPlaneCaller(db), gkeInstance).Error
		var httpErr *util.HttpError
		require.ErrorAs(t, err, &httpErr)
		assert.Equal(t, http.StatusConflict, httpErr.GetStatusCode())
		assert.Contains(t, err.Error(), ErrMsgDeleteBlocked)
		assert.True(t, exists(t, db, &GcpGkeKubernetesRuntimeInstance{}, *gkeInstance.ID), "the married instance is kept")
		assert.True(t, exists(t, db, &KubernetesRuntimeInstance{}, *gkeInstance.KubernetesRuntimeInstanceID))
	})

	t.Run("a married object that cannot be deleted keeps its base", func(t *testing.T) {
		db, gkeDefinition, _ := marriedDeleteDB(t)

		// the GKE instance still requires the GKE definition
		err := DeleteWithMarriedBases(db, gkeDefinition).Error
		require.True(t, errors.As(err, new(*BlockedDeleteError)))
		assert.True(t, exists(t, db, &GcpGkeKubernetesRuntimeDefinition{}, *gkeDefinition.ID))
		assert.True(t, exists(t, db, &KubernetesRuntimeDefinition{}, *gkeDefinition.KubernetesRuntimeDefinitionID))
	})

	t.Run("an object that marries nothing is deleted alone", func(t *testing.T) {
		db, gkeDefinition, gkeInstance := marriedDeleteDB(t)
		require.NoError(t, db.Delete(gkeInstance).Error)
		require.NoError(t, db.Delete(gkeDefinition).Error)
		runtimeInstanceID := *gkeInstance.KubernetesRuntimeInstanceID

		runtimeInstance := &KubernetesRuntimeInstance{}
		require.NoError(t, db.First(runtimeInstance, runtimeInstanceID).Error)
		require.NoError(t, DeleteWithMarriedBases(withControlPlaneCaller(db), runtimeInstance).Error)
		assert.False(t, exists(t, db, &KubernetesRuntimeInstance{}, runtimeInstanceID))
		assert.True(t, exists(t, db, &KubernetesRuntimeDefinition{}, *gkeDefinition.KubernetesRuntimeDefinitionID))
	})
}

// TestEachMarriedObjectNamesItsBase covers the MarriedBases the SDK generates
// for the six provider types that marry a kubernetes runtime definition or
// instance: each returns that base with the stored ID, and nothing when the
// foreign key is not set.
func TestEachMarriedObjectNamesItsBase(t *testing.T) {
	id := util.Ptr(uint(7))
	for name, c := range map[string]struct {
		set   MarriedBasesProvider
		unset MarriedBasesProvider
		base  any
	}{
		"EKS definition": {&AwsEksKubernetesRuntimeDefinition{KubernetesRuntimeDefinitionID: id}, &AwsEksKubernetesRuntimeDefinition{}, &KubernetesRuntimeDefinition{}},
		"GKE definition": {&GcpGkeKubernetesRuntimeDefinition{KubernetesRuntimeDefinitionID: id}, &GcpGkeKubernetesRuntimeDefinition{}, &KubernetesRuntimeDefinition{}},
		"OKE definition": {&OciOkeKubernetesRuntimeDefinition{KubernetesRuntimeDefinitionID: id}, &OciOkeKubernetesRuntimeDefinition{}, &KubernetesRuntimeDefinition{}},
		"EKS instance":   {&AwsEksKubernetesRuntimeInstance{KubernetesRuntimeInstanceID: id}, &AwsEksKubernetesRuntimeInstance{}, &KubernetesRuntimeInstance{}},
		"GKE instance":   {&GcpGkeKubernetesRuntimeInstance{KubernetesRuntimeInstanceID: id}, &GcpGkeKubernetesRuntimeInstance{}, &KubernetesRuntimeInstance{}},
		"OKE instance":   {&OciOkeKubernetesRuntimeInstance{KubernetesRuntimeInstanceID: id}, &OciOkeKubernetesRuntimeInstance{}, &KubernetesRuntimeInstance{}},
	} {
		t.Run(name, func(t *testing.T) {
			bases := c.set.MarriedBases()
			require.Len(t, bases, 1)
			assert.IsType(t, c.base, bases[0])
			assert.Equal(t, id, util.ObjectID(bases[0]))
			assert.Empty(t, c.unset.MarriedBases())
		})
	}
}
