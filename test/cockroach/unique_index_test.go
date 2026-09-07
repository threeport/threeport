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

// The API soft deletes its objects, so every unique index it declares is
// partial on deleted_at IS NULL and a deleted row gives up the value it held.
// An index without that predicate keeps the value until the database hard
// deletes the row, and a caller recreating what it deleted gets back a conflict
// it cannot clear.
//
// A unique index also treats every NULL as distinct, so a nullable guarded
// column takes no slot while it holds no value. Without that, an optional
// guarded column would be optional for one row only:
// https://docs.cockroachlabs.com/docs/stable/unique
//
// The write path recognizes a rejected write only as a pgx typed error carrying
// SQLSTATE 23505, and answers it 409 naming the API fields behind the
// conflicting columns while the index name goes to the log.

// TestUniqueViolationCarriesTheSqlstateAndConstraint asserts a duplicate write
// arrives as the typed driver error the classifier turns into a conflict.
func TestUniqueViolationCarriesTheSqlstateAndConstraint(t *testing.T) {
	reference := newReference("Workload", 1, "Gateway", 1, api_v0.RelationshipDescribes)
	require.NoError(t, testDb.Create(&reference).Error, "the first reference is accepted")

	// the repeated pair collides on the index over both sides of the reference,
	// which no relationship narrows
	duplicate := newReference("Workload", 1, "Gateway", 1, api_v0.RelationshipDescribes)
	err := testDb.Create(&duplicate).Error
	require.Error(t, err, "the duplicate pair is rejected")

	var pgErr *pgconn.PgError
	require.True(t, errors.As(err, &pgErr), "the rejection arrives as a typed driver error: %v", err)
	assert.Equal(t, "23505", pgErr.Code, "the rejection carries the unique violation sqlstate")
	assert.NotEmpty(t, pgErr.ConstraintName, "the rejection names the index that refused the write")

	conflict := apiserver_lib.UniqueViolation(err, new(api_v0.AttachedObjectReference))
	require.NotNil(t, conflict, "the classifier reports a conflict")
	assert.NotEmpty(t, conflict.Constraint, "the classifier recovers the index name for the log")
}

