package v0

import (
	"os"
	"reflect"
	"strconv"
	"strings"
)

const GoClientDebug = "ThreeportGoClientDebug"

func IsDebug() bool {
	v, err := strconv.ParseBool(os.Getenv(GoClientDebug))
	if err == nil && v {
		return true
	}
	return false
}

func ReplaceAssociatedObjectsWithNil(obj interface{}) (err error) {
	v := reflect.ValueOf(obj).Elem()
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		if grib, ok := t.Field(i).Tag.Lookup("validate"); ok {
			if strings.Contains(grib, "association") {
				fv := v.Field(i)
				fv.Set(reflect.Zero(fv.Type()))
			}
		}
	}
	return nil
}
