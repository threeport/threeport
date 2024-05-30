package v0

import (
	"errors"
	"reflect"
	"regexp"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v4"
)

const (
	REQUIRED             = "required"
	OPTIONAL             = "optional"
	OPTIONAL_ASSOCIATION = "optional,association"
)

type CustomValidator struct {
	Validator *validator.Validate
}

func (cv *CustomValidator) Validate(i interface{}) error {
	return cv.Validator.Struct(i)
}

/*
IsISO8601Date function to check parameter pattern for valid ISO8601 Date
*/
func IsISO8601Date(fl validator.FieldLevel) bool {
	ISO8601DateRegexString := "^(-?(?:[1-9][0-9]*)?[0-9]{4})-(1[0-2]|0[1-9])-(3[01]|0[1-9]|[12][0-9])(?:T|\\s)(2[0-3]|[01][0-9]):([0-5][0-9]):([0-5][0-9])?(Z)?$"
	ISO8601DateRegex := regexp.MustCompile(ISO8601DateRegexString)
	return ISO8601DateRegex.MatchString(fl.Field().String())
}

func IsOptional(fl validator.FieldLevel) bool {
	return true
}

func IsAssociation(fl validator.FieldLevel) bool {
	return true
}
func IsSliceOrArray(v interface{}) bool {
	return (reflect.TypeOf(v).Kind() == reflect.Array) || (reflect.TypeOf(v).Kind() == reflect.Slice)
}

func ValidateObj(c echo.Context, obj interface{}, missingRequiredFields *[]string) (int, error) {
	if err := c.Validate(obj); err != nil {
		for _, err := range err.(validator.ValidationErrors) {
			switch err.Tag() {
			case REQUIRED:
				*missingRequiredFields = append(*missingRequiredFields, err.Field())
			}
		}
	}

	return 500, nil
}

func ValidateBoundData(c echo.Context, obj interface{}, objectType string) (int, error) {
	var missingRequiredFields []string

	// validate a slice or an array
	if IsSliceOrArray(obj) {
		v := reflect.ValueOf(obj)
		if v.Kind() == reflect.Ptr {
			v = v.Elem()
		}
		for i := 0; i < v.Len(); i++ {
			val := v.Index(i).Interface()
			if id, err := ValidateObj(c, val, &missingRequiredFields); err != nil {
				return id, errors.New(err.Error())
			}
		}
	} else { // validate a single object
		if id, err := ValidateObj(c, obj, &missingRequiredFields); err != nil {
			return id, errors.New(err.Error())
		}
	}

	if len(missingRequiredFields) > 0 {
		return 400, errors.New(ErrMsgMissingRequiredFields + " : " + strings.Join(missingRequiredFields, ","))
	}

	return 500, nil
}
