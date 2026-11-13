package cockroach

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/threeport/threeport/cmd/database-migrator/migrations"
	"github.com/threeport/threeport/pkg/api-server/v0/handlers"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestDeletionProtectionMigratesAnExistingDatabase covers 000009 on a
// database built before the definitions recorded deletion protection, as a
// deployed control plane's is: it adds the column to each kubernetes runtime
// definition table, and a definition already there reads as unprotected.  On
// a database 000001 built with the field already, it changes nothing, and its
// down migration removes it.
func TestDeletionProtectionMigratesAnExistingDatabase(t *testing.T) {
	db := freshDatabase(t, "deletion_protection_migration")
	//nolint:staticcheck // SA1029: the key has to match what the migration reads
	ctx := context.WithValue(context.Background(), "gormdb", db)
	sqlDb, err := db.DB()
	require.NoError(t, err)
	definitions := []any{
		&api_v0.KubernetesRuntimeDefinition{},
		&api_v0.AwsEksKubernetesRuntimeDefinition{},
		&api_v0.GcpGkeKubernetesRuntimeDefinition{},
		&api_v0.OciOkeKubernetesRuntimeDefinition{},
	}
	withColumn := func() int {
		count := 0
		for _, definition := range definitions {
			if db.Migrator().HasColumn(definition, "deletion_protection") {
				count++
			}
		}
		return count
	}

	// a new install: 000001 builds the field from the models
	require.NoError(t, migrations.Up000001(ctx, nil))
	require.Equal(t, len(definitions), withColumn())
	require.NoError(t, migrations.Up000009(ctx, sqlDb), "000009 finds it there")

	// a database built before the field, holding a definition
	require.NoError(t, migrations.Down000009(ctx, sqlDb))
	assert.Zero(t, withColumn())
	require.NoError(t, migrations.Down000009(ctx, sqlDb), "a down migration with nothing left to remove")
	require.NoError(t, db.Exec(
		"INSERT INTO v0_kubernetes_runtime_definitions (name, infra_provider, created_at, updated_at) VALUES ('before', 'eks', now(), now())",
	).Error)

	require.NoError(t, migrations.Up000009(ctx, sqlDb))
	assert.Equal(t, len(definitions), withColumn(), "on every kubernetes runtime definition table")
	var protected *bool
	require.NoError(t, db.Raw(
		"SELECT deletion_protection FROM v0_kubernetes_runtime_definitions WHERE name = 'before'",
	).Scan(&protected).Error)
	require.NotNil(t, protected)
	assert.False(t, *protected, "a definition from before isn't protected")
}

// protectedEksRuntime is an EKS runtime's rows: its kubernetes runtime
// definition and instance, and its EKS definition and instance.
type protectedEksRuntime struct {
	definition    api_v0.KubernetesRuntimeDefinition
	instance      api_v0.KubernetesRuntimeInstance
	eksDefinition api_v0.AwsEksKubernetesRuntimeDefinition
	eksInstance   api_v0.AwsEksKubernetesRuntimeInstance
}

// runtimeDatabase returns a database with every table the API has, which
// the package's shared one doesn't.
func runtimeDatabase(t *testing.T, name string) *gorm.DB {
	t.Helper()
	db := freshDatabase(t, name)
	//nolint:staticcheck // SA1029: the key has to match what the migration reads
	require.NoError(t, migrations.Up000001(context.WithValue(context.Background(), "gormdb", db), nil))

	return db
}

