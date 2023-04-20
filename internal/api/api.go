package api

import (
	"github.com/labstack/echo/v4"

	v0 "github.com/threeport/threeport/pkg/api/v0"

	"net/http"
)

const (
	V0 = "v0"
)

var Versions = make(map[int]string)

func ResponseStatus200(c echo.Context, response v0.Response) error {
	code := http.StatusOK
	message := http.StatusText(code)
	v0.UpdateResponseStatus(&response, code, message, "")
	return c.JSON(code, response)
}

func ResponseStatus201(c echo.Context, response v0.Response) error {
	code := http.StatusCreated
	message := http.StatusText(code)
	v0.UpdateResponseStatus(&response, code, message, "")
	return c.JSON(code, response)
}

func ResponseStatus400(c echo.Context, params *v0.PageRequestParams, error error, objectType v0.ObjectType) error {
	return c.JSON(http.StatusBadRequest, v0.CreateResponseWithError400(params, error, objectType))
}

func ResponseStatusExpected(id int, c echo.Context, response v0.Response) error {
	switch id {
	case 200:
		return ResponseStatus200(c, response)
	case 201:
		return ResponseStatus201(c, response)
	}
	return c.JSON(http.StatusInternalServerError, "")
}

func ResponseStatus401(c echo.Context, params *v0.PageRequestParams, error error, objectType v0.ObjectType) error {
	return c.JSON(http.StatusUnauthorized, v0.CreateResponseWithError401(params, error, objectType))
}

func ResponseStatus404(c echo.Context, params *v0.PageRequestParams, error error, objectType v0.ObjectType) error {
	return c.JSON(http.StatusNotFound, v0.CreateResponseWithError404(params, error, objectType))
}

func ResponseStatus409(c echo.Context, params *v0.PageRequestParams, error error, objectType v0.ObjectType) error {
	return c.JSON(http.StatusConflict, v0.CreateResponseWithError409(params, error, objectType))
}

func ResponseStatus500(c echo.Context, params *v0.PageRequestParams, error error, objectType v0.ObjectType) error {
	return c.JSON(http.StatusInternalServerError, v0.CreateResponseWithError500(params, error, objectType))
}

func ResponseStatusErr(id int, c echo.Context, params *v0.PageRequestParams, error error, objectType v0.ObjectType) error {
	switch id {
	case 400:
		return ResponseStatus400(c, params, error, objectType)
	case 401:
		return ResponseStatus401(c, params, error, objectType)
	case 404:
		return ResponseStatus404(c, params, error, objectType)
	case 500:
		return ResponseStatus500(c, params, error, objectType)
	}

	return c.JSON(http.StatusInternalServerError, "")
}
