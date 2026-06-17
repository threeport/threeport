package v0

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	util "github.com/threeport/threeport/pkg/util/v0"
)

// GCP OAuth2 configuration for Application Default Credentials.
// These are the public client credentials used by gcloud CLI for user authentication.
// See: https://cloud.google.com/sdk/docs/authorizing
const (
	gcpOAuthClientID     = "764086051850-6qr4p6gpi6hn506pt8ejuq83di341hur.apps.googleusercontent.com"
	gcpOAuthClientSecret = "d-FL95Q19q7MQmFpd7hHD0Ty"
)

// GcpOAuthScopes defines the scopes needed for GKE operations.
var GcpOAuthScopes = []string{
	"https://www.googleapis.com/auth/cloud-platform",
	"https://www.googleapis.com/auth/userinfo.email",
}

// adcCredentials represents the structure of the Application Default Credentials file.
type adcCredentials struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RefreshToken string `json:"refresh_token"`
	Type         string `json:"type"`
}

// EnsureGCPAuth checks for valid GCP Application Default Credentials and initiates
// the OAuth flow if credentials are missing or invalid. This allows users to
// authenticate without manually running `gcloud auth application-default login`.
//
// This function handles three authentication scenarios:
//  1. CLI usage (tptctl): Uses browser-based OAuth flow for user authentication
//  2. Controller in GKE: Uses Workload Identity (automatic via metadata server)
//  3. Controller outside GCP: Uses service account credentials JSON
//
// The serviceAccountCredentials parameter should contain the JSON contents of a
// GCP service account key file. If empty, the function will check for existing
// credentials (scenarios 1 and 2) and fall back to browser-based auth if needed.
func EnsureGCPAuth(serviceAccountCredentials string) error {
	ctx := context.Background()

	// FIRST: Check if valid credentials already exist.
	// This covers (in order of preference):
	// - Workload Identity in GKE (scenario 2) — most secure, uses short-lived tokens
	// - User credentials from gcloud auth (scenario 1)
	// - Previously configured service account key file via GOOGLE_APPLICATION_CREDENTIALS
	if hasValidGCPCredentials(ctx) {
		// Only increment the ref count if a temp SA file is actually in use.
		// If Workload Identity or user ADC provided the valid credentials,
		// gcpCredTempFile is empty and no cleanup pairing is needed — the
		// conditional defer in the caller will fire but CleanupGCPCredentials
		// is a no-op when refCount is already zero.
		if serviceAccountCredentials != "" {
			gcpCredMu.Lock()
			if gcpCredTempFile != "" {
				gcpCredRefCount++
			}
			gcpCredMu.Unlock()
		}
		return nil
	}

	// SECOND: If no valid credentials exist and service account credentials are
	// provided, use them. This is the fallback for controllers running outside GCP.
	if serviceAccountCredentials != "" {
		if err := configureServiceAccountCredentials(serviceAccountCredentials); err != nil {
			return fmt.Errorf("failed to configure service account credentials: %w", err)
		}
		return nil
	}

	// THIRD: Fall back to browser-based OAuth flow (scenario 1 — CLI only).
	// This only works for CLI usage (tptctl), not for controllers.
	util.CliOutputInfo("GCP credentials not found or expired. Initiating authentication...")

	if err := performGCPOAuthFlow(ctx); err != nil {
		return fmt.Errorf("failed to authenticate with GCP: %w", err)
	}

	util.CliOutputInfo("GCP authentication successful!")
	return nil
}

// gcpCredMu guards gcpCredTempFile and gcpCredRefCount against concurrent access.
var gcpCredMu sync.Mutex

// gcpCredTempFile holds the path of any temp credentials file written by
// configureServiceAccountCredentials so it can be removed when no longer needed.
var gcpCredTempFile string

// gcpCredRefCount tracks how many concurrent operations are relying on the
// temp credentials file. The file is removed when this reaches zero.
var gcpCredRefCount int