// createEksRuntime writes an EKS runtime whose EKS definition has protection
// as given, under names no other test uses.
func createEksRuntime(t *testing.T, database *gorm.DB, name string, protection *bool) *protectedEksRuntime {
	t.Helper()
	db := database.Session(&gorm.Session{SkipHooks: true})

	awsProvider := api_v0.AwsProvider{
		Name:          util.Ptr(name),
		AccountID:     util.Ptr("123456789012"),
		DefaultRegion: util.Ptr("us-east-2"),
	}
	require.NoError(t, db.Create(&awsProvider).Error)

	runtime := &protectedEksRuntime{}
	runtime.definition = api_v0.KubernetesRuntimeDefinition{
		Definition:    api_v0.Definition{Name: util.Ptr(name)},
		InfraProvider: util.Ptr(api_v0.KubernetesRuntimeInfraProviderEKS),
	}
	require.NoError(t, db.Create(&runtime.definition).Error)
	runtime.eksDefinition = api_v0.AwsEksKubernetesRuntimeDefinition{
		Definition:                    api_v0.Definition{Name: util.Ptr(name)},
		ZoneCount:                     util.Ptr(2),
		DefaultNodeGroupInstanceType:  util.Ptr("t3.medium"),
		DefaultNodeGroupInitialSize:   util.Ptr(1),
		DefaultNodeGroupMinimumSize:   util.Ptr(1),
		DefaultNodeGroupMaximumSize:   util.Ptr(3),
		DeletionProtection:            protection,
		KubernetesRuntimeDefinitionID: runtime.definition.ID,
	}
	require.NoError(t, db.Create(&runtime.eksDefinition).Error)
	runtime.instance = api_v0.KubernetesRuntimeInstance{
		Instance:                      api_v0.Instance{Name: util.Ptr(name)},
		Location:                      util.Ptr("NorthAmerica:NewYork"),
		KubernetesRuntimeDefinitionID: runtime.definition.ID,
	}
	require.NoError(t, db.Create(&runtime.instance).Error)
	runtime.eksInstance = api_v0.AwsEksKubernetesRuntimeInstance{
		Instance:                            api_v0.Instance{Name: util.Ptr(name)},
		Region:                              util.Ptr("us-east-2"),
		AwsProviderID:                       awsProvider.ID,
		AwsEksKubernetesRuntimeDefinitionID: runtime.eksDefinition.ID,
		KubernetesRuntimeInstanceID:         runtime.instance.ID,
	}
	require.NoError(t, db.Create(&runtime.eksInstance).Error)

	return runtime
}

// TestTheApiRefusesToDeleteAProtectedCluster covers a delete of a runtime
// instance, or of its EKS instance, while the EKS definition has
// DeletionProtection set: the API answers 409, naming the definition and how
// to turn the protection off, and schedules nothing.
func TestTheApiRefusesToDeleteAProtectedCluster(t *testing.T) {
	db := runtimeDatabase(t, "deletion_protected")
	runtime := createEksRuntime(t, db, "deletion-protected", util.Ptr(true))
	handler := handlers.Handler{DB: db, Logger: zap.NewNop()}

	for _, request := range []struct {
		route  string
		id     uint
		model  any
		delete func(echo.Context) error
	}{
		{api_v0.PathKubernetesRuntimeInstances, *runtime.instance.ID, &api_v0.KubernetesRuntimeInstance{}, handler.DeleteKubernetesRuntimeInstance},
		{api_v0.PathAwsEksKubernetesRuntimeInstances, *runtime.eksInstance.ID, &api_v0.AwsEksKubernetesRuntimeInstance{}, handler.DeleteAwsEksKubernetesRuntimeInstance},
	} {
		c, recorder := newAPIRequest(http.MethodDelete, request.route+"/:id", fmt.Sprint(request.id), "")
		require.NoError(t, request.delete(c))
		assert.Equal(t, http.StatusConflict, recorder.Code, request.route)

		var response struct {
			Status struct{ Error string }
		}
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
		assert.Contains(t, response.Status.Error, "aws-eks-kubernetes-runtime-definition deletion-protected has DeletionProtection set")
		assert.Contains(t, response.Status.Error, "tptctl replace aws-eks-kubernetes-runtime-definition --config <file> --name deletion-protected")

		scheduled := map[string]any{}
		require.NoError(t, db.Model(request.model).Where("id = ?", request.id).Select("deletion_scheduled").Take(&scheduled).Error)
		assert.Nil(t, scheduled["deletion_scheduled"], "the delete isn't scheduled")
	}
}

// TestAnUnprotectedClusterCanBeDeleted covers the check the API makes on a
// delete when nothing protects the cluster: an EKS definition with
// DeletionProtection false or unset, and a kind runtime, which has no
// provider definition.
func TestAnUnprotectedClusterCanBeDeleted(t *testing.T) {
	db := runtimeDatabase(t, "deletion_unprotected")
	for name, protection := range map[string]*bool{
		"deletion-unprotected": util.Ptr(false),
		"deletion-unset":       nil,
	} {
		runtime := createEksRuntime(t, db, name, protection)
		assert.NoError(t, api_v0.CheckDeleteRefused(db, &runtime.instance), name)
		assert.NoError(t, api_v0.CheckDeleteRefused(db, &runtime.eksInstance), name)
	}

	kind := api_v0.KubernetesRuntimeDefinition{
		Definition:    api_v0.Definition{Name: util.Ptr("deletion-kind")},
		InfraProvider: util.Ptr(api_v0.KubernetesRuntimeInfraProviderKind),
	}
	require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).Create(&kind).Error)
	assert.NoError(t, api_v0.CheckDeleteRefused(db, &api_v0.KubernetesRuntimeInstance{KubernetesRuntimeDefinitionID: kind.ID}))
}

