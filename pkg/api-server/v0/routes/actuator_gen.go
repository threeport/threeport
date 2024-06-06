// generated by 'threeport-sdk gen' - do not edit

package routes

import (
	echo "github.com/labstack/echo/v4"
	handlers "github.com/threeport/threeport/pkg/api-server/v0/handlers"
	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// ProfileRoutes sets up all routes for the Profile handlers.
func ProfileRoutes(e *echo.Echo, h *handlers.Handler) {
	e.GET("/profiles/versions", h.GetProfileVersions)

	e.POST(v0.PathProfiles, h.AddProfile)
	e.GET(v0.PathProfiles, h.GetProfiles)
	e.GET(v0.PathProfiles+"/:id", h.GetProfile)
	e.PATCH(v0.PathProfiles+"/:id", h.UpdateProfile)
	e.PUT(v0.PathProfiles+"/:id", h.ReplaceProfile)
	e.DELETE(v0.PathProfiles+"/:id", h.DeleteProfile)
}

// TierRoutes sets up all routes for the Tier handlers.
func TierRoutes(e *echo.Echo, h *handlers.Handler) {
	e.GET("/tiers/versions", h.GetTierVersions)

	e.POST(v0.PathTiers, h.AddTier)
	e.GET(v0.PathTiers, h.GetTiers)
	e.GET(v0.PathTiers+"/:id", h.GetTier)
	e.PATCH(v0.PathTiers+"/:id", h.UpdateTier)
	e.PUT(v0.PathTiers+"/:id", h.ReplaceTier)
	e.DELETE(v0.PathTiers+"/:id", h.DeleteTier)
}
