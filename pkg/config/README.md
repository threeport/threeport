# pkg/config

This package contains the config objects for client programs.  It is used by
`tptctl` to provide abstractions to the raw API fields for users.  It allows
configurations to be simplified and removes the need to reference objects by the
unique ID used in the database.  Users can reference objects by name and the
client tooling can look up objects by name and then interact with the API using
unique IDs.

The config packages are versioned so that changes can be made to config
structures without breaking backward compatibility for tools that use them.

