package v0

import (
	"errors"
	"fmt"
	"net/http/httputil"
	"net/url"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

// BeforeDelete ensures that no associated routes exist for this API module
// before deleting it.
func (m *ModuleApi) BeforeDelete(tx *gorm.DB) error {
	var moduleRoutes []ModuleApiRoute
	if result := tx.Where("module_api_id = ?", *m.ID).Find(&moduleRoutes); result.Error != nil {
		return fmt.Errorf("failed to retrieve routes for module API with ID %d: %w", *m.ID, result.Error)
	}

	// return error if associated routes are present
	if len(moduleRoutes) != 0 {
		return fmt.Errorf("module API with ID %d cannot be deleted - has associated routes", *m.ID)
	}

	return nil
}

// BeforeCreate ensures no API route with the route path already exists
// before persisting an API route.
func (m *ModuleApiRoute) BeforeCreate(tx *gorm.DB) error {
	var existingRoute ModuleApiRoute
	if result := tx.Where("path = ?", *m.Path).First(&existingRoute); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			// no existing API module route with this path - return without
			// error
			return nil
		}
		// return any error that is NotFound
		return fmt.Errorf("failed to look up API routes for matching paths: %w", result.Error)
	}

	// no error returned from API route lookup - return error
	return fmt.Errorf("module API route already exists with path %s", *m.Path)
}

// AfterCreate updates the module router after new module API routes are
// created.
func (m *ModuleApiRoute) AfterCreate(tx *gorm.DB) error {
	// retrieve the API module
	var modApi ModuleApi
	if result := tx.Where("id = ?", *m.ModuleApiID).First(&modApi); result.Error != nil {
		return fmt.Errorf("failed to retrieve module API for route %s: %w", *m.Path, result.Error)
	}

	// add the route path to the module router
	ModRouter.AddRoute(*m.Path, func(c echo.Context) error {
		proxyUrl, err := url.Parse(
			fmt.Sprintf("http://%s", *modApi.Endpoint),
		)
		if err != nil {
			return fmt.Errorf("failed to parse module's proxy target URL: %w", err)
		}
		proxy := httputil.NewSingleHostReverseProxy(proxyUrl)
		proxy.ServeHTTP(c.Response().Writer, c.Request())
		return nil
	})

	return nil
}

// AfterDelete updates the module router after a module API route has
// been removed.
func (m *ModuleApiRoute) AfterDelete(tx *gorm.DB) error {
	ModRouter.RemoveRoute(*m.Path)
	return nil
}
