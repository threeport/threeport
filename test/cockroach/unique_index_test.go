package cockroach

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"

	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	handlers "github.com/threeport/threeport/pkg/api-server/v0/handlers"
	api_lib "github.com/threeport/threeport/pkg/api/lib/v0"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// Unique indexes on API objects are partial on deleted_at IS NULL, so a
// soft-deleted row frees its name. The write path maps SQLSTATE 23505 to 409.

// TestUniqueViolationCarriesTheSqlstateAndConstraint covers a duplicate write as 23505.
func TestUniqueViolationCarriesTheSqlstateAndConstraint(t *testing.T) {
	reference := newReference("Workload", 1, "Gateway", 1, api_v0.RelationshipDescribes)
	require.NoError(t, testDb.Create(&reference).Error, "the first reference is accepted")

	duplicate := newReference("Workload", 1, "Gateway", 1, api_v0.RelationshipDescribes)
	err := testDb.Create(&duplicate).Error
	require.Error(t, err, "the duplicate pair is rejected")

	// live Cockroach returns a typed pgx error, not sqlite's string
	var pgErr *pgconn.PgError
	require.True(t, errors.As(err, &pgErr), "the rejection arrives as a typed driver error: %v", err)
	assert.Equal(t, "23505", pgErr.Code, "the rejection carries the unique violation sqlstate")
	assert.NotEmpty(t, pgErr.ConstraintName, "the rejection names the index that refused the write")

	conflict := apiserver_lib.UniqueViolation(err, new(api_v0.AttachedObjectReference))
	require.NotNil(t, conflict, "the classifier reports a conflict")
	assert.NotEmpty(t, conflict.Constraint, "the classifier recovers the index name for the log")
}

// TestPartialUniqueIndexReleasesOnSoftDelete covers freeing a marriage on soft delete.
func TestPartialUniqueIndexReleasesOnSoftDelete(t *testing.T) {
	married := newReference("Workload", 20, "Gateway", 20, api_v0.RelationshipMarries)
	require.NoError(t, testDb.Create(&married).Error, "the first marriage is accepted")

	second := newReference("Workload", 20, "Gateway", 21, api_v0.RelationshipMarries)
	require.Error(t, testDb.Create(&second).Error, "a second live marriage on the same base is refused")

	// soft delete frees the unique slot; the row is still in the table
	require.NoError(t, testDb.Delete(&married).Error, "the marriage is soft deleted")
	var remaining int64
	require.NoError(t,
		testDb.Unscoped().Model(&api_v0.AttachedObjectReference{}).
			Where("id = ?", *married.ID).Count(&remaining).Error,
	)
	require.Equal(t, int64(1), remaining, "the soft-deleted row is still in the table")

	remarried := newReference("Workload", 20, "Gateway", 22, api_v0.RelationshipMarries)
	assert.NoError(t, testDb.Create(&remarried).Error,
		"the base is married again once the first marriage is soft deleted")
}

// TestRecreatingASoftDeletedReferenceIsAccepted covers recreating a soft-deleted pair.
func TestRecreatingASoftDeletedReferenceIsAccepted(t *testing.T) {
	reference := newReference("Workload", 50, "Gateway", 50, api_v0.RelationshipDescribes)
	require.NoError(t, testDb.Create(&reference).Error, "the first reference is accepted")
	require.NoError(t, testDb.Delete(&reference).Error, "the reference is soft deleted")

	// pair is free again while the tombstone row remains

	var remaining int64
	require.NoError(t,
		testDb.Unscoped().Model(&api_v0.AttachedObjectReference{}).
			Where("id = ?", *reference.ID).Count(&remaining).Error,
	)
	require.Equal(t, int64(1), remaining, "the soft-deleted row is still in the table")

	recreated := newReference("Workload", 50, "Gateway", 50, api_v0.RelationshipDescribes)
	assert.NoError(t, testDb.Create(&recreated).Error,
		"the same pair is accepted again once the first reference is soft deleted")
}

// fullTableUnique has a unique index with no deleted_at predicate.
type fullTableUnique struct {
	gorm.Model
	Slot *string `gorm:"uniqueIndex:idx_full_table_unique"`
}

