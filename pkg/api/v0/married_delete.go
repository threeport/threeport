package v0

import (
	"errors"
	"fmt"

	"gorm.io/gorm"

	lib "github.com/threeport/threeport/pkg/api/lib/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// MarriedBasesProvider is implemented by every API type with a foreign key
// tagged `relationship:"marries"`.  The SDK generates MarriedBases for it.
type MarriedBasesProvider interface {
	// MarriedBases returns a new object of each type the object marries, with
	// only its ID set from the foreign key, skipping a foreign key that is
	// not set.
	MarriedBases() []any
}

// DeleteWithMarriedBases deletes obj and, in the same transaction, each base
// object it marries.  An object that marries a base is one unit with it, so
// the delete handler removes both in one request: a client that deleted the
// base with a second request could fail between the two and leave the base
// with nothing that names it.
//
// obj is deleted first, which removes its marries reference, and then each
// base by the foreign key obj stored.  A base that is already gone, or whose
// own delete is underway, is left as it is.  A base that cannot be deleted,
// because something else depends on it or it refuses the delete, fails the
// whole transaction, so obj is kept and the same delete can be retried.
//
// The base is deleted directly, not scheduled for its reconciler: its
// reconciler's delete removes the married object, which is already gone.
func DeleteWithMarriedBases(db *gorm.DB, obj any) *gorm.DB {
	provider, ok := obj.(MarriedBasesProvider)
	if !ok {
		return db.Delete(obj)
	}
	bases := provider.MarriedBases()
	if len(bases) == 0 {
		return db.Delete(obj)
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(obj).Error; err != nil {
			return err
		}
		for _, base := range bases {
			if err := deleteMarriedBase(tx, base); err != nil {
				return err
			}
		}
		return nil
	})
	result := db.Session(&gorm.Session{NewDB: true})
	if err != nil {
		_ = result.AddError(err)
	}
	return result
}

// deleteMarriedBase deletes base, whose ID is set, in tx.
func deleteMarriedBase(tx *gorm.DB, base any) error {
	baseType := base.(lib.FullyQualifiedTypeProvider).GetFullyQualifiedType()
	baseID := util.ObjectID(base)
	if baseID == nil {
		return nil
	}

	// a base that is gone, or that its own delete is removing, is left alone
	loaded := tx.Limit(1).Find(base)
	if loaded.Error != nil {
		return fmt.Errorf("failed to load married %s/%d: %w", baseType, *baseID, loaded.Error)
	}
	if loaded.RowsAffected == 0 || deletionMarked(base) {
		return nil
	}

	// a base that refuses its delete keeps the married object too
	if err := CheckDeleteRefused(tx, base); err != nil {
		var refused *DeleteRefusedError
		if errors.As(err, &refused) {
			return util.NewConflictError(fmt.Sprintf(
				"married %s/%d %s: %s", baseType, *baseID, ErrMsgDeleteBlocked, refused.Error(),
			))
		}
		return err
	}

	// the base's delete hooks refuse it while anything else depends on it
	if err := tx.Delete(base).Error; err != nil {
		return err
	}

	return nil
}
