# Database Migrations for Threeport

Threeport changes to the API will require database updates to accomodate the required schema modifications.
The database migrator will help facilitate these migrations.

The migrator uses the [pressly/goose](https://github.com/pressly/goose/tree/master) library.
The migrator treats all db operations as steps. As such, the necessary logic for a particular migration step
is present in a file with the following conventions stepnumber_description.go

## Example: update control plane instance api to include image tags as part of the API
An example of a db migrator step is the following:

00001_image_tag_cp_instance.go

In the above file, the 00001 represent the step number for the migrations and the image_tag_cp_instance is a description
for what the step deals with.

We intend to add a image tag field in the control plane instance database as part of this step.
If we open the file, we can see the following code:

```go
package main

import (
	"context"
	"database/sql"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationNoTxContext(Up00001, Down00001)
}

func Up00001(ctx context.Context, db *sql.DB) error {
	_, err := db.Exec("ALTER TABLE control_plane_instances ADD image_tag text;")
	return err
}

func Down00001(ctx context.Context, db *sql.DB) error {
	_, err := db.Exec("ALTER TABLE control_plane_instances DROP COLUMN image_tag;")
	return err
}
```

The init function will register the step with the Goose library.
The two required step functions are Up00001 and Down00001
Up is run when the automigrator has to upgrade to the step and Down is run when a downgrade from the step is required.
Generally the logic will be inverse of one another.
So in the above, we add the image_tag column to the table when upgrading and remove it when downgrading.
If backfill logic for data is needed, this is also the place to accomodate for that.

### Every migration needs a down migration

When a migration fails, the migrator rolls it back automatically (see below),
and the rollback is made of down migrations. So every migration must have one,
and it has to work in two situations:

- after its migration completed, to revert it
- after its migration failed part way. Migrations run outside a transaction,
  since CockroachDB cannot change a schema inside one, so a migration that
  fails can leave some of its changes behind. Its down migration is run to
  remove them, so it must tolerate the changes it reverts being only partly
  there or not there at all: `DROP ... IF EXISTS`, `DROP COLUMN IF EXISTS`, and
  so on.

Register every migration with `goose.AddMigrationNoTxContext`. A migration run
in a transaction would be trusted to have undone itself on failing, and
CockroachDB does not undo every schema change a failed transaction made.

`TestEveryMigrationRollsBack` in `test/cockroach` enforces all of this against a
real CockroachDB: every migration has a down migration and runs outside a
transaction, the whole chain survives up, down to nothing and up again, and
every down migration runs cleanly on a database its migration never changed.

### A new migration's version sorts above every applied one

A migration whose version sorts below one the database has already applied
was never applied, and never would be. So `up` refuses to run while such a
migration is registered, naming it, as goose's own `up` does. This happens to
a migration merged out of order with migrations applied elsewhere first: give
it a version above the newest applied one.

## What happens when a migration fails

`database-migrator up` runs as an init container of the API server. Pods of
the API server can start together, so the migrator first takes a lock kept in
the `threeport_goose_db_version_lock` table: one migrator migrates at a time,
and the next sees what it did. The lock is leased and renewed while it is
held, so the lock of a migrator whose pod was killed expires within minutes.
Before every change it makes, the migrator checks that it still holds the lock
with time left on it, so one cut off from the database long enough for another
to take over stops instead of migrating alongside it.
Holding the lock, it applies the pending migrations one at a time:

1. On a production control plane that backs its database up, it first takes
   a full backup into the backup bucket. A new install, whose database is
   empty, takes none. The backup goes where the scheduled backups go, and the
   path recorded for it is the migrator's own, read back to confirm it is
   whole, never just the newest in the bucket.
2. When a migration fails, its down migration cleans up what it left, and the
   migrations applied before it in the same run are rolled back, newest first,
   so the database returns to the version it started from.
3. The failure is recorded in the `threeport_goose_db_version_failure` table
   and the init container fails with a message saying what failed, what
   version the database is at, and, if the rollback failed too, the backup to
   restore with `tptctl restore --backup`.

While a failure is recorded, every restart of the init container fails with
the same message without migrating or backing up again. Once the cause is
fixed, clear the record so the next start migrates:

```bash
kubectl exec -n threeport-control-plane crdb-0 -- \
  /cockroach/cockroach sql --certs-dir=/cockroach/cockroach-certs \
  -d threeport_api -e "DELETE FROM threeport_goose_db_version_failure"
```


## How to run database migrator

The end goal is to have the db-migrator run as part of init container whenever we deploy a new threeport version.

For now as part of an upgrade to Threeport, we will run the db migrator executable from
the local machine. The port 26257 must be forwarded to the appropiate crdb instance. This can be achieved via
port-forward from kubectl (The Makefile command dev-forward-crdb helps achieve this.)

Goose maintains state information in the db you have connected to. This table is named as "goose_db_version".
It keeps book-keeping information regarding which steps have been applied etc.
You can view this information by running:
```bash
db-migrator status
```

To perform a upgrade you can run:
```bash
db-migrator up
```

To perform a downgrade you can run:
```bash
db-migrator down
```

Every command that changes the schema, `down` included, takes the same lock
as `up`, so it waits while an API server pod's migrator is migrating rather
than running beside it. Only a plain `up` backs the database up first and
rolls a failed migration back; the others run as goose runs them.
