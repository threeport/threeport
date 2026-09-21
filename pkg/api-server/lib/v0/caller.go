package v0

import (
	"github.com/labstack/echo/v4"

	lib "github.com/threeport/threeport/pkg/api/lib/v0"
	auth "github.com/threeport/threeport/pkg/auth/v0"
)

// Database hooks read caller identity from the request context to
// decide whether a caller may change rows another object owns.

// CaptureCaller returns middleware that stashes the mTLS peer identity
// on the request context. It does not authenticate or reject the request.
func CaptureCaller(authEnabled bool) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// copy identity from the leaf client certificate subject
			if tlsState := c.Request().TLS; tlsState != nil && len(tlsState.PeerCertificates) > 0 {
				subject := tlsState.PeerCertificates[0].Subject
				id := lib.CallerIdentity{CommonName: subject.CommonName}
				if len(subject.Organization) > 0 {
					id.Organization = subject.Organization[0]
				}
				if len(subject.OrganizationalUnit) > 0 {
					id.OrganizationalUnit = subject.OrganizationalUnit[0]
				}
				c.SetRequest(c.Request().WithContext(
					lib.WithCaller(c.Request().Context(), id),
				))
				return next(c)
			}

			// treat the caller as the control plane when auth is off so
			// reconcilers can still update the rows they own
			if !authEnabled {
				c.SetRequest(c.Request().WithContext(
					lib.WithCaller(c.Request().Context(), lib.CallerIdentity{
						OrganizationalUnit: auth.OUControlPlane,
					}),
				))
				return next(c)
			}

			// leave identity empty so a request with no client
			// certificate is handled as an external caller
			return next(c)
		}
	}
}