// TestProtectionCantBeTurnedOnForExistingClusters covers an update of an
// EKS definition's DeletionProtection, by replace and by patch.  Turning it
// on while the definition has runtime instances is refused, since their
// clusters were created unprotected and the definition would report them
// protected.  Turning it off is allowed, and so is turning it on for a
// definition with no instances yet, or leaving it on.
func TestProtectionCantBeTurnedOnForExistingClusters(t *testing.T) {
	db := runtimeDatabase(t, "deletion_protection_update")
	handler := handlers.Handler{DB: db, Logger: zap.NewNop()}
	registerValidateTags(string(api_v0.ObjectTypeAwsEksKubernetesRuntimeDefinition), new(api_v0.AwsEksKubernetesRuntimeDefinition))
	unprotected := createEksRuntime(t, db, "update-unprotected", util.Ptr(false))
	protected := createEksRuntime(t, db, "update-protected", util.Ptr(true))
	empty := createEksRuntime(t, db, "update-empty", nil)
	require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).Delete(&empty.eksInstance).Error)

	update := func(method string, definition api_v0.AwsEksKubernetesRuntimeDefinition, protection bool) (int, string) {
		t.Helper()
		var body []byte
		var err error
		if method == http.MethodPatch {
			body, err = json.Marshal(map[string]any{"DeletionProtection": protection})
		} else {
			definition.DeletionProtection = util.Ptr(protection)
			body, err = json.Marshal(definition)
			require.NoError(t, err)
			// CockroachDB's IDs don't fit a float64, so numbers are kept as written
			fields := map[string]any{}
			decoder := json.NewDecoder(bytes.NewReader(body))
			decoder.UseNumber()
			require.NoError(t, decoder.Decode(&fields))
			// a replace carries the definition's own fields, as tptctl's does
			for _, model := range []string{"ID", "CreatedAt", "UpdatedAt", "DeletedAt"} {
				delete(fields, model)
			}
			for field, value := range fields {
				if value == nil {
					delete(fields, field)
				}
			}
			body, err = json.Marshal(fields)
		}
		require.NoError(t, err)
		c, recorder := newAPIRequest(method, api_v0.PathAwsEksKubernetesRuntimeDefinitions+"/:id", fmt.Sprint(*definition.ID), string(body))
		if method == http.MethodPatch {
			require.NoError(t, handler.UpdateAwsEksKubernetesRuntimeDefinition(c))
		} else {
			require.NoError(t, handler.ReplaceAwsEksKubernetesRuntimeDefinition(c))
		}
		return recorder.Code, recorder.Body.String()
	}
	stored := func(runtime *protectedEksRuntime) *bool {
		var definition api_v0.AwsEksKubernetesRuntimeDefinition
		require.NoError(t, db.First(&definition, *runtime.eksDefinition.ID).Error)
		return definition.DeletionProtection
	}

	for _, method := range []string{http.MethodPut, http.MethodPatch} {
		code, body := update(method, unprotected.eksDefinition, true)
		assert.Equal(t, http.StatusBadRequest, code, method)
		assert.Contains(t, body, "can't be turned on for a definition that has runtime instances", method)
		assert.Equal(t, util.Ptr(false), stored(unprotected), method)
	}

	code, body := update(http.MethodPut, unprotected.eksDefinition, false)
	assert.Equal(t, http.StatusOK, code, "a replace that leaves it off: %s", body)

	code, body = update(http.MethodPatch, protected.eksDefinition, true)
	assert.Equal(t, http.StatusOK, code, "leaving it on: %s", body)
	code, body = update(http.MethodPut, protected.eksDefinition, false)
	assert.Equal(t, http.StatusOK, code, "turning it off: %s", body)
	assert.Equal(t, util.Ptr(false), stored(protected))

	code, body = update(http.MethodPatch, empty.eksDefinition, true)
	assert.Equal(t, http.StatusOK, code, "a definition with no instances: %s", body)
	assert.Equal(t, util.Ptr(true), stored(empty))
}
