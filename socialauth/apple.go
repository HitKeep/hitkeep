package socialauth

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	json "hitkeep/jsonapi"
	"hitkeep/sso"
)

const AppleManualRevocationURL = "https://account.apple.com/account/manage"

// Apple returns email_verified as either a JSON boolean or its lowercase string form.
// Keep immutable subject authentication separate from email eligibility for new links.
func appleIdentity(token *oidc.IDToken, claims map[string]any) (Identity, error) {
	if token == nil || strings.TrimSpace(token.Subject) == "" {
		return Identity{}, ErrIdentityClaims
	}
	verified := claims["email_verified"] == true || claims["email_verified"] == "true"
	email, _, err := sso.NormalizeEmail(stringClaim(claims, "email"))
	if err != nil {
		email = ""
		verified = false
	}
	return Identity{Provider: ProviderApple, Subject: token.Subject, Email: email, EmailVerified: verified}, nil
}

type appleCredential struct {
	ClientID     string `json:"client_id"`
	Subject      string `json:"subject"`
	RefreshToken string `json:"refresh_token"`
}

// SealAppleRefreshToken authenticates the immutable identity and audience with the token.
func SealAppleRefreshToken(key, clientID, subject, refreshToken string) (string, error) {
	if len(strings.TrimSpace(key)) < 32 || clientID == "" || subject == "" || refreshToken == "" {
		return "", errors.New("Apple credential unavailable")
	}
	box, err := sso.NewPurposeSecretBox(key, "hitkeep/apple/refresh-token/v1")
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(appleCredential{ClientID: clientID, Subject: subject, RefreshToken: refreshToken})
	if err != nil {
		return "", errors.New("Apple credential unavailable")
	}
	return box.Seal(string(payload))
}

func OpenAppleRefreshToken(key, clientID, subject, ciphertext string) (string, error) {
	if len(strings.TrimSpace(key)) < 32 {
		return "", errors.New("Apple credential unavailable")
	}
	box, err := sso.NewPurposeSecretBox(key, "hitkeep/apple/refresh-token/v1")
	if err != nil {
		return "", err
	}
	plaintext, err := box.Open(ciphertext)
	if err != nil {
		return "", errors.New("Apple credential unavailable")
	}
	var credential appleCredential
	if json.Unmarshal([]byte(plaintext), &credential) != nil || credential.ClientID != clientID || credential.Subject != subject || credential.RefreshToken == "" {
		return "", errors.New("Apple credential unavailable")
	}
	return credential.RefreshToken, nil
}

// RevokeApple makes one bounded attempt; callers must report manual fallback on failure.
func (c *Client) RevokeApple(ctx context.Context, cfg ProviderConfig, clientID, refreshToken string) error {
	if cfg.Provider != ProviderApple || cfg.ClientID != clientID || cfg.ClientSecret == "" || refreshToken == "" || cfg.RevokeURL == "" {
		return ErrNotConfigured
	}
	ctx, cancel := context.WithTimeout(ctx, providerTimeout)
	defer cancel()
	values := url.Values{"client_id": {clientID}, "client_secret": {cfg.ClientSecret}, "token": {refreshToken}, "token_type_hint": {"refresh_token"}}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.RevokeURL, strings.NewReader(values.Encode()))
	if err != nil {
		return ErrProviderUnavailable
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := c.httpClient
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return ErrProviderUnavailable
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxProviderBody))
	if response.StatusCode != http.StatusOK {
		return ErrProviderUnavailable
	}
	return nil
}
