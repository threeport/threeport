# Threeport RESTful API

The heart of the Threeport control plane.

Here you will find the main package for the Threeport API.  It is the
interaction point for clients to use the Threeport control plane.  The API is
responsible for persisting desired state of the system as expressed by users and
external systems, and for notifying the controllers in the system to reconcile
state accordingly.  The controllers read and write to and from the API as needed
and all coordination between controllers happen through this API - they never
interact directly with each other.  The API is built using the [echo
framework](https://github.com/labstack/echo).

## API conventions

The Qleet/Threeport REST API follows a set of conventions to make interacting
with it consistent.
This document outlines those conventions.  For detailed documentation of all
supported API objects and methods refer to the swagger docs at
`{{host}}:{{port}}/swagger/index.html` on a running API instance.

### REST URLs

Each element type on the server is represented as a top-level URL with a plural form

```text
{{protocol}}://{{host}}:{{port}}/{{rest-resource-version}}/{{rest-resource}}
```
for example
```text
https://localhost:8443/v0/workload-definitions
```

### Versioning

There are two different versions to consider with the QleetOS API:

* The version of the API release: This is represented with standard semantic
  versioning, e.g. `v1.2.3`.  This version can be retrieved at the `/version`
  endpoint.  Each released version has a list of supported versions for each
  particular API.
* The version of a particular API: This is the version of the API for a
  particular endpoint.  It is represented with a single version number. e.g.
  `v0` and is a part of the path in the URL, e.g. `{{host}}:{{port}}/v0/users`.
  It is used to maintain backward compatibility for clients of the API.  This
  allows us to update the data model for any particular object without breaking
  existing integrations.  The available version for an API can be found at the
  path `<rest-resource>/versions`.

For particular API versions, when we add new optional fields no new API version
will be created.  However, if we add any required fields or remove any existing
fields, that will result in a new API version.

### Required HTTP headers

The Threeport REST API accept an input in JSON format and return an output in
JSON format.  Following HTTP conventions, the `Content-Type` request header is
required for operations that provide JSON input, and the `Accept` request header
is required for operations that produce JSON output, with the media type value
of `application/json`.

### Basic operations

The basic create, read, update, delete operations are provided according to common REST API conventions.
- POST: Add a new object.
- GET: Retrieve one or more objects.
- PUT: Replace an existing object.  When using this method, you must provide all
  required fields.  Any optional fields will become NULL if not provided.  This
  method should be used when you want to update a record to remove an optional
  field such as a foreign key ID to another object.
- PATCH: Update one or more fields of an existing object.  This method allows
  you to provide a single field for an obejct and update _only_ that field
  without affecting any other fields.
- DELETE: Remove an object.

The top-level URL for an element type represents the collection of items of that type wrapped in a
standard `Response` object

### Request query parameters

Each REST API resource can return it's objects either idividually by `ID` or all of them (with paginations)
- `{{protocol}}://{{host}}:{{port}}/{{rest-resource-version}}/{{rest-resource}}/1` - returns a singe account with `ID=1`
    ```json
    {
        "Meta": {
            "Page": 1,
            "Size": 1,
            "TotalCount": 1
        },
        "Type": "Account",
        "Data": [
            {
                "ID": 1,
                "CreatedAt": "2022-10-26T17:43:01.267158Z",
                "UpdatedAt": "2022-10-26T17:43:01.267158Z",
                "DeletedAt": null,
                "Address": "test-address",
                "NetworkID": 1,
                "NodeID": 10000,
                "PoolID": 1
            }
        ],
        "Status": {
            "code": 200,
            "message": "OK",
            "error": ""
        }
    }
    ```
- `{{protocol}}://{{host}}:{{port}}/{{rest-resource-version}}/account` - returns all paginated account records
    ```json
    {
      "Meta": {
        "Page": 1,
        "Size": 50,
        "TotalCount": 429
      },
      "Type": "Account",
      "Data": [
        {
          "ID": 1,
          "CreatedAt": "2022-10-26T17:43:01.267158Z",
          "UpdatedAt": "2022-10-26T17:43:01.267158Z",
          "DeletedAt": null,
          "Address": "test-address",
          "NetworkID": 1,
          "NodeID": 10000,
          "PoolID": 1
        },
        ...
      ],
      "Status": {
        "code": 200,
        "message": "OK",
        "error": ""
      }
    }
    ```
Request that returns multiple records can utilize query parameters to refine the result set:
`{{protocol}}://{{host}}:{{port}}/{{rest-resource-version}}/account?address=9257b4cbb8d21b8c68acbaed25e4342065bf62b2&networkid=2&nodeid=10000&poolid=1`
Query parameter names are specific to the each REST API resource.

#### Pagination

The database behind a REST API can get very large. Sometimes, there’s so much data that it shouldn’t be returned 
all at once because it’s way too slow or will bring down our systems. Therefore, we need ways to filter items.
We also need ways to paginate data so that we only return a few results at a time.
Pagination is build in our REST API, two query parameters can be specified to modify default pagination values.

- page: page to return, default value 1
- size: number of rows per page to return, default value 50

These REST API call are equivalent:
- `{{host}}:{{port}}/v0/accounts`
- `{{host}}:{{port}}/v0/accounts?page=1&size=50`

and produces the same `JSON` Response object

```json
{
    "Meta": {
        "Page": 1,
        "Size": 50,
        "TotalCount": 429
    },
    "Type": "Account",
    "Data": [
        {
            "ID": 1,
            "CreatedAt": "2022-10-26T17:43:01.267158Z",
            "UpdatedAt": "2022-10-26T17:43:01.267158Z",
            "DeletedAt": null,
            "Address": "test-address",
            "NetworkID": 1,
            "NodeID": 10000,
            "PoolID": 1
        },
      ...
    ],
    "Status": {
        "code": 200,
        "message": "OK",
        "error": ""
    }
}
```

### Request data format

REST API supports `JSON` as a request body payload

#### Request data checks and errors

##### Empty payload check

If an empty payload is submitted via POST/PUT/PATCH
```json
{}
```
REST API resource will return HTTP error `400` with message `JSON payload is empty`.
```json
{
    "Meta": {
        "Page": 0,
        "Size": 0,
        "TotalCount": 0
    },
    "Type": "Account",
    "Data": null,
    "Status": {
        "code": 400,
        "message": "Bad Request",
        "error": "JSON payload is empty"
    }
}
```

##### Unsupported field check

If unsupported fields are encountered in a payload submitted via POST/PUT/PATCH
```json
{
  "Address":"test-address",
  "UnsupportedField1": "val1",
  "NetworkID":1,
  "PoolID":1,
  "NodeID":10000,
  "UnsupportedField1": "val2"
}
```
REST API resource will return HTTP error `400` with message 
`Unsupported fields : ` and comma de delimited list of encountered unsupported fields.
```json
{
    "Meta": {
        "Page": 0,
        "Size": 0,
        "TotalCount": 0
    },
    "Type": "Account",
    "Data": null,
    "Status": {
        "code": 400,
        "message": "Bad Request",
        "error": "Unsupported fields : UnsupportedField1, UnsupportedField2"
    }
}
```

##### Attempt to update GORM Model fields (ID, CreatedAt, UpdatedAt, DeletedAt)

If GORM Model fields (ID, CreatedAt, UpdatedAt, DeletedAt) are encountered in a payload submitted via POST/PUT/PATCH
```json
{
  "ID":429,
  "CreatedAt":"2022-10-19T23:18:15.775713Z",
  "UpdatedAt":"2022-10-19T23:18:15.775713Z",
  "DeletedAt":"2022-10-19T23:18:15.775713Z",
  "Address":"test-address",
  "NetworkID":1,
  "NodeID":10000,
  "PoolID":1
}
```
REST API resource will return HTTP error `400` with message
`Update of GORM Model fields is not allowed : ` and comma de delimited list of encountered GORM Model fields.
```json
{
  "Meta": {
    "Page": 0,
    "Size": 0,
    "TotalCount": 0
  },
  "Type": "Account",
  "Data": null,
  "Status": {
    "code": 400,
    "message": "Bad Request",
    "error": "Update of GORM Model fields is not allowed : DeletedAt,ID,CreatedAt,UpdatedAt"
  }
}
```

##### Missing required fields check

REST APIs model are defined as go type structs. Special tag `validate` is used to define field types.
There are three distinct field types:
- `validate:"required"` - mandatory in the request payload
- `validate:"optional"` - not required, but may be present in the request payload
- `validate:"optional,association"` - not required and should not be present in the request payload. If found in the
request payload it will cause HTTP error 400 (see `Attempt to update an associated field check`)

```go
// Account is a unique identity on the network that can hold tokens.
type Account struct {
	gorm.Model `swaggerignore:"true"`

	// Required.  The address to which and from which tokens are sent.
	Address *string `json:"Address,omitempty" query:"address" gorm:"unique;not null" validate:"required"`

	// Required.  The network the account belongs to.
	NetworkID *uint `json:"NetworkID,omitempty" query:"networkid" gorm:"not null" validate:"required"`

	// Optional.  The network node the account is associated with.
	NodeID *uint `json:"NodeID,omitempty" query:"nodeid" validate:"optional"`

	// Optional.  The pool the account is associated with.
	PoolID *uint `json:"PoolID,omitempty" query:"poolid" validate:"optional"`

	// Optional.  The transactions which sent tokens to this account.
	ToTransactions []*Transaction `json:"ToTransactions,omitempty" gorm:"foreignKey:ToAccountID" validate:"optional,association"`

	// Optional.  The transactions which sent tokens from this account.
	FromTransactions []*Transaction `json:"FromTransactions,omitempty" gorm:"foreignKey:FromAccountID" validate:"optional,association"`
}
```

If required fields are missing in a payload submitted via POST/PUT/PATCH
```json
{
  "PoolID":1,
  "NodeID":10000
}
```
REST API resource will return HTTP error `400` with message
`Missing required field(s) : ` and comma de delimited list of missing required fields.
```json
{
  "Meta": {
    "Page": 0,
    "Size": 0,
    "TotalCount": 0
  },
  "Type": "Account",
  "Data": null,
  "Status": {
    "code": 400,
    "message": "Bad Request",
    "error": "Missing required field(s) : Address,NetworkID"
  }
}
```

##### Attempt to update an associated field check

If associated fields are encountered in a payload (see `validate:"optional,association"` tag) submitted via POST/PUT/PATCH
```json
{
  "Address":"test-address",
  "NetworkID":1,
  "PoolID":1,
  "NodeID":10000,
  "ToTransactions" : [],
  "FromTransactions" : []
}
```
REST API resource will return HTTP error `400` with message
`Update of associated objects is not allowed. Use PUT for each associated object : ` and comma de delimited list of encountered associated fields.
```json
{
  "Meta": {
    "Page": 0,
    "Size": 0,
    "TotalCount": 0
  },
  "Type": "Account",
  "Data": null,
  "Status": {
    "code": 400,
    "message": "Bad Request",
    "error": "Update of associated objects is not allowed. Use PUT for each associated object : FromTransactions,ToTransactions"
  }
}
```

### Response data format

All REST API objects produce the same `JSON` Response format. The difference will be the content of the
`Data` field which represents an array of the specific object type.

Response object can be described with `JSON` schema as follows: 
```javascript
ResponseSchema =  {
  "type": "object",
  "properties": {
    "Meta": {
        metaSchema
    },
    "Type": {
        "type": "string"
    },
    "Data": {
        coincoverOrderSchemaArray
    }
    ,
    "Status": {
        statusSchema
    }
    },
    "required": [
        "Meta",
        "Type",
        "Data",
        "Status"
    ]
};
```
and this is an example of actual `JSON` Response object produced by Response `JSON` schema
```json
{
    "Meta": {
        "Page": 1,
        "Size": 1,
        "TotalCount": 1
    },
    "Type": "CoincoverOrder",
    "Data": [
        {
            "ID": 1,
            "CreatedAt": "2022-10-25T02:07:47.944117Z",
            "UpdatedAt": "2022-10-25T02:07:47.944117Z",
            "DeletedAt": null,
            "LevelUSD": 30000,
            "Active": true,
            "Start": "2022-05-03T00:00:00Z",
            "Signature": "testSig",
            "PublicKey": "testPK",
            "NodeID": 1,
            "CoincoverOrderID": "101"
        }
    ],
    "Status": {
        "code": 201,
        "message": "Created",
        "error": ""
    }
}
```
Here is a complete `JSON` schema for the `CoincoverOrder` for the example above. It describes fields
types and are they required or optional. `MetaSchema` provides info about `Data` array, such as current page 
-`Page`, size of the page - `Size` and tolal number of records available for particular request.
`StatusSchema` describes returned result with error code - `code`, error code message - `message` and
error description - `error`
```javascript
metaSchema =  {
  "type": "object",
  "properties": {
    "Page": {
        "type": "integer"
    },
    "Size": {
        "type": "integer"
    },
    "TotalCount": {
        "type": "integer"
    }
    },
    "required": [
        "Page",
        "Size",
        "TotalCount"
    ]
};

statusSchema =  {
  "type": "object",
  "properties": {
    "code": {
        "type": "integer"
    },
    "message": {
        "type":  [ "string", "null" ]
    },
    "error": {
        "type": [ "string", "null" ]
    }
    },
    "required": [
        "code",
        "message",
        "error"
    ]
};

coincoverOrderSchema =  {
  "type": "object",
    "properties": {
        "ID": {
            "type": "integer"
        },
        "CreatedAt": {
            "type": "string"
        },
        "UpdatedAt": {
            "type": "string"
        },
        "DeletedAt": {
            "type": [ "string", "null" ]
        },
        "LevelUSD": {
            "type": "integer"
        },
        "Active": {
            "type": "boolean"
        },
        "Start": {
            "type": "string"
        },
        "End": {
            "type": "string"
        },
        "Signature": {
            "type": "string"
        },
        "PublicKey": {
            "type": "string"
        },
        "CoincoverOrderID": {
            "type": "string"
        }  
    },
    "required": [
        "ID",
        "CreatedAt",
        "UpdatedAt",
        "DeletedAt",
        "LevelUSD",
        "Active",
        "Start",
        "Signature",
        "PublicKey",
        "NodeID"
    ]
};

coincoverOrderSchemaArray = {
  "type": "array",
  "items": [
      coincoverOrderSchema
  ]
};

coincoverOrderResponseSchema =  {
  "type": "object",
  "properties": {
    "Meta": {
        metaSchema
    },
    "Type": {
        "type": "string"
    },
    "Data": {
        coincoverOrderSchemaArray
    }
    ,
    "Status": {
        statusSchema
    }
    },
    "required": [
        "Meta",
        "Type",
        "Data",
        "Status"
    ]
};
```

### Response Statuses

- 200 - Success
- 201 - Created
- 400 - Bad Request
- 404 - Not found
- 500 - Internal Server Error

