# API Object Struct Tags

Threeport API objects are Go structs whose fields carry tags that drive
wire serialization, validation, database persistence, encryption, and
swagger documentation. This page is the canonical reference for every
tag key, when each one applies, the suggested ordering, and the codegen
behavior that enforces the conventions.

The same tags apply to Threeport core types (in `pkg/api/v0/` of the
upstream repo) and to module types (scaffolded by `threeport-sdk
create` into a module's own `pkg/api/v0/` directory). The SDK and the
api server treat both identically; nothing on this page is core-only or
module-only.

## Reference

### `json`

API object fields carry no `json` tag. `encoding/json` already defaults
to the Go field name, so the field-name part was always redundant, and
the omission of absent values is now a marshal-time policy rather than a
per-field tag.

`util.MarshalObject()` in `pkg/util/v0/serialize.go` — the single
chokepoint every request body to the api server passes through — marshals
with `encoding/json/v2` and the `OmitZeroStructFields` option. Every
zero-valued field is dropped, which for the pointer fields on an API
object means every nil pointer. That is what keeps a partial PATCH body
from carrying a JSON `null` for a field the caller never meant to touch,
which the api server's `PayloadCheck()` null-on-required guard would
reject.

```go
Name *string `validate:"required"`
```

Two consequences differ from the old tag, because
`omitzero` and `omitempty` do not omit the same set of values:

- A `*string` pointing at `""` is now sent as `""`. The tag used to drop
  it, which silently discarded a caller's attempt to clear a string
  field.
- An initialized-but-empty association slice is now sent as `[]`. The
  tag used to drop it. A nil slice is still omitted.

A new API object field needs no `json` tag. Adding one back is redundant
at best, and in particular reintroduces the
empty-string omission this policy deliberately removed.

### `validate`

Request-body validation, enforced by the api server's `PayloadCheck`.
Values:

- `required`: must be present and non-null on Create and Update requests.
- `optional`: may be absent or null.
- `optional,association`: many-to-one or many-to-many association field
  (a slice of pointers to a related type), optional on the wire.

```go
Hostname *string `validate:"required" gorm:"not null"`
SSHKey   *string `validate:"optional" encrypt:"true"`
```

### `gorm`

[GORM](https://gorm.io) ORM directives for the database schema. Common
values:

- `not null`: column-level not-null constraint. Pairs with
  `validate:"required"` for symmetric API and DB enforcement.
- `default:<value>`: column default (e.g. `default:false`,
  `default:'describes'`).
- `uniqueIndex:idx_<name>`: unique index. Multiple columns can share an
  index name to form a composite unique index.
- `type:jsonb;serializer:json`: store a Go slice or struct as PostgreSQL
  JSONB with JSON marshaling on read/write.
- `primarykey`: primary key. Only used on `Common.ID`.

```go
ID *uint `gorm:"primarykey"`
KubernetesWorkloadDefinitionID *uint `validate:"required" gorm:"not null" relationship:"requires"`
```

### `encrypt`

Server-side field encryption. `encrypt:"true"` directs the api server's
encrypt hooks to AES-GCM encrypt the field value before writing to the
database, and decrypt on read when an encryption key is supplied. Use on
any field containing secrets (SSH keys, passwords, API credentials).

```go
SSHPassword *string `validate:"optional" encrypt:"true"`
```

### `relationship`

[Attached Object Reference](../concepts/attached-object-reference.md)
(AOR) modeling. The value drives lifecycle behavior on the base object.
Values:

- `describes`: informational; does not block delete or update of the
  base. The default.
- `requires`: blocks any caller from deleting the base while this
  reference exists.
- `owns`: blocks both delete and update of the base for any caller
  except the controller registered for the attached object's type. An
  owned base has at most one owner; an owner may own many bases.
- `marries`: enforces 1-to-1 cardinality between base and attacher.
  Blocks both delete and update of the base for any caller except the
  partner's controller.

A `type:<TargetType>` modifier may be appended after a semicolon when
the target type can't be inferred from the field name (cross-type FK
fields).

```go
AwsProviderID *uint `validate:"required" gorm:"not null" relationship:"requires"`
ParentControlPlaneInstanceID *uint `validate:"optional" relationship:"requires;type:ControlPlaneInstance"`
HelmWorkloadDefinitionID *uint `validate:"optional" relationship:"owns;type:HelmWorkloadDefinition"`
```

### `persist`

`persist:"false"` excludes the field from the database. The api server
nulls the column before insert and the generated reconciler skips its
pre-reconcile reload. Only `"false"` is allowed; codegen rejects any
other value. Used for fields whose value must travel through the
notification payload only (`Secret.Data` is the current consumer).

```go
Data *datatypes.JSON `validate:"required" persist:"false"`
```

### `swaggerignore`

`swaggerignore:"true"` hides the field from generated swagger docs. Used
on every embedded composite field (`Common`, `Definition`, `Instance`,
`Reconciliation`) so the swagger output shows only the type-specific
fields, not the framework scaffolding.

```go
tpapi_v0.Common `mapstructure:",squash" swaggerignore:"true"`
```

### `mapstructure`

`mapstructure:",squash"` flattens an embedded struct's fields into the
parent during `mapstructure`-based decoding (yaml config files into Go
structs). Used on every embedded composite field.

```go
tpapi_v0.Definition `mapstructure:",squash"`
```

### `example`

`example:"<value>"` provides a swagger example value for API
documentation. Optional, but useful for non-obvious string formats.

```go
ObjectCount int64 `json:"ObjectCount" example:"1"`
```

## Forbidden tags

### `query`

Don't use. The api server's query binder derives query parameter keys
from the lowercase Go field name automatically. An explicit `query:`
tag is redundant noise at best and a silent rename hazard at worst.
Codegen rejects any field carrying one.

### `yaml`

Don't use. The codebase uses `sigs.k8s.io/yaml`, which converts YAML to
JSON internally and reads `json:` tags. `yaml:` tags are silently
ignored.

## Conventions

### Tag order

```
validate -> gorm -> encrypt -> relationship -> persist
```

Rationale: `validate` states the API contract for the field, so it leads.
`gorm` follows because the DB constraint usually mirrors that contract
(`validate:"required"` with `gorm:"not null"`), which makes a mismatch
between the two easy to spot at a glance.

### Pairing rules

- `validate:"required"` typically pairs with `gorm:"not null"` so the
  API contract and the DB schema agree on field presence.
- `encrypt:"true"` typically pairs with `validate:"optional"` since
  encrypted secrets are nullable on the wire (writing one is optional;
  the server only re-encrypts when the field is non-null on a write).

## Codegen behavior

`threeport-sdk create` emits scaffolded struct tags in convention
order, so newly-scaffolded code lands ready to read.

`threeport-sdk gen` validates tags before generating any code. An
unrecognized `validate` value, a `query` tag, or a `persist` value other
than `"false"` fails codegen with a descriptive error.

## Complete examples

### Required field with DB enforcement

```go
Name *string `validate:"required" gorm:"not null"`
```

### Required foreign key with relationship

```go
KubernetesWorkloadDefinitionID *uint `validate:"required" gorm:"not null" relationship:"requires"`
```

### Optional encrypted secret

```go
SSHKey *string `validate:"optional" encrypt:"true"`
```

### Association (one-to-many back reference)

```go
KubernetesWorkloadInstances []*KubernetesWorkloadInstance `validate:"optional,association"`
```

### Embedded composite fields

```go
tpapi_v0.Common         `mapstructure:",squash" swaggerignore:"true"`
tpapi_v0.Reconciliation `mapstructure:",squash"`
tpapi_v0.Definition     `mapstructure:",squash"`
```