// CleanupGCPCredentials decrements the credential ref count and removes the
// temporary service account key file once all concurrent operations have
// released it. Each call to EnsureGCPAuth with a non-empty
// serviceAccountCredentials must be paired with exactly one CleanupGCPCredentials.
func CleanupGCPCredentials() {
	gcpCredMu.Lock()
	defer gcpCredMu.Unlock()
	if gcpCredRefCount > 0 {
		gcpCredRefCount--
	}
	if gcpCredRefCount == 0 && gcpCredTempFile != "" {
		os.Remove(gcpCredTempFile)
		os.Unsetenv("GOOGLE_APPLICATION_CREDENTIALS")
		gcpCredTempFile = ""
	}
}

// configureServiceAccountCredentials writes the service account JSON to a
// temporary file and sets the GOOGLE_APPLICATION_CREDENTIALS environment
// variable to point to it. The file must persist for the process lifetime
// since the Google SDK reads it on every token refresh; call
// CleanupGCPCredentials at shutdown to remove it.
func configureServiceAccountCredentials(credentialsJSON string) error {
	tmpFile, err := os.CreateTemp("", "gcp-sa-*.json")
	if err != nil {
		return fmt.Errorf("failed to create temp credentials file: %w", err)
	}

	if _, err := tmpFile.WriteString(credentialsJSON); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return fmt.Errorf("failed to write credentials to temp file: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpFile.Name())
		return fmt.Errorf("failed to close temp credentials file: %w", err)
	}

	gcpCredMu.Lock()
	if gcpCredTempFile != "" {
		// Another goroutine registered credentials while we were writing the
		// file — discard ours and reuse the existing one to avoid orphaning
		// a file containing key material.
		os.Remove(tmpFile.Name())
		gcpCredRefCount++
		gcpCredMu.Unlock()
		return nil
	}
	gcpCredTempFile = tmpFile.Name()
	os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", tmpFile.Name())
	gcpCredRefCount++
	gcpCredMu.Unlock()
	return nil
}

// hasValidGCPCredentials checks if valid Application Default Credentials exist
// and that those credentials carry the cloud-platform scope required for IAM
// operations.
func hasValidGCPCredentials(ctx context.Context) bool {
	tokenSource, err := google.DefaultTokenSource(ctx, GcpOAuthScopes...)
	if err != nil {
		return false
	}

	token, err := tokenSource.Token()
	if err != nil {
		return false
	}

	if !token.Valid() {
		return false
	}

	return gcpTokenHasCloudPlatformScope(token)
}

// tokeninfoClient is a dedicated HTTP client for scope checks with a short
// timeout so a stalled tokeninfo response never blocks EnsureGCPAuth.
var tokeninfoClient = &http.Client{Timeout: 5 * time.Second}

