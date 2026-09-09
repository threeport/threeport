package v0

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
)

var Versions = make(map[int]string)

func ResponseStatus200(c echo.Context, response Response) error {
	code := http.StatusOK
	message := http.StatusText(code)
	UpdateResponseStatus(&response, code, message, "")
	return c.JSON(code, response)
}

func ResponseStatus201(c echo.Context, response Response) error {
	code := http.StatusCreated
	message := http.StatusText(code)
	UpdateResponseStatus(&response, code, message, "")
	return c.JSON(code, response)
}

func ResponseStatus202(c echo.Context, response Response) error {
	code := http.StatusAccepted
	message := http.StatusText(code)
	UpdateResponseStatus(&response, code, message, "")
	return c.JSON(code, response)
}

func ResponseStatusExpected(id int, c echo.Context, response Response) error {
	switch id {
	case 200:
		return ResponseStatus200(c, response)
	case 201:
		return ResponseStatus201(c, response)
	}
	return c.JSON(http.StatusInternalServerError, "")
}

func ResponseStatus400(c echo.Context, params *PageRequestParams, error error, objectType string) error {
	return c.JSON(http.StatusBadRequest, CreateResponseWithError400(params, error, objectType))
}

func ResponseStatus401(c echo.Context, params *PageRequestParams, error error, objectType string) error {
	return c.JSON(http.StatusUnauthorized, CreateResponseWithError401(params, error, objectType))
}

func ResponseStatus403(c echo.Context, params *PageRequestParams, error error, objectType string) error {
	return c.JSON(http.StatusForbidden, CreateResponseWithError403(params, error, objectType))
}

func ResponseStatus404(c echo.Context, params *PageRequestParams, error error, objectType string) error {
	return c.JSON(http.StatusNotFound, CreateResponseWithError404(params, error, objectType))
}

func ResponseStatus409(c echo.Context, params *PageRequestParams, error error, objectType string) error {
	return c.JSON(http.StatusConflict, CreateResponseWithError409(params, error, objectType))
}

func ResponseStatus500(c echo.Context, params *PageRequestParams, error error, objectType string) error {
	return c.JSON(http.StatusInternalServerError, CreateResponseWithError500(params, error, objectType))
}

// ResponseStatusBindErr maps an echo request-binding error to an HTTP
// response. An *echo.HTTPError carrying a 4xx code, typically 400 for a
// JSON unmarshal or type mismatch, is returned to the client at that
// code. Any other error, and any *echo.HTTPError carrying a 5xx code,
// is returned as an internal server error.
//
// Only the error's Message reaches the body. Rendering the whole error
// would repeat the code already carried in Status.Code and append the
// wrapped cause, which names Go struct and field types. The caller logs
// the whole error before calling this, so that detail stays available
// on the server.
func ResponseStatusBindErr(c echo.Context, params *PageRequestParams, err error, objectType string) error {
	var httpErr *echo.HTTPError
	if errors.As(err, &httpErr) && httpErr.Code >= 400 && httpErr.Code < 500 {
		return c.JSON(httpErr.Code, CreateResponseErrorWithStatus(
			params,
			CreateStatus(httpErr.Code, http.StatusText(httpErr.Code), fmt.Sprintf("%v", httpErr.Message)),
			objectType,
		))
	}
	return ResponseStatus500(c, params, err, objectType)
}

func ResponseStatusErr(id int, c echo.Context, params *PageRequestParams, error error, objectType string) error {
	switch id {
	case 400:
		return ResponseStatus400(c, params, error, objectType)
	case 401:
		return ResponseStatus401(c, params, error, objectType)
	case 403:
		return ResponseStatus403(c, params, error, objectType)
	case 404:
		return ResponseStatus404(c, params, error, objectType)
	case 409:
		return ResponseStatus409(c, params, error, objectType)
	case 500:
		return ResponseStatus500(c, params, error, objectType)
	}

	return c.JSON(http.StatusInternalServerError, "")
}
