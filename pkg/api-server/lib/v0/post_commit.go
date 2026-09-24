package v0

import (
	"fmt"

	"gorm.io/gorm"
)

// Some API objects carry process state that has to agree with the database:
// the module router is one, a map of paths this process proxies. Keeping that
// state in step from a GORM persist hook does not work, because a hook runs
// inside the transaction and so acts on a write that may never commit.
//
// The interfaces below let an object bring its process state in line once the
// transaction has committed. Handlers call them after the write succeeds and
// before the response is written, so a client that gets a 200 can rely on the
// state being in place.
//
// An object that has nothing to do implements none of them, which is every
// object but ModuleApiRoute today.

// PostCommitCreator is implemented by API objects with process state to
// reconcile after a create commits.
type PostCommitCreator interface {
	AfterCommitCreate(db *gorm.DB) error
}

// PostCommitDeleter is implemented by API objects with process state to
// reconcile after a delete commits.
type PostCommitDeleter interface {
	AfterCommitDelete(db *gorm.DB) error
}

// AfterCommitCreate reconciles an object's process state after its create has
// committed, and does nothing for an object that has none.
func AfterCommitCreate(db *gorm.DB, object any) error {
	creator, ok := object.(PostCommitCreator)
	if !ok {
		return nil
	}

	if err := creator.AfterCommitCreate(db); err != nil {
		return fmt.Errorf("failed to reconcile state after create committed: %w", err)
	}

	return nil
}

// AfterCommitDelete reconciles an object's process state after its delete has
// committed, and does nothing for an object that has none.
func AfterCommitDelete(db *gorm.DB, object any) error {
	deleter, ok := object.(PostCommitDeleter)
	if !ok {
		return nil
	}

	if err := deleter.AfterCommitDelete(db); err != nil {
		return fmt.Errorf("failed to reconcile state after delete committed: %w", err)
	}

	return nil
}
