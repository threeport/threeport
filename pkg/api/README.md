# pkg/api

The packages here contain the data model for the threeport API.  The types
represent the objects that are manipulated through the API and often correspond to
tables in the database, the type fields to the columns of those tables.

The generated code for the objects includes all the needed methods, the string
constants that represent their object types (used in responses to client
requests), the NATS subject names that are used for notifications to
the controllers as well as the REST paths that are used by the APIs request
router.

The objects are versioned so that backward compatibility can be maintained.
When required fields are added to an object or when fields are removed, a new
version for that API object must be created.

