package v0

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	util "github.com/threeport/threeport/pkg/util/v0"
)

const (
	QueryParamQueryId           = "queryid"
	QueryParamCursor            = "cursor"
	QueryParamLimit             = "limit"
	DefaultPaginationLimitValue = 100
	MaxPaginationLimitValue     = 1000
)

type CustomContext struct {
	echo.Context
}

// PageRequestParams contains pagination request information from the client.  These are
// sent by the client as query paramters in the request to the Threeport API.
type PageRequestParams struct {
	// QueryId is the ID of the query that produced the paginated objects.  The client receives
	// this info with the previous page of results.
	QueryId string `json:"QueryId" query:"queryid" example:"1234567890-1234567890-1234567890"`

	// Cursor is the unique ID of the first object in the next page of results.  The client
	// gets this information from the `NextCursor` field in the previous page of results.
	Cursor uint `json:"Cursor" query:"cursor" example:"1234567890"`

	// The maximum number of objects the client wishes to receive in a page of results.
	Limit int64 `json:"Limit" query:"limit" example:"500"`
}

// GetPaginationParams parses pagination query parameters into PageRequestParams.
// If the limit is not provided, the default value is used.
func (c *CustomContext) GetPaginationParams() (*PageRequestParams, error) {
	params := new(PageRequestParams)
	// extract query ID if provided
	queryId := c.Request().URL.Query().Get(QueryParamQueryId)
	if queryId != "" {
		params.QueryId = queryId
	}

	// extract cursor if provided
	strCursor := c.Request().URL.Query().Get(QueryParamCursor)
	if strCursor != "" {
		cursor, err := strconv.Atoi(strCursor)
		if err != nil {
			return params, fmt.Errorf("invalid cursor value: %s", strCursor)
		}
		params.Cursor = uint(cursor)
	}

	// extract limit if provided
	strLimit := c.Request().URL.Query().Get(QueryParamLimit)
	params.Limit = DefaultPaginationLimitValue
	if strLimit != "" {
		limit, err := strconv.Atoi(strLimit)
		if err != nil {
			return params, fmt.Errorf("invalid limit value: %s", strLimit)
		}
		params.Limit = int64(limit)
	}
	if params.Limit > MaxPaginationLimitValue {
		return params, fmt.Errorf("limit value is too large: %d - maximum value is %d", params.Limit, MaxPaginationLimitValue)
	}

	return params, nil
}

// PayloadCheck parses JSON request body into key value pairs to perform validations such as:
// - check for empty JSON object
// - check for GORM Model fields in the payload
// - check for optional associations fields in they payload if checkAssociation parameter is true
// - check for unsupported fields in the payload
// and returns an error code and error message if any of the conditions above are met
func PayloadCheck(
	c echo.Context,
	extension bool,
	checkAssociation bool,
	objectType string,
	objectStruct interface{},
) (int, error) {
	var payload map[string]interface{}
	var payloadArray []map[string]interface{}
	var providedGORMModelFields []string
	var providedAssociationsFields []string
	var unsupportedFields []string

	// extract API version from context path i.e. v0, v11 etc.
	apiVer := versionFromPath(c.Path(), extension)

	bodyBytes := readBody(c)

	// get payload k/v pairs
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		if err = json.Unmarshal(bodyBytes, &payloadArray); err != nil {
			return 500, err
		} else {
			if len(payloadArray) == 0 {
				return 400, errors.New(ErrMsgJSONPayloadEmpty)
			}
			// check array/slice of payload objects
			for _, v := range payloadArray {
				if id, err := checkPayloadObject(
					apiVer, v, objectType, objectStruct,
					&providedGORMModelFields,
					&providedAssociationsFields,
					&unsupportedFields,
				); err != nil {
					return id, err
				}
			}
		}
	} else {
		if len(payload) == 0 {
			return 400, errors.New(ErrMsgJSONPayloadEmpty)
		}
		// check single payload object
		if id, err := checkPayloadObject(
			apiVer, payload, objectType, objectStruct,
			&providedGORMModelFields,
			&providedAssociationsFields,
			&unsupportedFields,
		); err != nil {
			return id, err
		}
	}

	if len(providedGORMModelFields) > 0 {
		return 400, errors.New(ErrMsgGORMModelFieldsUpdateNotAllowed + " : " + strings.Join(providedGORMModelFields, ","))
	}

	if checkAssociation {
		if len(providedAssociationsFields) > 0 {
			return 400, errors.New(ErrMsgAssociationsUpdateNotAllowed + " : " + strings.Join(providedAssociationsFields, ","))
		}
	}

	if len(unsupportedFields) > 0 {
		return 400, errors.New(ErrMsgUnsupportedFieldsNotAllowed + " : " + strings.Join(unsupportedFields, ","))
	}

	return 500, nil
}

