package v0

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/threeport/threeport/pkg/msg"
	yaml "gopkg.in/yaml.v2"
)

// Error returns a formatted error message in red.
func Error(message string, err error) {
	msg.Error(message, err)
}

// Info returns a formatted info message.
func Info(message string) {
	msg.Info(message)
}

// Notice returns a formatted notice message in blue.
func Notice(message string) {
	msg.Notice(message)
}

// Warning returns a formatted warning message in yellow.
func Warning(message string) {
	msg.Warning(message)
}

// Complete returns a formatted message in green.  Used when operations are
// finished.
func Complete(message string) {
	msg.Complete(message)
}

// YamlObjectOutput marshals an object or slice of objects to YAML and prints the output.
// If there is only one object in a slice, it will marshal the single object directly to YAML.
// If there are multiple objects in a slice, it will marshal the entire slice to YAML.
func YamlObjectOutput(objects interface{}) error {
	var output []byte

	// Use reflection to check if the input is a slice
	val := reflect.ValueOf(objects)
	if val.Kind() == reflect.Slice {
		// If it's a slice with exactly one element, marshal just that element
		if val.Len() == 1 {
			singleObject := val.Index(0).Interface()
			o, err := yaml.Marshal(singleObject)
			if err != nil {
				return err
			}
			output = o
		} else {
			// Marshal the entire slice
			o, err := yaml.Marshal(objects)
			if err != nil {
				return err
			}
			output = o
		}
	} else {
		// Not a slice, marshal as-is
		o, err := yaml.Marshal(objects)
		if err != nil {
			return err
		}
		output = o
	}

	fmt.Println(string(output))
	return nil
}

// JsonObjectOutput marshals an object or slice of objects to JSON and prints the output.
// If there is only one object in a slice, it will marshal the single object directly to JSON.
// If there are multiple objects in a slice, it will marshal the entire slice to JSON.
func JsonObjectOutput(objects interface{}) error {
	var output []byte

	// Use reflection to check if the input is a slice
	val := reflect.ValueOf(objects)
	if val.Kind() == reflect.Slice {
		// If it's a slice with exactly one element, marshal just that element
		if val.Len() == 1 {
			singleObject := val.Index(0).Interface()
			o, err := json.MarshalIndent(singleObject, "", "  ")
			if err != nil {
				return err
			}
			output = o
		} else {
			// Marshal the entire slice
			o, err := json.MarshalIndent(objects, "", "  ")
			if err != nil {
				return err
			}
			output = o
		}
	} else {
		// Not a slice, marshal as-is
		o, err := json.MarshalIndent(objects, "", "  ")
		if err != nil {
			return err
		}
		output = o
	}

	fmt.Println(string(output))
	return nil
}
