package v0

import (
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"

	"github.com/labstack/echo/v4"
)

// JSONSerializer overrides echo's default JSON serializer so responses omit
// absent fields instead of emitting JSON null for them.
//
// The api types used to carry a per-field json:",omitempty" tag, which
// governed both directions: request bodies built by the client library and
// response bodies written here. Dropping the tag moved the request side to the
// OmitZeroStructFields option in util.MarshalObject. Without the same option
// here, echo's default serializer would spell out every nil pointer as null —
// on a typical object that is most of the fields, roughly ten times the bytes,
// and a wire change for any consumer that distinguishes an absent key from a
// null one. This keeps responses byte-identical to what the struct tag
// produced, and gives the api server a single marshal policy for both
// directions.
type JSONSerializer struct {
	// deserialization is unchanged: OmitZeroStructFields is a marshal option,
	// and echo's default reader already maps malformed bodies to 400.
	reader echo.DefaultJSONSerializer
}

// NewJSONSerializer returns the serializer to assign to echo's JSONSerializer
// field.
func NewJSONSerializer() *JSONSerializer {
	return &JSONSerializer{}
}

// Serialize writes i to the response as JSON, omitting zero-valued struct
// fields. A non-empty indent produces a pretty document, as echo's JSONPretty
// expects.
func (s *JSONSerializer) Serialize(c echo.Context, i interface{}, indent string) error {
	opts := []jsonv2.Options{jsonv2.OmitZeroStructFields(true)}
	if indent != "" {
		opts = append(opts, jsontext.WithIndent(indent))
	}

	if err := jsonv2.MarshalWrite(c.Response(), i, opts...); err != nil {
		return err
	}

	// echo's default serializer writes through json.Encoder, which terminates
	// every document with a newline. MarshalWrite does not, so put it back
	// rather than change what the response body looks like on the wire.
	_, err := c.Response().Write([]byte("\n"))

	return err
}

// Deserialize reads a JSON request body into i, delegating to echo's default
// so malformed bodies keep answering 400 with the same message.
func (s *JSONSerializer) Deserialize(c echo.Context, i interface{}) error {
	return s.reader.Deserialize(c, i)
}
