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

To perform a downgrade you can rum:
```bash
db-migrator down
```
