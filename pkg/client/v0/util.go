package v0

import (
	"os"
	"strconv"
)

const GoClientDebug = "ThreeportGoClientDebug"

func IsDebug() bool {
	v, err := strconv.ParseBool(os.Getenv(GoClientDebug))
	if err == nil && v {
		return true
	}
	return false
}
