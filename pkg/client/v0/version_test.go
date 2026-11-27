package v0

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
)

// versionServer serves handler and returns the endpoint the way the threeport
// config stores one: host and port without a scheme.
func versionServer(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	return strings.TrimPrefix(server.URL, "http://")
}

func TestGetRestApiVersion(t *testing.T) {
	t.Run("a 200 with a version returns it", func(t *testing.T) {
		var gotPath string
		endpoint := versionServer(t, func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"Version":"v0.7.0-rc.6.a1b2c3d"}`))
		})

		version, err := GetRestApiVersion(&http.Client{}, endpoint)

		require.NoError(t, err)
		assert.Equal(t, "v0.7.0-rc.6.a1b2c3d", version)
		assert.Equal(t, "/version", gotPath)
	})

	t.Run("whitespace around the version is trimmed", func(t *testing.T) {
		endpoint := versionServer(t, func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"Version":" v0.7.0 \n"}`))
		})

		version, err := GetRestApiVersion(&http.Client{}, endpoint)

		require.NoError(t, err)
		assert.Equal(t, "v0.7.0", version)
	})

	t.Run("a non-200 is an error that carries the status and the message", func(t *testing.T) {
		endpoint := versionServer(t, func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "upstream unavailable", http.StatusBadGateway)
		})

		_, err := GetRestApiVersion(&http.Client{}, endpoint)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "502")
		assert.Contains(t, err.Error(), "upstream unavailable")
	})

	t.Run("an error from the API is reported in place of the raw body", func(t *testing.T) {
		endpoint := versionServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"Status":{"Error":"database is down"}}`))
		})

		_, err := GetRestApiVersion(&http.Client{}, endpoint)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "database is down")
		assert.NotContains(t, err.Error(), `"Status"`)
	})

	t.Run("401 and 403 can be recognized with errors.Is", func(t *testing.T) {
		for status, want := range map[int]error{
			http.StatusUnauthorized: client_lib.ErrUnauthorized,
			http.StatusForbidden:    client_lib.ErrForbidden,
		} {
			endpoint := versionServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
			})

			_, err := GetRestApiVersion(&http.Client{}, endpoint)

			assert.ErrorIs(t, err, want, "status %d", status)
		}
	})

	t.Run("a malformed body is an error", func(t *testing.T) {
		endpoint := versionServer(t, func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`<html>not json</html>`))
		})

		_, err := GetRestApiVersion(&http.Client{}, endpoint)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "decode")
	})

	t.Run("a 200 without a version is an error", func(t *testing.T) {
		endpoint := versionServer(t, func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"Data":[]}`))
		})

		_, err := GetRestApiVersion(&http.Client{}, endpoint)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "no version")
	})

	t.Run("an endpoint nothing answers on is an error", func(t *testing.T) {
		// take a port the OS just handed out and close it again
		server := httptest.NewServer(http.NotFoundHandler())
		endpoint := strings.TrimPrefix(server.URL, "http://")
		server.Close()

		_, err := GetRestApiVersion(&http.Client{}, endpoint)

		require.Error(t, err)
	})
}

func TestGetRestApiVersionTimeout(t *testing.T) {
	// shorten the bound so the test does not wait ten seconds
	previous := restApiVersionTimeout
	restApiVersionTimeout = 100 * time.Millisecond
	t.Cleanup(func() { restApiVersionTimeout = previous })

	// release holds the handler open; it is closed before the server so that
	// the server's Close does not wait on a request that is still blocked
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() { close(release) })
	endpoint := strings.TrimPrefix(server.URL, "http://")

	t.Run("a client with no timeout is bounded", func(t *testing.T) {
		start := time.Now()
		_, err := GetRestApiVersion(&http.Client{}, endpoint)

		require.Error(t, err)
		assert.Less(t, time.Since(start), 5*time.Second)
	})

	t.Run("a client with a longer timeout is bounded and left unchanged", func(t *testing.T) {
		apiClient := &http.Client{Timeout: 2 * time.Minute}

		start := time.Now()
		_, err := GetRestApiVersion(apiClient, endpoint)

		require.Error(t, err)
		assert.Less(t, time.Since(start), 5*time.Second)
		assert.Equal(t, 2*time.Minute, apiClient.Timeout, "the caller's client must not be changed")
	})

	t.Run("a client with a shorter timeout keeps it", func(t *testing.T) {
		restApiVersionTimeout = time.Minute
		t.Cleanup(func() { restApiVersionTimeout = 100 * time.Millisecond })
		apiClient := &http.Client{Timeout: 100 * time.Millisecond}

		start := time.Now()
		_, err := GetRestApiVersion(apiClient, endpoint)

		require.Error(t, err)
		assert.Less(t, time.Since(start), 5*time.Second)
	})
}
