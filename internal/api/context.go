package api

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

	//"github.com/threeport/threeport/internal/authority"
	"github.com/threeport/threeport/internal/api/util"
	v0 "github.com/threeport/threeport/pkg/api/v0"
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
func (c *CustomContext) GetPaginationParams() (params v0.PageRequestParams, err error) {

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
func CheckPayloadObject(apiVer string, payloadObject map[string]interface{}, objectType v0.ObjectType, providedGORMModelFields *[]string, providedAssociationsFields *[]string, unsupportedFields *[]string) (int, error) {
	var associatedFields = &[]string{}
	var optionalFields = &[]string{}
	var optionalAssociationsFields = &[]string{}
	var requiredFields = &[]string{}

	// error out if a GORM Model field was passed for an update
	for k, _ := range payloadObject {
		if util.Contains(v0.GORMModelFields, k, false) {
			*providedGORMModelFields = append(*providedGORMModelFields, k)
		}
	}

	// to be able to do this, needed to introduce "validate" tag parsing into ObjectTaggedFields
	associatedFields = &ObjectTaggedFields[VersionObject{Version: apiVer, Object: string(objectType)}].OptionalAssociations

	// error out if an association was passed for an update
	for k, _ := range payloadObject {
		if util.Contains(*associatedFields, k, false) {
			*providedAssociationsFields = append(*providedAssociationsFields, k)
		}
	}

	// field not Optional, OptionalAssociation or Required - it's Unsupported

	optionalFields = &ObjectTaggedFields[VersionObject{Version: apiVer, Object: string(objectType)}].Optional
	optionalAssociationsFields = &ObjectTaggedFields[VersionObject{Version: apiVer, Object: string(objectType)}].OptionalAssociations
	requiredFields = &ObjectTaggedFields[VersionObject{Version: apiVer, Object: string(objectType)}].Required
	// in the payload k can be supplied an JSON alias i.e. DefinitionJSON instead of JSONDefinition
	// type WorkloadResourceDefinition struct {
	//	 gorm.Model `swaggerignore:"true" mapstructure:",squash"`
	//	 JSONDefinition *datatypes.JSON `json:"DefinitionJSON,omitempty" gorm:"not null" validate:"required"`
	//	 WorkloadDefinitionID *uint `json:"WorkloadDefinitionID,omitempty" query:"workloaddefinitionid" gorm:"not null" validate:"required"`
	// }
	// {
	//	 "DefinitionJSON": "{\"foo\": \"bar\"}",
	//	 "WorkloadDefinitionID": 1
	// }
	for k, _ := range payloadObject {
		// check the field k form the payload
		if !util.Contains(*optionalFields, k, false) &&
			!util.Contains(*optionalAssociationsFields, k, false) &&
			!util.Contains(*requiredFields, k, false) {
			// now we need to check the same for the alias of the k
			kAlias := getFieldNameByJsonTag(k, "json", v0.GetStructByObjectType(objectType))
			if len(kAlias) > 0 {
				if !util.Contains(*optionalFields, kAlias, false) &&
					!util.Contains(*optionalAssociationsFields, kAlias, false) &&
					!util.Contains(*requiredFields, kAlias, false) {
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
func PayloadCheck(c echo.Context, checkAssociation bool, objectType v0.ObjectType) (int, error) {
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
				return 400, errors.New(v0.ErrMsgJSONPayloadEmpty)
			}
			// check array/slice of payload objects
			for _, v := range payloadArray {
				if id, err := CheckPayloadObject(apiVer, v, objectType, &providedGORMModelFields, &providedAssociationsFields, &unsupportedFields); err != nil {
					return id, err
				}
			}
		}
	} else {
		if len(payload) == 0 {
			return 400, errors.New(v0.ErrMsgJSONPayloadEmpty)
		}
		// check single payload object
		if id, err := CheckPayloadObject(apiVer, payload, objectType, &providedGORMModelFields, &providedAssociationsFields, &unsupportedFields); err != nil {
			return id, err
		}
	}

	if len(providedGORMModelFields) > 0 {
		return 400, errors.New(v0.ErrMsgGORMModelFieldsUpdateNotAllowed + " : " + strings.Join(providedGORMModelFields, ","))
	}

	if checkAssociation {
		if len(providedAssociationsFields) > 0 {
			return 400, errors.New(v0.ErrMsgAssociationsUpdateNotAllowed + " : " + strings.Join(providedAssociationsFields, ","))
		}
	}

	if len(unsupportedFields) > 0 {
		return 400, errors.New(v0.ErrMsgUnsupportedFieldsNotAllowed + " : " + strings.Join(unsupportedFields, ","))
	}

	return 500, nil
}

//// AuthorizationTokenCheck middleware that checks if user presented OAuth token has permissions to
//// call REST API method
//func AuthorizationTokenCheck(next echo.HandlerFunc) echo.HandlerFunc {
//	return func(c echo.Context) error {
//		// authentication
//		reqToken := c.Request().Header.Get(AuthorizationKey)
//		splitToken := strings.Split(reqToken, "Bearer ")
//
//		// construct ObjectType from Request Path
//		objectType := v0.GetObjectTypeByPath(c.Request().URL.Path)
//
//		if len(strings.TrimSpace(reqToken)) == 0 || len(splitToken) == 1 {
//			return ResponseStatus400(c, nil, errors.New(ErrTokenIsNotProvided), objectType)
//		} else {
//			authenticate.Token = splitToken[1]
//		}
//
//		// create Authentik client configuration and client itself
//		akadminConfig := authenticate.CreateConfiguration(os.Getenv(authenticate.AuthentikScheme), os.Getenv(authenticate.AuthentikHost), authenticate.Token)
//		akadminApiClient := authenticate.NewAPIClient(akadminConfig)
//
//		// retrieve Groups user belongs to
//		userGroups, err := authenticate.MeRetrieveUserGroups(context.Background(), akadminApiClient)
//		if err != nil {
//			return ResponseStatus400(c, nil, errors.New("Cannot retrieve user info"), objectType)
//		}
//		if len(userGroups) == 0 {
//			return ResponseStatus400(c, nil, errors.New("User has no permission to call this API [User doesn't belong to any Authentik group]"), objectType)
//		}
//
//		// authorization, check if user has permission to call this API function
//		hasPermission, err := authority.Auth.CheckGroupsPermission(userGroups, objectType, c.Request().Method)
//		if err != nil {
//			return ResponseStatus400(c, nil, errors.New(err.Error()), objectType)
//		}
//		if !hasPermission {
//			return ResponseStatus400(c, nil, errors.New("User has no permission to call this API"), objectType)
//		}
//
//		// Continue default operation of Echo
//		return next(c)
//	}
//}

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
