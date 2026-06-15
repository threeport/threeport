package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"golang.org/x/oauth2/google"

	gcpauth "github.com/threeport/threeport/pkg/auth/v0"
)

// adcOAuthUserEmail returns the human Google account email when Application Default
// Credentials come from an interactive user (OAuth). For service accounts and other
// non-user credential types it returns ("", false, nil).
func adcOAuthUserEmail(ctx context.Context) (email string, isOAuthUser bool, err error) {
	creds, err := google.FindDefaultCredentials(ctx, gcpauth.GcpOAuthScopes...)
	if err != nil {
		return "", false, fmt.Errorf("failed to find default credentials: %w", err)
	}

	if len(creds.JSON) > 0 {
		var meta struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(creds.JSON, &meta) == nil {
			switch meta.Type {
			case "service_account", "external_account", "impersonated_service_account":
				return "", false, nil
			case "authorized_user":
				isOAuthUser = true
			default:
				if meta.Type != "" {
					return "", false, nil
				}
			}
		}
	}

	token, err := creds.TokenSource.Token()
	if err != nil {
		return "", false, fmt.Errorf("failed to get access token for GCP credentials: %w", err)
	}

	uiEmail, err := fetchOAuth2UserinfoEmail(ctx, token.AccessToken)
	if err != nil {
		return "", false, nil
	}
	if uiEmail == "" {
		if isOAuthUser {
			return "", true, fmt.Errorf("could not resolve user email from OAuth2 userinfo (credentials type authorized_user)")
		}
		return "", false, nil
	}

	return uiEmail, true, nil
}

func fetchOAuth2UserinfoEmail(ctx context.Context, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.googleapis.com/oauth2/v2/userinfo", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", nil
	}

	var ui struct {
		Email string `json:"email"`
	}
	if json.Unmarshal(body, &ui) != nil {
		return "", nil
	}
	return strings.TrimSpace(ui.Email), nil
}

// validateADCUserMatchesGcloudAccount ensures interactive user ADC matches the account
// in the active gcloud configuration when that profile sets core.account.
func validateADCUserMatchesGcloudAccount(ctx context.Context) error {
	gcloudAcct := readActiveGcloudAccount()
	if gcloudAcct == "" {
		return nil
	}

	adcEmail, ok, err := adcOAuthUserEmail(ctx)
	if err != nil {
		return err
	}
	if !ok || adcEmail == "" {
		return nil
	}

	if strings.EqualFold(adcEmail, gcloudAcct) {
		return nil
	}

	return fmt.Errorf(
		"GCP credentials are authenticated as %q, but the active gcloud configuration uses account %q. "+
			"When project and region are taken from gcloud, sign in with the same Google account during the browser flow, "+
			"or align `gcloud config set account` with the account you use for Application Default Credentials. "+
			"You can also pass --gcp-project-id and --gcp-region explicitly for a project your current credentials can administer",
		adcEmail, gcloudAcct,
	)
}