// TestPartialUniqueIndexReleasesOnSoftDelete asserts a soft delete of one
// marriage frees the base to marry a different attacher.
func TestPartialUniqueIndexReleasesOnSoftDelete(t *testing.T) {
	married := newReference("Workload", 20, "Gateway", 20, api_v0.RelationshipMarries)
	require.NoError(t, testDb.Create(&married).Error, "the first marriage is accepted")

	second := newReference("Workload", 20, "Gateway", 21, api_v0.RelationshipMarries)
	require.Error(t, testDb.Create(&second).Error, "a second live marriage on the same base is refused")

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

// TestRecreatingASoftDeletedReferenceIsAccepted asserts a soft delete frees the
// attachment pair for a new reference between the same two objects.
func TestRecreatingASoftDeletedReferenceIsAccepted(t *testing.T) {
	reference := newReference("Workload", 50, "Gateway", 50, api_v0.RelationshipDescribes)
	require.NoError(t, testDb.Create(&reference).Error, "the first reference is accepted")
	require.NoError(t, testDb.Delete(&reference).Error, "the reference is soft deleted")

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

// fullTableUnique is a model whose unique index carries no deleted_at
// predicate, the index shape the API types avoid.
type fullTableUnique struct {
	gorm.Model

	// The value the unique index guards
	Slot *string `gorm:"uniqueIndex:idx_full_table_unique"`
}

// TestFullTableUniqueIndexHoldsAfterSoftDelete asserts an index over the whole
// table refuses a value a soft-deleted row still holds.
func TestFullTableUniqueIndexHoldsAfterSoftDelete(t *testing.T) {
	require.NoError(t, testDb.AutoMigrate(&fullTableUnique{}), "the table is built")

	slot := "one-per-slot"
	row := fullTableUnique{Slot: &slot}
	require.NoError(t, testDb.Create(&row).Error, "the first row is accepted")
	require.NoError(t, testDb.Delete(&row).Error, "the row is soft deleted")

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

// TestGeneratedHandlerAnswers409OnUniqueViolation asserts a generated add
// handler answers a duplicate attachment with a conflict naming no index.
func TestGeneratedHandlerAnswers409OnUniqueViolation(t *testing.T) {
	registerValidateTags(api_v0.ObjectTypeAttachedObjectReference, new(api_v0.AttachedObjectReference))
	handler := handlers.Handler{DB: testDb, Logger: zap.NewNop()}

	body := `{"ObjectType":"Workload","ObjectID":40,"AttachedObjectType":"Gateway","AttachedObjectID":40}`

	created, _ := newCreateRequest(api_v0.PathAttachedObjectReferences, body)
	require.NoError(t, handler.AddAttachedObjectReference(created))

	conflicted, recorder := newCreateRequest(api_v0.PathAttachedObjectReferences, body)
	require.NoError(t, handler.AddAttachedObjectReference(conflicted))

	assert.Equal(t, http.StatusConflict, recorder.Code,
		"a write the index refused is answered as a conflict the client can act on")
	assert.NotContains(t, recorder.Body.String(), "idx_",
		"the response does not name the index that refused the write")
}

// nullableSlot is a model whose partial unique index guards a column that
// accepts a NULL.
type nullableSlot struct {
	gorm.Model

	// The value the unique index guards, NULL until a caller sets it
	Slot *string `gorm:"uniqueIndex:idx_nullable_slot,where:deleted_at IS NULL"`
}

// TestUniqueIndexTreatsEveryNullAsDistinct asserts rows holding no value all
// pass the index while a repeated value does not.
func TestUniqueIndexTreatsEveryNullAsDistinct(t *testing.T) {
	require.NoError(t, testDb.AutoMigrate(&nullableSlot{}), "the table is built")

	for range 3 {
		assert.NoError(t, testDb.Create(&nullableSlot{}).Error,
			"a row leaving the guarded column unset takes no slot in the index")
	}

	slot := "one-per-slot"
	require.NoError(t, testDb.Create(&nullableSlot{Slot: &slot}).Error,
		"the first row carrying a value is accepted")

	err := testDb.Create(&nullableSlot{Slot: &slot}).Error
	require.Error(t, err, "a second row carrying the same value is refused")

	conflict := apiserver_lib.UniqueViolation(err, new(nullableSlot))
	assert.NotNil(t, conflict, "the refusal is a unique violation")
}

// TestGeneratedHandlerAnswers409OnDuplicateName asserts a second object under a
// taken name draws a conflict naming the field rather than the index.
func TestGeneratedHandlerAnswers409OnDuplicateName(t *testing.T) {
	registerValidateTags(api_v0.ObjectTypeDomainNameDefinition, new(api_v0.DomainNameDefinition))
	handler := handlers.Handler{DB: testDb, Logger: zap.NewNop()}
	body := newDomainNameDefinitionBody("duplicate-name")

	created, _ := newCreateRequest(api_v0.PathDomainNameDefinitions, body)
	require.NoError(t, handler.AddDomainNameDefinition(created))

	// the handler runs no name lookup of its own, so the index is what refuses
	// the second object
	conflicted, recorder := newCreateRequest(api_v0.PathDomainNameDefinitions, body)
	require.NoError(t, handler.AddDomainNameDefinition(conflicted))

	assert.Equal(t, http.StatusConflict, recorder.Code,
		"a second object under the same name is refused as a conflict")
	assert.Contains(t, recorder.Body.String(), "Name",
		"the response names the field the write collided on")
	assert.NotContains(t, recorder.Body.String(), "idx_",
		"the response does not name the index that refused the write")
}

// TestNameIsAcceptedAgainAfterSoftDelete asserts a soft delete frees the name
// for the next object the handler creates.
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

// newDomainNameDefinitionBody returns a create request body under name,
// carrying every other field the object requires.
func newDomainNameDefinitionBody(name string) string {
	return fmt.Sprintf(
		`{"Name":%q,"Domain":"example.com","Zone":"public","AdminEmail":"admin@example.com"}`,
		name,
	)
}

// newReference returns an unsaved attached object reference joining a base
// object to an attaching object under relationship.
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

// newCreateRequest returns a POST context over body and the recorder holding
// the response, wired with the binder, validator, and context wrapper the API
// server installs.
func newCreateRequest(route, body string) (*apiserver_lib.CustomContext, *httptest.ResponseRecorder) {
	e := echo.New()
	e.Binder = apiserver_lib.NewQueryBinder()

	// register the custom validations; the validator panics on a tag it has no
	// function for
	validate := validator.New()
	validate.RegisterValidation("optional", apiserver_lib.IsOptional)
	validate.RegisterValidation("association", apiserver_lib.IsAssociation)
	validate.RegisterValidation("ISO8601date", apiserver_lib.IsISO8601Date)
	e.Validator = &apiserver_lib.CustomValidator{Validator: validate}

	req := httptest.NewRequest(http.MethodPost, route, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)

	recorder := httptest.NewRecorder()
	c := e.NewContext(req, recorder)

	// the payload check reads the API version off the route pattern
	c.SetPath(route)

	return &apiserver_lib.CustomContext{Context: c}, recorder
}

// registerValidateTags parses the validate tags on obj and registers them under
// objectType, which the handler's payload check answers 500 without. The
// versions package does this for every object when the server starts.
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
