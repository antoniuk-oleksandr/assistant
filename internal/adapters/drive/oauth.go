package drive

import (
	"context"
	"errors"
	"net/http"

	"github.com/alex/assistent/internal/platform/config"
	"golang.org/x/oauth2"
)

const readonlyScope = "https://www.googleapis.com/auth/drive.readonly"

func newTokenSource(cfg config.DriveConfig, endpoint string, client *http.Client) (oauth2.TokenSource, error) {
	if cfg.ClientID == "" && cfg.ClientSecret == "" && cfg.RefreshToken == "" {
		return nil, nil
	}
	if cfg.ClientID == "" || cfg.ClientSecret == "" || cfg.RefreshToken == "" {
		return nil, errors.New("Drive OAuth requires GOOGLE_DRIVE_OAUTH_CLIENT_ID, GOOGLE_DRIVE_OAUTH_CLIENT_SECRET, and GOOGLE_DRIVE_OAUTH_REFRESH_TOKEN")
	}
	oauth := oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		Scopes:       []string{readonlyScope},
		Endpoint:     oauth2.Endpoint{TokenURL: endpoint, AuthStyle: oauth2.AuthStyleInParams},
	}
	// The source is shared across polls. The HTTP client bounds token refreshes
	// without tying future refreshes to a single poll's context.
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, client)
	return oauth.TokenSource(ctx, &oauth2.Token{RefreshToken: cfg.RefreshToken}), nil
}