// TestFullTableUniqueIndexHoldsAfterSoftDelete covers a unique value held after soft delete.
func TestFullTableUniqueIndexHoldsAfterSoftDelete(t *testing.T) {
	require.NoError(t, testDb.AutoMigrate(&fullTableUnique{}), "the table is built")

	slot := "one-per-slot"
	row := fullTableUnique{Slot: &slot}
	require.NoError(t, testDb.Create(&row).Error, "the first row is accepted")
	require.NoError(t, testDb.Delete(&row).Error, "the row is soft deleted")

	// gorm hides the row; the unique index still holds the value
	var found fullTableUnique
	assert.ErrorIs(t, testDb.Where("id = ?", row.ID).First(&found).Error, gorm.ErrRecordNotFound,
		"the soft-deleted row reads as absent")

	recreated := fullTableUnique{Slot: &slot}
	err := testDb.Create(&recreated).Error
	require.Error(t, err, "the slot is still held after the soft delete")

	conflict := apiserver_lib.UniqueViolation(err, new(fullTableUnique))
	assert.NotNil(t, conflict,
		"the refusal is a unique violation, so a client is answered 409 it cannot clear")
}

// TestGeneratedHandlerAnswers409OnUniqueViolation covers a 409 that omits the index name.
func TestGeneratedHandlerAnswers409OnUniqueViolation(t *testing.T) {
	registerValidateTags(api_v0.ObjectTypeAttachedObjectReference, new(api_v0.AttachedObjectReference))
	handler := handlers.Handler{DB: testDb, Logger: zap.NewNop()}

	body := `{"ObjectType":"Workload","ObjectID":40,"AttachedObjectType":"Gateway","AttachedObjectID":40}`

	created, _ := newCreateRequest(api_v0.PathAttachedObjectReferences, body)
	require.NoError(t, handler.AddAttachedObjectReference(created))

	// second create hits the unique index and must 409 without naming it
	conflicted, recorder := newCreateRequest(api_v0.PathAttachedObjectReferences, body)
	require.NoError(t, handler.AddAttachedObjectReference(conflicted))

	assert.Equal(t, http.StatusConflict, recorder.Code,
		"a write the index refused is answered as a conflict the client can act on")
	assert.NotContains(t, recorder.Body.String(), "idx_",
		"the response does not name the index that refused the write")
}

// nullableSlot has a partial unique index on a nullable column.
type nullableSlot struct {
	gorm.Model
	Slot *string `gorm:"uniqueIndex:idx_nullable_slot,where:deleted_at IS NULL"`
}

// TestUniqueIndexTreatsEveryNullAsDistinct covers NULLs not colliding with each other.
func TestUniqueIndexTreatsEveryNullAsDistinct(t *testing.T) {
	require.NoError(t, testDb.AutoMigrate(&nullableSlot{}), "the table is built")

	for range 3 {
		assert.NoError(t, testDb.Create(&nullableSlot{}).Error,
			"a row leaving the guarded column unset takes no slot in the index")
	}

	// a repeated value still collides
	slot := "one-per-slot"
	require.NoError(t, testDb.Create(&nullableSlot{Slot: &slot}).Error,
		"the first row carrying a value is accepted")

	err := testDb.Create(&nullableSlot{Slot: &slot}).Error
	require.Error(t, err, "a second row carrying the same value is refused")

	conflict := apiserver_lib.UniqueViolation(err, new(nullableSlot))
	assert.NotNil(t, conflict, "the refusal is a unique violation")
}

// TestGeneratedHandlerAnswers409OnDuplicateName covers a 409 that names the field.
func TestGeneratedHandlerAnswers409OnDuplicateName(t *testing.T) {
	registerValidateTags(api_v0.ObjectTypeDomainNameDefinition, new(api_v0.DomainNameDefinition))
	handler := handlers.Handler{DB: testDb, Logger: zap.NewNop()}
	body := newDomainNameDefinitionBody("duplicate-name")

	created, _ := newCreateRequest(api_v0.PathDomainNameDefinitions, body)
	require.NoError(t, handler.AddDomainNameDefinition(created))

	// 409 names the field, not the index
	conflicted, recorder := newCreateRequest(api_v0.PathDomainNameDefinitions, body)
	require.NoError(t, handler.AddDomainNameDefinition(conflicted))

	assert.Equal(t, http.StatusConflict, recorder.Code,
		"a second object under the same name is refused as a conflict")
	assert.Contains(t, recorder.Body.String(), "Name",
		"the response names the field the write collided on")
	assert.NotContains(t, recorder.Body.String(), "idx_",
		"the response does not name the index that refused the write")
}