// checkPayloadObject analyzes payload using Object model tags and returns providedGORMModelFields,
// providedAssociationsFields, unsupportedFields for further decision making
func checkPayloadObject(apiVer string, payloadObject map[string]interface{}, objectType string, objectStruct interface{}, providedGORMModelFields *[]string, providedAssociationsFields *[]string, unsupportedFields *[]string) (int, error) {
	var associatedFields = &[]string{}
	var optionalFields = &[]string{}
	var optionalAssociationsFields = &[]string{}
	var requiredFields = &[]string{}

	// error out if a GORM Model field was passed for an update
	for k, _ := range payloadObject {
		if util.StringSliceContains(GORMModelFields, k, false) {
			*providedGORMModelFields = append(*providedGORMModelFields, k)
		}
	}

	// to be able to do this, needed to introduce "validate" tag parsing into
	// ObjectTaggedFields
	if fieldsByTag, ok := ObjectTaggedFields[VersionObject{Version: apiVer, Object: objectType}]; ok {
		associatedFields = &fieldsByTag.OptionalAssociations
	} else {
		return 500, errors.New("ObjectTaggedFields for " + apiVer + " and " + objectType + " not found")
	}

	// error out if an association was passed for an update
	for k, _ := range payloadObject {
		if util.StringSliceContains(*associatedFields, k, false) {
			*providedAssociationsFields = append(*providedAssociationsFields, k)
		}
	}

	// field not Optional, OptionalAssociation or Required - it's Unsupported
	optionalFields = &ObjectTaggedFields[VersionObject{Version: apiVer, Object: string(objectType)}].Optional
	optionalAssociationsFields = &ObjectTaggedFields[VersionObject{Version: apiVer, Object: string(objectType)}].OptionalAssociations
	requiredFields = &ObjectTaggedFields[VersionObject{Version: apiVer, Object: string(objectType)}].Required
	for k, _ := range payloadObject {
		// check the field k form the payload
		if !util.StringSliceContains(*optionalFields, k, false) &&
			!util.StringSliceContains(*optionalAssociationsFields, k, false) &&
			!util.StringSliceContains(*requiredFields, k, false) {
			// now we need to check the same for the alias of the k
			kAlias := getFieldNameByJsonTag(k, "json", objectStruct)
			if len(kAlias) > 0 {
				if !util.StringSliceContains(*optionalFields, kAlias, false) &&
					!util.StringSliceContains(*optionalAssociationsFields, kAlias, false) &&
					!util.StringSliceContains(*requiredFields, kAlias, false) {
					*unsupportedFields = append(*unsupportedFields, kAlias)
				}
			} else {
				*unsupportedFields = append(*unsupportedFields, k)
			}
		}
	}

	return 200, nil
}

// versionFromPath returns the API version from the REST path based on whether
// the path is from core Threeport or an extension of threeport.
func versionFromPath(path string, extension bool) string {
	parsedPath := strings.Split(path, "/")
	if extension {
		return parsedPath[2]
	}
	return parsedPath[1]
}

// readBody reads the request's body and returns it as a byte slice.
func readBody(c echo.Context) []byte {
	defer c.Request().Body.Close()
	bodyBytes, _ := io.ReadAll(c.Request().Body)
	c.Request().Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	return bodyBytes
}

// getFieldNameByJsonTag returns the field name of the struct by the given tag and key.
func getFieldNameByJsonTag(tag, key string, s interface{}) (fieldname string) {
	rt := reflect.TypeOf(s)
	if rt.Kind() != reflect.Struct {
		panic("bad type")
	}
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		v := strings.Split(f.Tag.Get(key), ",")[0] // use split to ignore tag "options"
		if v == tag {
			return f.Name
		}
	}
	return ""
}
