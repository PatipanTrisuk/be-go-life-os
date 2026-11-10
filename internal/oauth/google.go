package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

var (
	ErrGoogleNotConfigured = errors.New("google oauth is not configured")
	ErrGoogleExchange      = errors.New("google token exchange failed")
	ErrGoogleUserFetch     = errors.New("google user fetch failed")
	ErrGoogleEmailMissing  = errors.New("google account has no verified email")
)

type GoogleConfig struct {
	ClientID     string
	ClientSecret string
}

func GoogleConfigFromEnv() GoogleConfig {
	return GoogleConfig{
		ClientID:     strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_ID")),
		ClientSecret: strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_SECRET")),
	}
}

func (c GoogleConfig) Enabled() bool {
	return c.ClientID != "" && c.ClientSecret != ""
}

type GoogleUser struct {
	Sub           string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
	GivenName     string `json:"given_name"`
	Picture       string `json:"picture"`
}

type googleTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Error       string `json:"error"`
	ErrorDesc   string `json:"error_description"`
}

type GoogleClient struct {
	cfg        GoogleConfig
	httpClient *http.Client
}

func NewGoogleClient(cfg GoogleConfig) *GoogleClient {
	return &GoogleClient{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *GoogleClient) ExchangeCode(ctx context.Context, code, redirectURI string) (string, error) {
	if !c.cfg.Enabled() {
		return "", ErrGoogleNotConfigured
	}

	code = strings.TrimSpace(code)
	redirectURI = strings.TrimSpace(redirectURI)
	if code == "" || redirectURI == "" {
		return "", ErrGoogleExchange
	}

	form := url.Values{}
	form.Set("client_id", c.cfg.ClientID)
	form.Set("client_secret", c.cfg.ClientSecret)
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		"https://oauth2.googleapis.com/token",
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	res, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return "", err
	}

	var token googleTokenResponse
	if err := json.Unmarshal(body, &token); err != nil {
		return "", ErrGoogleExchange
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 || token.AccessToken == "" {
		if token.ErrorDesc != "" {
			return "", fmt.Errorf("%w: %s", ErrGoogleExchange, token.ErrorDesc)
		}
		return "", ErrGoogleExchange
	}

	return token.AccessToken, nil
}

func (c *GoogleClient) FetchUser(ctx context.Context, accessToken string) (*GoogleUser, error) {
	if strings.TrimSpace(accessToken) == "" {
		return nil, ErrGoogleUserFetch
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://openidconnect.googleapis.com/v1/userinfo", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, ErrGoogleUserFetch
	}

	var user GoogleUser
	if err := json.Unmarshal(body, &user); err != nil {
		return nil, ErrGoogleUserFetch
	}
	if user.Sub == "" {
		return nil, ErrGoogleUserFetch
	}
	if user.Email == "" || !user.EmailVerified {
		return nil, ErrGoogleEmailMissing
	}

	return &user, nil
}

func (u *GoogleUser) DisplayName() string {
	if name := strings.TrimSpace(u.Name); name != "" {
		return name
	}
	if name := strings.TrimSpace(u.GivenName); name != "" {
		return name
	}
	if email := strings.TrimSpace(u.Email); email != "" {
		if at := strings.IndexByte(email, '@'); at > 0 {
			return email[:at]
		}
		return email
	}
	return "google"
}
