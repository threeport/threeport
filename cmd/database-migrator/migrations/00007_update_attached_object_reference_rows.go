package migrations

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"
	v1 "github.com/threeport/threeport/pkg/api/v1"
	util "github.com/threeport/threeport/pkg/util/v0"
)

func init() {
	goose.AddMigrationContext(Up00007, Down00007)
}

func Up00007(ctx context.Context, tx *sql.Tx) error {
	// map type -> attached_object_type
	if _, err := tx.ExecContext(ctx, `
		UPDATE attached_object_references
		SET attached_object_type = type;
	`); err != nil {
		return fmt.Errorf("failed to copy type to attached_object_type: %w", err)
	}

	// map object_id -> attached_object_id
	if _, err := tx.ExecContext(ctx, `
		UPDATE attached_object_references
		SET attached_object_id = object_id;
	`); err != nil {
		return fmt.Errorf("failed to set attached_object_id to object_id: %w", err)
	}

	// map object_type -> 'v1.WorkloadInstance'
	if _, err := tx.ExecContext(ctx,
		fmt.Sprintf(`UPDATE attached_object_references
					 SET object_type = '%s';`,
			util.TypeName(v1.WorkloadInstance{}),
		)); err != nil {
		return fmt.Errorf("failed to set object_type to 'v1.WorkloadInstance': %w", err)
	}

	// map workload_instance_id -> object_id
	if _, err := tx.ExecContext(ctx, `
		UPDATE attached_object_references
		SET object_id = workload_instance_id;
	`); err != nil {
		return fmt.Errorf("failed to set object_id to workload_instance_id: %w", err)
	}

	return nil
}

func Down00007(ctx context.Context, tx *sql.Tx) error {
	// map object_id -> workload_instance_id
	if _, err := tx.ExecContext(ctx, `
		UPDATE attached_object_references
		SET workload_instance_id = object_id;
	`); err != nil {
		return fmt.Errorf("failed to set workload_instance_id to object_id: %w", err)
	}

	// map attached_object_id -> object_id
	if _, err := tx.ExecContext(ctx, `
		UPDATE attached_object_references
		SET object_id = attached_object_id;
	`); err != nil {
		return fmt.Errorf("failed to set object_id to attached_object_id: %w", err)
	}

	// map attached_object_type -> type
	if _, err := tx.ExecContext(ctx, `
		UPDATE attached_object_references
		SET type = attached_object_type;
	`); err != nil {
		return fmt.Errorf("failed to copy attached_object_type to type: %w", err)
	}

	return nil
}
