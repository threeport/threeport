package v0

import (
	"fmt"
	"strconv"

	"github.com/labstack/echo/v4"
)

// PathParamId is the path parameter that carries an object's ID on the by-ID
// routes.
const PathParamId = "id"

// ParseObjectID returns the object ID in a request's path as a number.
//
// A handler must query with this value rather than the raw path parameter.
// GORM reads a string it is given as the condition of First as SQL when the
// string is not a number, so a request for /1%20OR%201=1 would otherwise run
// `WHERE (1 OR 1=1)` and act on a row the caller never named.
func ParseObjectID(c echo.Context) (uint, error) {
	raw := c.Param(PathParamId)
	id, err := strconv.ParseUint(raw, 10, strconv.IntSize)
	if err != nil {
		return 0, fmt.Errorf("invalid id %q: must be a non-negative integer", raw)
	}

	return uint(id), nil
}
