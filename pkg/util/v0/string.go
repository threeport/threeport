package v0

import (
	"encoding/base64"
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"time"
)

const (
	alphaCharset        = "abcdefghijklmnopqrstuvwxyz"
	alphaNumericCharset = "abcdefghijklmnopqrstuvwxyz0123456789"
)

// Given a map of key value pairs, creates a formatted http query string
func CreateQueryStringFromMap(queryMap map[string]string) string {
	queryString := ""
	for k, v := range queryMap {
		seperator := ""
		if queryString != "" {
			seperator = "&"
		}

		queryString += seperator + fmt.Sprintf("%s=%s", k, v)
	}

	return queryString
}

// StringSliceContains returns true if a slice contains a certain string.
func StringSliceContains(sl []string, name string, caseSensitive bool) bool {
	for _, value := range sl {
		switch caseSensitive {
		case true:
			if value == name {
				return true
			}
		case false:
			if strings.EqualFold(value, name) {
				return true
			}
		}
	}
	return false
}

// RandomAlphaString returns a random string with the provided length
// using alphabetic charcaters.
func RandomAlphaString(length int) string {
	seededRand := rand.New(rand.NewSource(time.Now().UnixNano()))

	bytes := make([]byte, length)
	for i := range bytes {
		bytes[i] = alphaCharset[seededRand.Intn(len(alphaCharset))]
	}

	return string(bytes)
}

// RandomAlphaNumericString returns a random string with the provided length
// using alpha-numeric charcaters.
func RandomAlphaNumericString(length int) string {
	seededRand := rand.New(rand.NewSource(time.Now().UnixNano()))

	bytes := make([]byte, length)
	for i := range bytes {
		bytes[i] = alphaNumericCharset[seededRand.Intn(len(alphaNumericCharset))]
	}

	return string(bytes)
}

// Base64Encode base64 encodes any string.
func Base64Encode(str string) string {
	return base64.StdEncoding.EncodeToString([]byte(str))
}

// Base64Decode base64 decodes any string.
func Base64Decode(str string) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(str)
	if err != nil {
		return "", err

	}
	return string(decoded), nil
}

// StringListContains returns true if a string is in a list of strings.
func StringListContains(value string, input []string) bool {
	for _, i := range input {
		if i == value {
			return true
		}
	}
	return false
}

// StringToInterfaceList converts a string slice to an interface slice.
func StringToInterfaceList(input []string) []interface{} {
	output := make([]interface{}, len(input))
	for i, v := range input {
		output[i] = v
	}
	return output
}

// HyphenDelimitedString takes a slice of strings and returns a hyphen delimited string.
func HyphenDelimitedString(input []string) string {
	output := ""
	for _, v := range input {
		output += fmt.Sprintf("---\n%s", v)
	}

	return output
}

// TypeName returns the type name of the input.
func TypeName(in any) string {
	return reflect.TypeOf(in).String()
}
