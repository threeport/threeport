package cockroach

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/threeport/threeport/pkg/api-server/v0/handlers"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// referenceRuntime writes the attached object references the hooks write for
// an EKS runtime createEksRuntime wrote without them: each EKS object marries
// its kubernetes runtime object, and requires what its foreign keys name.
func referenceRuntime(t *testing.T, db *gorm.DB, runtime *protectedEksRuntime) {
	t.Helper()

	reference := func(attached any, attachedID *uint, base any, baseID *uint, relationship api_v0.Relationship) {
		attachedType := attached.(interface{ GetFullyQualifiedType() string }).GetFullyQualifiedType()
		baseType := base.(interface{ GetFullyQualifiedType() string }).GetFullyQualifiedType()
		require.NoError(t, db.Create(&api_v0.AttachedObjectReference{
			ObjectType:         &baseType,
			ObjectID:           baseID,
			AttachedObjectType: &attachedType,
			AttachedObjectID:   attachedID,
			Relationship:       &relationship,
		}).Error)
	}
	reference(&runtime.eksDefinition, runtime.eksDefinition.ID, &runtime.definition, runtime.definition.ID, api_v0.RelationshipMarries)
	reference(&runtime.instance, runtime.instance.ID, &runtime.definition, runtime.definition.ID, api_v0.RelationshipRequires)
	reference(&runtime.eksInstance, runtime.eksInstance.ID, &runtime.instance, runtime.instance.ID, api_v0.RelationshipMarries)
	reference(&runtime.eksInstance, runtime.eksInstance.ID, &runtime.eksDefinition, runtime.eksDefinition.ID, api_v0.RelationshipRequires)
}

// confirmDeletion marks an EKS instance's delete confirmed, as its
// controller does once the cluster is gone, so its next delete removes it.
func confirmDeletion(t *testing.T, db *gorm.DB, eksInstance *api_v0.AwsEksKubernetesRuntimeInstance) {
	t.Helper()

	now := time.Now().UTC()
	require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).Model(eksInstance).Updates(map[string]any{
		"deletion_scheduled": now,
		"deletion_confirmed": now,
	}).Error)
}

// live reports whether the row of model's type with id is in db and not
// soft deleted.
func live(t *testing.T, db *gorm.DB, model any, id uint) bool {
	t.Helper()

	var count int64
	require.NoError(t, db.Model(model).Where("id = ?", id).Count(&count).Error)
	return count == 1
}

// TestADeleteRemovesTheBaseItMarriesOnCockroach covers #768 through the
// generated delete handlers against a real CockroachDB.  Removing an EKS
// instance whose delete is confirmed removes its kubernetes runtime instance
// in the same transaction, and deleting the EKS definition then removes its
// kubernetes runtime definition: each with one request.  When the base
// cannot go, the transaction is rolled back and the married object stays,
// with the reference that names its base, so the same delete can be retried.
func TestADeleteRemovesTheBaseItMarriesOnCockroach(t *testing.T) {
	db := runtimeDatabase(t, "married_delete")
	handler := handlers.Handler{DB: db, Logger: zap.NewNop()}
	deleteRequest := func(t *testing.T, route string, id uint, handle func(echo.Context) error) int {
		t.Helper()
		c, recorder := newAPIRequest(http.MethodDelete, route+"/:id", fmt.Sprint(id), "")
		require.NoError(t, handle(c))
		return recorder.Code
	}

	t.Run("one request each", func(t *testing.T) {
		runtime := createEksRuntime(t, db, "married-delete", nil)
		referenceRuntime(t, db, runtime)
		confirmDeletion(t, db, &runtime.eksInstance)

		code := deleteRequest(t, api_v0.PathAwsEksKubernetesRuntimeInstances, *runtime.eksInstance.ID, handler.DeleteAwsEksKubernetesRuntimeInstance)
		require.Equal(t, http.StatusOK, code)
		assert.False(t, live(t, db, &api_v0.AwsEksKubernetesRuntimeInstance{}, *runtime.eksInstance.ID))
		assert.False(t, live(t, db, &api_v0.KubernetesRuntimeInstance{}, *runtime.instance.ID),
			"the kubernetes runtime instance goes with the EKS instance")

		code = deleteRequest(t, api_v0.PathAwsEksKubernetesRuntimeDefinitions, *runtime.eksDefinition.ID, handler.DeleteAwsEksKubernetesRuntimeDefinition)
		require.Equal(t, http.StatusOK, code)
		assert.False(t, live(t, db, &api_v0.AwsEksKubernetesRuntimeDefinition{}, *runtime.eksDefinition.ID))
		assert.False(t, live(t, db, &api_v0.KubernetesRuntimeDefinition{}, *runtime.definition.ID),
			"the kubernetes runtime definition goes with the EKS definition")

		var references int64
		require.NoError(t, db.Model(&api_v0.AttachedObjectReference{}).
			Where("object_id IN ? OR attached_object_id IN ?",
				[]uint{*runtime.definition.ID, *runtime.instance.ID},
				[]uint{*runtime.eksDefinition.ID, *runtime.eksInstance.ID}).
			Count(&references).Error)
		assert.Zero(t, references, "no reference outlives the rows it named")
	})

	t.Run("a base something else depends on", func(t *testing.T) {
		runtime := createEksRuntime(t, db, "married-delete-blocked", nil)
		referenceRuntime(t, db, runtime)
		confirmDeletion(t, db, &runtime.eksInstance)

		// something else requires the kubernetes runtime instance
		baseType := runtime.instance.GetFullyQualifiedType()
		otherType := "threeport.io/v0.KubernetesWorkloadInstance"
		requires := api_v0.RelationshipRequires
		require.NoError(t, db.Create(&api_v0.AttachedObjectReference{
			ObjectType:         &baseType,
			ObjectID:           runtime.instance.ID,
			AttachedObjectType: &otherType,
			AttachedObjectID:   util.Ptr(uint(424242)),
			Relationship:       &requires,
		}).Error)

		code := deleteRequest(t, api_v0.PathAwsEksKubernetesRuntimeInstances, *runtime.eksInstance.ID, handler.DeleteAwsEksKubernetesRuntimeInstance)
		assert.Equal(t, http.StatusConflict, code)
		assert.True(t, live(t, db, &api_v0.AwsEksKubernetesRuntimeInstance{}, *runtime.eksInstance.ID), "the EKS instance is kept")
		assert.True(t, live(t, db, &api_v0.KubernetesRuntimeInstance{}, *runtime.instance.ID))

		eksInstanceType := runtime.eksInstance.GetFullyQualifiedType()
		var marries int64
		require.NoError(t, db.Model(&api_v0.AttachedObjectReference{}).
			Where("attached_object_type = ? AND attached_object_id = ? AND relationship = ?",
				eksInstanceType, *runtime.eksInstance.ID, api_v0.RelationshipMarries).
			Count(&marries).Error)
		assert.Equal(t, int64(1), marries, "with the reference that names its kubernetes runtime instance")
	})
}
