package v0

import (
	"errors"
	"net/http"
	"reflect"
)

const (
	ErrMsgJSONPayloadEmpty                = "JSON payload is empty"
	ErrMsgMissingRequiredFields           = "Missing required field(s)"
	ErrMsgRequiredFieldsCannotBeNull      = "Required field(s) cannot be null"
	ErrMsgAssociationsUpdateNotAllowed    = "Update of associated objects is not allowed. Use PUT for each associated object"
	ErrMsgGORMModelFieldsUpdateNotAllowed = "Update of GORM Model fields is not allowed"
	ErrMsgUnsupportedFieldsNotAllowed     = "Unsupported fields are not allowed"
)

var GORMModelFields = []string{"ID", "CreatedAt", "UpdatedAt", "DeletedAt"}

// ObjectType model info.
type ObjectType string

// Object model info.
type Object interface{}

// Response is the response to a request. It contains the requested data,
// meta information, and the status of the request.
type Response struct {
	// Meta contains PageRequestParams (current page and size of current page) and TotalCount (number of returned Object elements)
	Meta Meta `json:"Meta"`

	// Type contains ObjectType of returned Data elements.
	Type string `json:"Type" example:"KubernetesWorkloadInstance"`

	// Data contains array of returned Object elements.
	Data []Object `json:"Data"`

	// Status represents an error that occurred while handling a request.
	Status Status `json:"Status"`
}

// Meta model info
type Meta struct {
	// Pagination contains the pagination information for a paginated response.
	Pagination Pagination `json:"Pagination"`

	// The number of objects returned in the response.
	ObjectCount int64 `json:"ObjectCount" example:"1"`
}

// Pagination contains the pagination information for a paginated response.
type Pagination struct {
	// Limit is the maximum number of objects returned.  This is either set in the client's
	// request or set to a default value by the server.  In any case, this field informs the
	// client of the maximum number of objects they can expect to receive.
	Limit int64 `json:"Limit" query:"limit" example:"1"`

	// NextCursor is the ID of the last object in the previous page of results.
	NextCursor uint `json:"NextCursor" query:"nextcursor" example:"1234567890"`

	// QueryId is the ID of the query that produced the paginated objects.  This must be
	// referenced by the client to fetch subsequent pages of results.
	QueryId string `json:"QueryId" query:"queryid" example:"1234567890-1234567890-1234567890"`

	// HasMore is a boolean indicating if there are more objects to fetch after the current page.
	HasMore bool `json:"HasMore" query:"hasmore" example:"true"`
}

// Status represents the response HTTP status including error messages if
// applicable.
type Status struct {
	// The HTTP response status code, e.g. 200 | 201 | 500
	Code int `json:"code" example:"200"`

	// The HTTP response status code message, e.g. OK | Created | Internal Server Error
	Message string `json:"message" example:"OK"`

	// The response error message if applicable, defaults to ""
	Error string `json:"error" example:""`
}

// CreateResponse creates a Response object with the given meta, object, and object type.
func CreateResponse(
	meta *Meta,
	obj interface{},
	objType string,
) (*Response, error) {

	if meta == nil {
		return nil, errors.New("response metadata must not be nil")
	}

	if obj == nil {
		return nil, errors.New("response object must not be nil")
	}

	var code = http.StatusOK
	var message = http.StatusText(code)

	response := new(Response)
	response.Type = objType

	if reflect.TypeOf(obj).Kind() == reflect.Slice {
		s := reflect.ValueOf(obj)
		response.Data = make([]Object, s.Len())
		for i := 0; i < s.Len(); i++ {
			val := s.Index(i).Interface()
			response.Data[i] = val
		}
	} else {
		response.Data = make([]Object, 0)
		response.Data = append(response.Data, obj)
	}

	response.Meta = *meta
	response.Status = Status{Code: code, Message: message, Error: ""}

	return response, nil
}

// SingleObjectMeta creates a Meta object with an ObjectCount of 1 and
// no pagination information.  This is used in the common use case when
// returning a single object.
func SingleObjectMeta() *Meta {
	return &Meta{
		ObjectCount: 1,
	}
}

// UpdateResponseStatus updates the status of a response.
func UpdateResponseStatus(response *Response, code int, message string, error string) {
	response.Status.Code = code
	response.Status.Message = message
	response.Status.Error = error
}

// CreateStatus creates a Status object with the given code, message, and error.
func CreateStatus(code int, message string, error string) *Status {
	return &Status{Code: code, Message: message, Error: error}
}

// CreateResponseErrorWithStatus creates a Response object with the given status, object type.
func CreateResponseErrorWithStatus(params *PageRequestParams, status *Status, objectType string) *Response {
	return &Response{
		Meta: Meta{
			Pagination: Pagination{
				Limit:      0,
				NextCursor: 0,
				QueryId:    "",
			}, ObjectCount: 0,
		},
		Type: objectType,
		Data: nil,
		Status: Status{
			Code:    status.Code,
			Message: status.Message,
			Error:   status.Error,
		},
	}
}

// CreateResponseWithError400 creates a Response object with the a 400 status, given params, error, and object type.
func CreateResponseWithError400(params *PageRequestParams, error error, objectType string) *Response {
	return CreateResponseErrorWithStatus(params, CreateStatus(http.StatusBadRequest, http.StatusText(http.StatusBadRequest), error.Error()), objectType)
}

// CreateResponseWithError401 creates a Response object with the a 401 status, given params, error, and object type.
func CreateResponseWithError401(params *PageRequestParams, error error, objectType string) *Response {
	return CreateResponseErrorWithStatus(params, CreateStatus(http.StatusUnauthorized, http.StatusText(http.StatusUnauthorized), error.Error()), objectType)
}

// CreateResponseWithError403 creates a Response object with the a 403 status, given params, error, and object type.
func CreateResponseWithError403(params *PageRequestParams, error error, objectType string) *Response {
	return CreateResponseErrorWithStatus(params, CreateStatus(http.StatusForbidden, http.StatusText(http.StatusForbidden), error.Error()), objectType)
}

// CreateResponseWithError404 creates a Response object with the a 404 status, given params, error, and object type.
func CreateResponseWithError404(params *PageRequestParams, error error, objectType string) *Response {
	return CreateResponseErrorWithStatus(params, CreateStatus(http.StatusNotFound, http.StatusText(http.StatusNotFound), error.Error()), objectType)
}

// CreateResponseWithError409 creates a Response object with the a 409 status, given params, error, and object type.
func CreateResponseWithError409(params *PageRequestParams, error error, objectType string) *Response {
	return CreateResponseErrorWithStatus(params, CreateStatus(http.StatusConflict, http.StatusText(http.StatusConflict), error.Error()), objectType)
}

// CreateResponseWithError500 creates a Response object with the a 500 status, given params, error, and object type.
func CreateResponseWithError500(params *PageRequestParams, error error, objectType string) *Response {
	return CreateResponseErrorWithStatus(params, CreateStatus(http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError), error.Error()), objectType)
}
