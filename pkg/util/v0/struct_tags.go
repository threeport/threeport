package v0

import (
	"reflect"
	"strings"

)

// ParseStructTag parses a struct tag string into a map[string]string
func ParseStructTag(tagString string) map[string]string {
	tag := reflect.StructTag(strings.Trim(tagString, "`"))
	tagMap := make(map[string]string)
	for _, key := range tagList(tag) {
		tagMap[key] = tag.Get(key)
	}
	return tagMap
}

// tagList extracts keys from a struct tag
func tagList(tag reflect.StructTag) []string {
	raw := string(tag)
	var list []string
	for raw != "" {
		var pair string
		pair, raw = next(raw)
		key, _ := split(pair)
		list = append(list, key)
	}
	return list
}

// next gets the next key-value pair from a struct tag
func next(raw string) (pair, rest string) {
	i := strings.Index(raw, " ")
	if i < 0 {
		return raw, ""
	}
	return raw[:i], raw[i+1:]
}

// split splits a key-value pair from a struct tag
func split(pair string) (key, value string) {
	i := strings.Index(pair, ":")
	if i < 0 {
		return pair, ""
	}
	key = strings.TrimSpace(pair[:i])
	value = strings.TrimSpace(pair[i+1:])
	return key, value
}
