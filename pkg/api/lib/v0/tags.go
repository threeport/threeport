package v0

// TagKey names a struct-tag key recognized by the threeport API schema.
type TagKey string

const (
	RelationshipTag TagKey = "relationship"
	EncryptTag      TagKey = "encrypt"
	PersistTag      TagKey = "persist"
	ValidateTag     TagKey = "validate"
	QueryTag        TagKey = "query"
	JsonTag         TagKey = "json"
)

// JsonOmitempty is the json-tag substring every `validate:"optional"`
// field must carry so absent values are dropped on serialize rather
// than emitting JSON null / zero values.
const JsonOmitempty = "omitempty"

// RelationshipTypeKey is the modifier name in a relationship tag value
// (e.g. `relationship:"requires;type:KubernetesRuntimeInstance"`).
const RelationshipTypeKey = "type"

// Validate is the value space of the "validate" struct tag on API type fields.
type Validate string

const (
	ValidateRequired            Validate = "required"
	ValidateOptional            Validate = "optional"
	ValidateAssociation         Validate = "association"
	ValidateOptionalAssociation Validate = ValidateOptional + "," + ValidateAssociation
)

const (
	EncryptTrue  = "true"
	PersistFalse = "false"
)
