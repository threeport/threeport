package v0

import (
	"encoding/json"

	"gorm.io/datatypes"

	api_v0 "github.com/threeport/threeport/pkg/api/v0"
)

// cidrsToJSON returns cidrs as the JSON array the API stores, or nil when
// none are given.
func cidrsToJSON(cidrs *[]string) *datatypes.JSON {
	if cidrs == nil {
		return nil
	}

	// a slice of strings always marshals
	raw, _ := json.Marshal(*cidrs)
	cidrsJSON := datatypes.JSON(raw)
	return &cidrsJSON
}

// cidrsFromJSON returns the CIDR blocks in a JSON array the API stores, or
// nil when it holds none.  The API validates the array on every write, so
// one that cannot be read is treated as holding none.
func cidrsFromJSON(raw *datatypes.JSON) *[]string {
	cidrs, err := api_v0.ParseCidrs("", raw)
	if err != nil || cidrs == nil {
		return nil
	}
	return &cidrs
}

// validateAllowedCidrs checks a config's AllowedCidrs before it is sent to
// the API, which checks it again along with the provider's limit.
func validateAllowedCidrs(cidrs *[]string) error {
	_, err := api_v0.ParseCidrs("AllowedCidrs", cidrsToJSON(cidrs))
	return err
}