// TestNameIsAcceptedAgainAfterSoftDelete covers reusing a name after soft delete.
func TestNameIsAcceptedAgainAfterSoftDelete(t *testing.T) {
	registerValidateTags(api_v0.ObjectTypeDomainNameDefinition, new(api_v0.DomainNameDefinition))
	handler := handlers.Handler{DB: testDb, Logger: zap.NewNop()}
	const name = "reused-after-delete"
	body := newDomainNameDefinitionBody(name)

	created, _ := newCreateRequest(api_v0.PathDomainNameDefinitions, body)
	require.NoError(t, handler.AddDomainNameDefinition(created))

	var definition api_v0.DomainNameDefinition
	require.NoError(t, testDb.Where("name = ?", name).First(&definition).Error)
	require.NoError(t, testDb.Delete(&definition).Error, "the object is soft deleted")

	// row remains; the partial index does not hold the name

	var remaining int64
	require.NoError(t,
		testDb.Unscoped().Model(&api_v0.DomainNameDefinition{}).
			Where("id = ?", *definition.ID).Count(&remaining).Error,
	)
	require.Equal(t, int64(1), remaining, "the soft-deleted row is still in the table")

	recreated, recorder := newCreateRequest(api_v0.PathDomainNameDefinitions, body)
	require.NoError(t, handler.AddDomainNameDefinition(recreated))
	assert.Equal(t, http.StatusCreated, recorder.Code,
		"the name is free again once the object holding it is soft deleted")
}

// newDomainNameDefinitionBody returns a create body with the required fields set.
func newDomainNameDefinitionBody(name string) string {
	return fmt.Sprintf(
		`{"Name":%q,"Domain":"example.com","Zone":"public","AdminEmail":"admin@example.com"}`,
		name,
	)
}

// newReference returns an unsaved attached object reference.
func newReference(
	objectType string,
	objectID uint,
	attachedType string,
	attachedID uint,
	relationship api_v0.Relationship,
) api_v0.AttachedObjectReference {
	return api_v0.AttachedObjectReference{
		ObjectType:         util.Ptr(objectType),
		ObjectID:           util.Ptr(objectID),
		AttachedObjectType: util.Ptr(attachedType),
		AttachedObjectID:   util.Ptr(attachedID),
		Relationship:       util.Ptr(relationship),
	}
}

// newCreateRequest returns a POST context and recorder wired like the API server.
func newCreateRequest(route, body string) (*apiserver_lib.CustomContext, *httptest.ResponseRecorder) {
	e := echo.New()
	e.Binder = apiserver_lib.NewQueryBinder()

	// validator panics on a tag it has no function for
	validate := validator.New()
	validate.RegisterValidation("optional", apiserver_lib.IsOptional)
	validate.RegisterValidation("association", apiserver_lib.IsAssociation)
	validate.RegisterValidation("ISO8601date", apiserver_lib.IsISO8601Date)
	e.Validator = &apiserver_lib.CustomValidator{Validator: validate}

	req := httptest.NewRequest(http.MethodPost, route, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)

	recorder := httptest.NewRecorder()
	c := e.NewContext(req, recorder)
	c.SetPath(route)

	return &apiserver_lib.CustomContext{Context: c}, recorder
}

// registerValidateTags registers obj's validate tags, as the API server does at start.
func registerValidateTags(objectType string, obj any) {
	taggedFields := map[string]*apiserver_lib.FieldsByTag{
		string(api_lib.ValidateTag): {
			TagName:              string(api_lib.ValidateTag),
			Required:             []string{},
			Optional:             []string{},
			OptionalAssociations: []string{},
		},
	}
	apiserver_lib.ParseStruct(
		string(api_lib.ValidateTag),
		reflect.ValueOf(obj),
		"",
		apiserver_lib.Translate,
		taggedFields,
	)
	apiserver_lib.ObjectTaggedFields[apiserver_lib.VersionObject{
		Version: "v0",
		Object:  objectType,
	}] = taggedFields[string(api_lib.ValidateTag)]
}
