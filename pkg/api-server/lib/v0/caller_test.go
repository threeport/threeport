package v0

import (
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apilib "github.com/threeport/threeport/pkg/api/lib/v0"
	auth "github.com/threeport/threeport/pkg/auth/v0"
)

// TestCaptureCallerStashesIdentity covers a peer certificate and the
// two no-certificate paths.
func TestCaptureCallerStashesIdentity(t *testing.T) {
	// build a peer certificate with a control-plane subject
	peer := &x509.Certificate{
		Subject: pkix.Name{
			CommonName:         "api-server",
			Organization:       []string{"threeport"},
			OrganizationalUnit: []string{auth.OUControlPlane},
		},
	}

	// define the auth and peer combinations
	tests := []struct {
		name        string
		authEnabled bool
		peer        *x509.Certificate
		want        apilib.CallerIdentity
	}{
		{
			name:        "peer certificate identity is stashed",
			authEnabled: true,
			peer:        peer,
			want: apilib.CallerIdentity{
				CommonName:         "api-server",
				Organization:       "threeport",
				OrganizationalUnit: auth.OUControlPlane,
			},
		},
		{
			name:        "peer certificate is stashed even when auth is off",
			authEnabled: false,
			peer:        peer,
			want: apilib.CallerIdentity{
				CommonName:         "api-server",
				Organization:       "threeport",
				OrganizationalUnit: auth.OUControlPlane,
			},
		},
		{
			name:        "auth off with no certificate is the control plane",
			authEnabled: false,
			want: apilib.CallerIdentity{
				OrganizationalUnit: auth.OUControlPlane,
			},
		},
		{
			name:        "auth on with no certificate leaves identity empty",
			authEnabled: true,
			want:        apilib.CallerIdentity{},
		},
	}

	// run each combination
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// set up an echo context for the case
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.peer != nil {
				// attach the peer certificate to the request TLS state
				req.TLS = &tls.ConnectionState{
					PeerCertificates: []*x509.Certificate{tc.peer},
				}
			}
			c := e.NewContext(req, httptest.NewRecorder())

			// run CaptureCaller and read the identity from the request context
			var got apilib.CallerIdentity
			err := CaptureCaller(tc.authEnabled)(func(c echo.Context) error {
				got = apilib.Caller(c.Request().Context())
				return nil
			})(c)
			require.NoError(t, err)

			// assert the stashed identity matches the case
			assert.Equal(t, tc.want, got)
		})
	}
}
