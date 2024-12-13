# Threeport Data Model Updates

The Threeport data model is the collection of objects that are available in the
Threeport API that are used to represent elements of applications and their
dependencies.  These objects are available to users to create, update and delete
through the API and the Threeport controllers are responsible for reconciling
the existing state of the system to represent what users define for them in the
Threeport API.

Threeport uses a relational database with strict schemas that must be updated to
match the application when data model changes occur.

## Threeport Upgrades

The data models were introduced at version v0.4.1 for Threeport.  In order to
provide reliable upgrades and roll-backs while keeping the DB schema intact,
new installations of Threeport, begin with the database schema for v0.4.1 and
are upgraded with all of the migrations to the reach the latest version.

When v1 of Threeport is released, those migrations can be retired as backward
compatibility with v0 will not be provided.

## Data Model Changes

When a data model changes, a database migration must be built that modifies the
tables in the database.

Database migrations live in `cmd/database-migrator/migrations`.  Migrations that
peform an `up` operation for upgrades and a `down` operation for roll-backs must
be implemented.  When adding a migration, add a new file with the appropriate
sequential prefix.  See existing migrations for examples.

## Testing Migrations

1. Spin up a dev Threeport environment.
1. Check the tables in the database.  Use `make dev-query-crdb` to get a SQL
   prompt for the running Cockroach DB instance.  Use `\dt` to check tables and
   `\d [table name]` to check table schemas.
1. Make the database migration changes needed.  See existing migrations for
   examples on how to add, update and remove tables as needed.
1. Build new rest-api and database-migrator images that contain your DB
   migration:

   ```bash
   make build-tptdev
   ./bin/tptdev build -r $TEST_REPO -t $TEST_TAG --load --names database-migrator
   ./bin/tptdev build -r $TEST_REPO -t $TEST_TAG --load --names rest-api
   ```
1. Restart the REST API by killing the `threeport-api-server` pod in the dev
   environment.
1. Your migration should run when the pod restarts using the new images.  You
   can now recheck the tables in the database to ensure the DB schema has been
   updated.
1. Provided the migration was successful, now check the roll-back.  Open a
   port-forward to the Cockroach database pod:

   ```bash
   kubectl port-forward -n threeport-control-plane pod/crdb-0 26257:26257
   ```
1. Now run the down command for the database-migrator.

   ```bash
   ./bin/database-migrator down
   ```
1. Recheck the database schema.  The migration should have been rolled back to
   its original state.
1. Run the up command to apply the migration once more.

   ```bash
   ./bin/database-migrator up
   ```

Your dev environment database will now be in the up-to-date state with your
migration applied.

## Backward Compatibility

> Note: At this time, Threeport is in early development with backward
> compatibility not guaranteed.  In the near future this will change and
> backward compatibility will be enforced.

If a required field is added to an object in the data model, that will break
existing implementations that are developed against the API and a new API
version for that object must be created.

The relevant reconcilers will need to be updated to accomodate the different
versions and manage that backward compatibility.

Over time, older versions of an API object will be deprecated and then retired.

