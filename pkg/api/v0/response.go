// +threeport-sdk route-exclude
// +threeport-sdk database-exclude
package v0

import (
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
)

// ObjectType model info
type ObjectType string

const (
	ObjectTypeUnknown = "Unknown"
	ObjectTypeBlank   = ""

	ErrMsgNoRecordsFound                  = "No records found"
	ErrMsgStatusUnauthorized              = "Unauthorized"
	ErrMsgJSONPayloadEmpty                = "JSON payload is empty"
	ErrMsgMissingRequiredFields           = "Missing required field(s)"
	ErrMsgAssociationsUpdateNotAllowed    = "Update of associated objects is not allowed. Use PUT for each associated object"
	ErrMsgGORMModelFieldsUpdateNotAllowed = "Update of GORM Model fields is not allowed"
	ErrMsgUnsupportedFieldsNotAllowed     = "Unsupported fields are not allowed"
)

var GORMModelFields = []string{"ID", "CreatedAt", "UpdatedAt", "DeletedAt"}

// Object model info.
type Object interface{}

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

// Response Paginated response model info
// @Description Meta info with ObjectType array of Data of Object
type Response struct {
	// Meta contains PageRequestParams (current page and size of current page) and TotalCount (number of returned Object elements)
	Meta Meta `json:"Meta"`

	// Type contains ObjectType of returned Data elements
	Type ObjectType `json:"Type" example:"Transfer"`

	// Data contains array of returned Object elements
	Data []Object `json:"Data"`

	// Status represents an error that occurred while handling a request
	Status Status `json:"Status"`
}

// PageRequestParams Paginated params model info
// @Description Requested Page and Size of an array to return
type PageRequestParams struct {
	// current Page
	Page int `json:"Page" query:"page" example:"1"`
	// Size of the current page (number of returned Object elements)
	Size int `json:"Size" query:"size" example:"1"`
}

// Meta model info
type Meta struct {
	PageRequestParams

	// TotalCount of returned Object elements
	TotalCount int64 `json:"TotalCount" example:"1"`
}

func CreateMeta(params PageRequestParams, totalCount int64) *Meta {
	return &Meta{PageRequestParams: PageRequestParams{Page: params.Page, Size: params.Size}, TotalCount: totalCount}
}

// CreateResponse creates an v0.Response from an Object or slice of Object of
// ObjectType (i.e []Account, []Block etc.)
func CreateResponse(meta *Meta, obj interface{}, objType ObjectType) (*Response, error) {

	if obj == nil {
		return nil, errors.New("obj must not be nil")
	}

	var code = http.StatusOK
	var message = http.StatusText(code)

	response := new(Response)
	response.Type = objType

	var page = 1
	var size = 1
	var totalCount = int64(1)

	if meta != nil {
		page = meta.Page
		size = meta.Size
		totalCount = meta.TotalCount
	}

	if reflect.TypeOf(obj).Kind() == reflect.Slice {
		if meta == nil {
			size = reflect.ValueOf(obj).Len()
			totalCount = int64(size)
		}

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

	response.Meta = Meta{PageRequestParams: PageRequestParams{Page: page, Size: size}, TotalCount: totalCount}
	response.Status = Status{Code: code, Message: message, Error: ""}

	return response, nil
}

func UpdateResponseStatus(response *Response, code int, message string, error string) {
	response.Status.Code = code
	response.Status.Message = message
	response.Status.Error = error
}
func CreateStatus(code int, message string, error string) *Status {
	return &Status{Code: code, Message: message, Error: error}
}

func CreateResponseErrorWithCode(params *PageRequestParams, code int, error string, objectType ObjectType) *Response {
	return CreateResponseErrorWithStatus(params, CreateStatus(code, http.StatusText(code), error), objectType)
}

func CreateResponseErrorWithStatus(params *PageRequestParams, status *Status, objectType ObjectType) *Response {
	return &Response{Meta: Meta{PageRequestParams: PageRequestParams{Page: 0, Size: 0}, TotalCount: 0}, Type: objectType, Data: nil, Status: Status{Code: status.Code, Message: status.Message, Error: status.Error}}
}

func CreateResponseWithError400(params *PageRequestParams, error error, objectType ObjectType) *Response {
	return CreateResponseErrorWithStatus(params, CreateStatus(http.StatusBadRequest, http.StatusText(http.StatusBadRequest), error.Error()), objectType)
}

func CreateResponseWithError401(params *PageRequestParams, error error, objectType ObjectType) *Response {
	return CreateResponseErrorWithStatus(params, CreateStatus(http.StatusUnauthorized, http.StatusText(http.StatusUnauthorized), error.Error()), objectType)
}

func CreateResponseWithError403(params *PageRequestParams, error error, objectType ObjectType) *Response {
	return CreateResponseErrorWithStatus(params, CreateStatus(http.StatusForbidden, http.StatusText(http.StatusForbidden), error.Error()), objectType)
}

func CreateResponseWithError404(params *PageRequestParams, error error, objectType ObjectType) *Response {
	return CreateResponseErrorWithStatus(params, CreateStatus(http.StatusNotFound, http.StatusText(http.StatusNotFound), error.Error()), objectType)
}

func CreateResponseWithError409(params *PageRequestParams, error error, objectType ObjectType) *Response {
	return CreateResponseErrorWithStatus(params, CreateStatus(http.StatusConflict, http.StatusText(http.StatusConflict), error.Error()), objectType)
}

func CreateResponseWithError500(params *PageRequestParams, error error, objectType ObjectType) *Response {
	return CreateResponseErrorWithStatus(params, CreateStatus(http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError), error.Error()), objectType)
}

// GetResponseData returns Response.Data from Response
func GetResponseData(data []byte) (*[]Object, error) {
	var response Response

	if err := json.Unmarshal(data, &response); err != nil {
		return nil, err
	}

	return &response.Data, nil
}
