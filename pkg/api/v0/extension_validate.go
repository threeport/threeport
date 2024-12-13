package v0

import (
	"errors"
	"fmt"
	"net/http/httputil"
	"net/url"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

// BeforeDelete ensures that no associated routes exist for this API extension
// before deleting it.
func (e *ExtensionApi) BeforeDelete(tx *gorm.DB) error {
	var extensionRoutes []ExtensionApiRoute
	if result := tx.Where("extension_api_id = ?", *e.ID).Find(&extensionRoutes); result.Error != nil {
		return fmt.Errorf("failed to retrieve routes for extension API with ID %d: %w", *e.ID, result.Error)
	}

	// return error if associated routes are present
	if len(extensionRoutes) != 0 {
		return fmt.Errorf("extension API with ID %d cannot be deleted - has associated routes", *e.ID)
	}

	return nil
}

// BeforeCreate ensures no API route with the route path already exists
// before persisting an API route.
func (e *ExtensionApiRoute) BeforeCreate(tx *gorm.DB) error {
	var existingRoute ExtensionApiRoute
	if result := tx.Where("path = ?", *e.Path).First(&existingRoute); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			// no existing API extension route with this path - return without
			// error
			return nil
		}
		// return any error that is NotFound
		return fmt.Errorf("failed to look up API routes for matching paths: %w", result.Error)
	}

	// no error returned from API route lookup - return error
	return fmt.Errorf("extension API route already exists with path %s", *e.Path)
}

// AfterCreate updates the extension router after new extension API routes are
// created.
func (e *ExtensionApiRoute) AfterCreate(tx *gorm.DB) error {
	// retrieve the API extension
	var extApi ExtensionApi
	if result := tx.Where("id = ?", *e.ExtensionApiID).First(&extApi); result.Error != nil {
		return fmt.Errorf("failed to retrieve extension API for route %s: %w", *e.Path, result.Error)
	}

	// add the route path to the extension router
	ExtRouter.AddRoute(*e.Path, func(c echo.Context) error {
		proxyUrl, err := url.Parse(
			fmt.Sprintf("http://%s", *extApi.Endpoint),
		)
		if err != nil {
			return fmt.Errorf("failed to parse extension's proxy target URL: %w", err)
		}
		proxy := httputil.NewSingleHostReverseProxy(proxyUrl)
		proxy.ServeHTTP(c.Response().Writer, c.Request())
		return nil
	})

	return nil
}

// AfterDelete updates the extension router after an extension API route has
// been removed.
func (e *ExtensionApiRoute) AfterDelete(tx *gorm.DB) error {
	ExtRouter.RemoveRoute(*e.Path)
	return nil
}
