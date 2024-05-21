package gen

// CheckStructTagMap checks if a struct tag map contains a specific value.
func (a *ApiObjectGroup) CheckStructTagMap(
	object,
	field,
	tagKey,
	expectedTagValue string,
) bool {
	if fieldTagMap, objectKeyFound := a.StructTags[object]; objectKeyFound {
		if tagValueMap, fieldKeyFound := fieldTagMap[field]; fieldKeyFound {
			if tagValue, tagKeyFound := tagValueMap[tagKey]; tagKeyFound {
				if tagValue == expectedTagValue {
					return true
				}
			}
		}
	}
	return false
}
