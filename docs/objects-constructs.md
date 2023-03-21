# Objects & Constructs

The Threeport API includes two different API endpoints:

* Object endpoints: these are used to create, update and delete individual
  objects.
* Construct endpoints: these are used to create and update groups of related
  objects.  They cannot be used for deletion.

## Objects

These objects must be created one at a time.  Related objects are never created
or updated through these APIs.  This is to maintain safety and prevent
unintended changes.

For example the `/v0/users` endpoint will allow you to create, update or delete
one user at a time - and only a User object.

## Constructs

For efficiency and convenience, construct APIs are offered.  These can be
either:

* A set construct that allows clients to create multiple identical objects at
  once.
  For example, the `/v0/usersets` endpoint will let clients create multiple
  users with a single request to the API.  This is always a transactional insert
  to the database, i.e. either all users are created or zero users are created.
* A relational construct that allows clients to create multiple related objects
  at once.
  For example, the `/v0/workloads` endpoint will let clients create a
  WorkloadDefinition and WorkloadInstance with one request.  This is also a
  transactional insert to the database to prevent dangling objects polluting the
  system.

The DELETE verb is never used for construct endpoints to prevent inadvertent
object deletion.  Deletions must be performed one object per request to prevent
unintended destructive results.

