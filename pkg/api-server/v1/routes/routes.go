// generated by 'threeport-sdk gen' but will not be regenerated - intended for modification

package routes

import (
	echo "github.com/labstack/echo/v4"
	handlers "github.com/threeport/threeport/pkg/api-server/v1/handlers"
)

// AddCustomRoutes adds non-code-generated routes for special use cases.
func AddCustomRoutes(e *echo.Echo, h *handlers.Handler) {
	EventsCustomRoutes(e, h)
}