// gcpTokenHasCloudPlatformScope verifies the access token includes the
// cloud-platform scope by querying the Google tokeninfo endpoint.
func gcpTokenHasCloudPlatformScope(token *oauth2.Token) bool {
	resp, err := tokeninfoClient.Get("https://oauth2.googleapis.com/tokeninfo?access_token=" + token.AccessToken)
	if err != nil {
		return true
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false
	}

	var info struct {
		Scope string `json:"scope"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return true
	}

	for _, s := range strings.Fields(info.Scope) {
		if s == "https://www.googleapis.com/auth/cloud-platform" {
			return true
		}
	}
	return false
}

// performGCPOAuthFlow performs the browser-based OAuth flow for GCP authentication.
func performGCPOAuthFlow(ctx context.Context) error {
	state, err := generateRandomState()
	if err != nil {
		return fmt.Errorf("failed to generate state: %w", err)
	}

	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return fmt.Errorf("failed to create listener: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	redirectURL := fmt.Sprintf("http://localhost:%d/callback", port)

	oauth2Config := &oauth2.Config{
		ClientID:     gcpOAuthClientID,
		ClientSecret: gcpOAuthClientSecret,
		Endpoint:     google.Endpoint,
		RedirectURL:  redirectURL,
		Scopes:       GcpOAuthScopes,
	}

	codeChan := make(chan string, 1)
	errChan := make(chan error, 1)

	mux := http.NewServeMux()
	server := &http.Server{Handler: mux}
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			errChan <- fmt.Errorf("invalid state parameter")
			http.Error(w, "Invalid state parameter", http.StatusBadRequest)
			return
		}

		if errMsg := r.URL.Query().Get("error"); errMsg != "" {
			errChan <- fmt.Errorf("OAuth error: %s - %s", errMsg, r.URL.Query().Get("error_description"))
			http.Error(w, "Authentication failed", http.StatusBadRequest)
			return
		}

		code := r.URL.Query().Get("code")
		if code == "" {
			errChan <- fmt.Errorf("no authorization code received")
			http.Error(w, "No authorization code received", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<html><body><h1>Authentication Successful!</h1><p>You can close this window and return to the terminal.</p></body></html>`)
		codeChan <- code
	})

	go func() {
		if err := server.Serve(listener); err != http.ErrServerClosed {
			errChan <- fmt.Errorf("callback server error: %w", err)
		}
	}()

	authURL := oauth2Config.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)

	util.CliOutputNotice("Opening browser for GCP authentication...")
	util.CliOutputInfo("If the browser doesn't open automatically, please visit:")
	util.CliOutputInfo(authURL)
	fmt.Println()

	if err := openBrowser(authURL); err != nil {
		util.CliOutputWarning("Failed to open browser automatically. Please open the URL above manually.")
	}

	var code string
	select {
	case code = <-codeChan:
	case err := <-errChan:
		server.Shutdown(ctx)
		return err
	case <-time.After(5 * time.Minute):
		server.Shutdown(ctx)
		return fmt.Errorf("authentication timed out after 5 minutes")
	}

	server.Shutdown(ctx)

	token, err := oauth2Config.Exchange(ctx, code)
	if err != nil {
		return fmt.Errorf("failed to exchange authorization code: %w", err)
	}

	if err := saveADCCredentials(token); err != nil {
		return fmt.Errorf("failed to save credentials: %w", err)
	}

	return nil
}

// saveADCCredentials saves the OAuth2 token as Application Default Credentials.
func saveADCCredentials(token *oauth2.Token) error {
	adcPath, err := getADCPath()
	if err != nil {
		return err
	}

	adcDir := filepath.Dir(adcPath)
	if err := os.MkdirAll(adcDir, 0700); err != nil {
		return fmt.Errorf("failed to create ADC directory: %w", err)
	}

	creds := adcCredentials{
		ClientID:     gcpOAuthClientID,
		ClientSecret: gcpOAuthClientSecret,
		RefreshToken: token.RefreshToken,
		Type:         "authorized_user",
	}

	credsJSON, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal credentials: %w", err)
	}

	if err := os.WriteFile(adcPath, credsJSON, 0600); err != nil {
		return fmt.Errorf("failed to write ADC file: %w", err)
	}

	return nil
}

// getADCPath returns the standard well-known path for Application Default
// Credentials. Intentionally ignores GOOGLE_APPLICATION_CREDENTIALS — that
// env var may point to a service account temp file set by
// configureServiceAccountCredentials, and overwriting it with OAuth user
// credentials would silently destroy those service account credentials.
func getADCPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	if runtime.GOOS == "windows" {
		return filepath.Join(homeDir, "AppData", "Roaming", "gcloud", "application_default_credentials.json"), nil
	}

	return filepath.Join(homeDir, ".config", "gcloud", "application_default_credentials.json"), nil
}

// generateRandomState generates a random state string for CSRF protection.
func generateRandomState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// openBrowser opens the specified URL in the default browser.
func openBrowser(url string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		if _, err := exec.LookPath("xdg-open"); err == nil {
			cmd = exec.Command("xdg-open", url)
		} else if _, err := exec.LookPath("gnome-open"); err == nil {
			cmd = exec.Command("gnome-open", url)
		} else if _, err := exec.LookPath("kde-open"); err == nil {
			cmd = exec.Command("kde-open", url)
		} else {
			return fmt.Errorf("no browser opener found")
		}
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", strings.ReplaceAll(url, "&", "^&"))
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	return cmd.Start()
}
