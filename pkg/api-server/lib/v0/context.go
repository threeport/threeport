package v0

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"reflect"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	util "github.com/threeport/threeport/pkg/util/v0"
)

const (
	QueryParamPage                   = "page"
	QueryParamSize                   = "size"
	DefaultParamSizeValue            = 50
	ErrMsgQueryParamInvalidPageValue = "Query parameter is not a valid integer value: " + QueryParamPage
	ErrMsgQueryParamInvalidSizeValue = "Query parameter is not a valid integer value: " + QueryParamSize
	ErrTokenIsNotProvided            = "OAuth token is not provided"
	AuthorizationKey                 = "Authorization"
	BearerKey                        = "Bearer "
)

type CustomContext struct {
	echo.Context
}

// GetBearerToken extracts the OAuth token from Authorization header.
func (c *CustomContext) GetBearerToken() (token string, err error) {

	reqToken := c.Request().Header.Get(AuthorizationKey)

	splitToken := strings.Split(reqToken, BearerKey)

	if len(strings.TrimSpace(reqToken)) == 0 || len(splitToken) == 1 {
		err = errors.New(ErrTokenIsNotProvided)
	} else {
		token = splitToken[1]
	}

	return token, err
}

func indexAt(s, substr string, n int) int {
	idx := strings.Index(s[n:], substr)
	if idx > -1 {
		idx += n
	}
	return idx
}

// GetPaginationParams parses pagination query parameters into PageRequestParams.
func (c *CustomContext) GetPaginationParams() (params PageRequestParams, err error) {

	strPage := c.Request().URL.Query().Get(QueryParamPage)
	params.Page = -1
	if strPage != "" {
		params.Page, err = strconv.Atoi(strPage)
		if err != nil || params.Page < -1 {
			return params, errors.New(ErrMsgQueryParamInvalidPageValue)
		}
	} else {
		params.Page = 1
	}

	strSize := c.Request().URL.Query().Get(QueryParamSize)
	// with a value as -1 for gorms Limit method, we'll get a request without limit as default

	params.Size = DefaultParamSizeValue
	if strSize != "" {
		params.Size, err = strconv.Atoi(strSize)
		if err != nil || params.Size < -1 {
			return params, errors.New(ErrMsgQueryParamInvalidSizeValue)
		}
	}

	return params, err
}

// readBody Read request's body is a way so it can be red again by other methods
func readBody(c echo.Context) []byte {
	defer c.Request().Body.Close()
	bodyBytes := []byte{}
	bodyBytes, _ = ioutil.ReadAll(c.Request().Body)
	c.Request().Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))
	return bodyBytes
}

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

// CheckPayloadObject analyzes payload using Object model tags and returns providedGORMModelFields,
// providedAssociationsFields, unsupportedFields for further decision making
func CheckPayloadObject(apiVer string, payloadObject map[string]interface{}, objectType string, objectStruct interface{}, providedGORMModelFields *[]string, providedAssociationsFields *[]string, unsupportedFields *[]string) (int, error) {
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

// PayloadCheck parses JSON request body into key value pairs to perform validations such as:
// - check for empty JSON object
// - check for GORM Model fields in the payload
// - check for optional associations fields in they payload if checkAssociation parameter is true
// - check for unsupported fields in the payload
// and returns an error code and error message if any of the conditions above are met
func PayloadCheck(c echo.Context, checkAssociation bool, objectType string, objectStruct interface{}) (int, error) {
	var payload map[string]interface{}
	var payloadArray []map[string]interface{}
	var providedGORMModelFields []string
	var providedAssociationsFields []string
	var unsupportedFields []string
	// extract API version from context path i.e. v0, v11 etc.
	apiVer := c.Path()[1:indexAt(c.Path(), "/", 1)]

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
				if id, err := CheckPayloadObject(apiVer, v, objectType, objectStruct, &providedGORMModelFields, &providedAssociationsFields, &unsupportedFields); err != nil {
					return id, err
				}
			}
		}
	} else {
		if len(payload) == 0 {
			return 400, errors.New(ErrMsgJSONPayloadEmpty)
		}
		// check single payload object
		if id, err := CheckPayloadObject(apiVer, payload, objectType, objectStruct, &providedGORMModelFields, &providedAssociationsFields, &unsupportedFields); err != nil {
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

// MiddlewareFunc is a potential replacement for PayloadCheck in the future
func MiddlewareFunc(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		// read origin body bytes
		var bodyBytes []byte
		if c.Request().Body != nil {
			bodyBytes, _ = ioutil.ReadAll(c.Request().Body)
			// write back to request body
			c.Request().Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))
			// parse json data
			reqData := struct {
				ID string `json:"id"`
			}{}
			err := json.Unmarshal(bodyBytes, &reqData)
			if err != nil {
				return c.JSON(400, "error json.")
			}
			fmt.Println(reqData.ID)
		}
		return next(c)
	}
}
