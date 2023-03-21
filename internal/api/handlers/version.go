package handlers

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/threeport/threeport/internal/version"
)

///////////////////////////////////////////////////////////////////////////////
// API Version
///////////////////////////////////////////////////////////////////////////////

// GetApiVersion gets an REST API version.
// @Summary gets an REST API version.
// @Description Get a version of REST API.
// @ID get-api-version
// @Produce	json
// @Success 200 {object} api.RESTAPIVersion	"OK"
// @Failure 500 {object} api.RESTAPIVersion	"Internal Server Error"
// @Router /version [get]
func (h Handler) GetApiVersion(c echo.Context) error {
	return c.JSON(http.StatusOK, version.RESTAPIVersion{
		Version: version.GetVersion(),
	})
}
