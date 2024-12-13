// generated by 'threeport-sdk gen' but will not be regenerated - intended for modification

package migrations

import (
	"context"
	"database/sql"
	"fmt"

	goose "github.com/pressly/goose/v3"
	v0 "github.com/threeport/threeport/pkg/api/v0"
)

func init() {
	goose.AddMigrationNoTxContext(Up000001, Down000001)
}

func Up000001(ctx context.Context, db *sql.DB) error {
	gormDb, err := getGormDbFromContext(ctx)
	if err != nil {
		return err
	}

	if err := gormDb.AutoMigrate(dbInterfaces000001()...); err != nil {
		return fmt.Errorf("could not run gorm AutoMigrate: %w", err)
	}

	return nil
}

func Down000001(ctx context.Context, db *sql.DB) error {
	gormDb, err := getGormDbFromContext(ctx)
	if err != nil {
		return err
	}

	tablesToDrop := dbInterfaces000001()
	for _, table := range tablesToDrop {
		if err := gormDb.Migrator().DropTable(table); err != nil {
			return fmt.Errorf("could not drop table with gorm db: %w", err)
		}
	}

	return nil
}

func dbInterfaces000001() []interface{} {
	return []interface{}{
		&v0.AttachedObjectReference{},
		&v0.AwsAccount{},
		&v0.AwsEksKubernetesRuntimeDefinition{},
		&v0.AwsEksKubernetesRuntimeInstance{},
		&v0.AwsObjectStorageBucketDefinition{},
		&v0.AwsObjectStorageBucketInstance{},
		&v0.AwsRelationalDatabaseDefinition{},
		&v0.AwsRelationalDatabaseInstance{},
		&v0.KubernetesRuntimeDefinition{},
		&v0.KubernetesRuntimeInstance{},
		&v0.ControlPlaneDefinition{},
		&v0.ControlPlaneInstance{},
		&v0.ControlPlaneComponent{},
		&v0.Definition{},
		&v0.DomainNameDefinition{},
		&v0.DomainNameInstance{},
		&v0.Event{},
		&v0.ExtensionApi{},
		&v0.ExtensionApiRoute{},
		&v0.GatewayDefinition{},
		&v0.GatewayHttpPort{},
		&v0.GatewayInstance{},
		&v0.GatewayTcpPort{},
		&v0.HelmWorkloadDefinition{},
		&v0.HelmWorkloadInstance{},
		&v0.Instance{},
		&v0.LogBackend{},
		&v0.LogStorageDefinition{},
		&v0.LogStorageInstance{},
		&v0.LoggingDefinition{},
		&v0.LoggingInstance{},
		&v0.MetricsDefinition{},
		&v0.MetricsInstance{},
		&v0.ObservabilityDashboardDefinition{},
		&v0.ObservabilityDashboardInstance{},
		&v0.ObservabilityStackDefinition{},
		&v0.ObservabilityStackInstance{},
		&v0.Profile{},
		&v0.SecretDefinition{},
		&v0.SecretInstance{},
		&v0.TerraformDefinition{},
		&v0.TerraformInstance{},
		&v0.Tier{},
		&v0.WorkloadDefinition{},
		&v0.WorkloadEvent{},
		&v0.WorkloadInstance{},
		&v0.WorkloadResourceDefinition{},
		&v0.WorkloadResourceInstance{},
	}
}
