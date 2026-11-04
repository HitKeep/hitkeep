package api

import "time"

// NativeAuthStart is a first-party human-session continuation, not an OAuth grant.
type NativeAuthStart struct {
	Purpose             string `json:"purpose,omitempty"`
	ReauthAttempt       string `json:"reauth_attempt,omitempty"`
	ClientID            string `json:"client_id"`
	RedirectURI         string `json:"redirect_uri"`
	CodeChallenge       string `json:"code_challenge"`
	CodeChallengeMethod string `json:"code_challenge_method"`
	State               string `json:"state"`
	Intent              string `json:"intent"`
	RememberMe          bool   `json:"remember_me"`
}

type NativeAuthStarted struct {
	AuthorizationURL string    `json:"authorization_url"`
	ExpiresAt        time.Time `json:"expires_at"`
}

type NativeAuthExchange struct {
	ClientID     string `json:"client_id"`
	RedirectURI  string `json:"redirect_uri"`
	Code         string `json:"code"`
	CodeVerifier string `json:"code_verifier"`
}

type NativeAuthStatus struct {
	Enabled                       bool     `json:"enabled"`
	Available                     bool     `json:"available"`
	Issuer                        string   `json:"issuer"`
	StartURL                      string   `json:"start_url"`
	AuthorizeURL                  string   `json:"authorize_url"`
	ExchangeURL                   string   `json:"exchange_url"`
	CodeChallengeMethodsSupported []string `json:"code_challenge_methods_supported"`
	BrowserPasskeys               bool     `json:"browser_passkeys"`
	NativeIOSPasskeys             bool     `json:"native_ios_passkeys"`
	NativeAndroidPasskeys         bool     `json:"native_android_passkeys"`
}

type NativeAuthConsent struct {
	Purpose    string    `json:"purpose,omitempty"`
	Request    string    `json:"request"`
	ClientID   string    `json:"client_id"`
	Issuer     string    `json:"issuer"`
	Email      string    `json:"email"`
	CSRFToken  string    `json:"csrf_token"`
	RememberMe bool      `json:"remember_me"`
	ExpiresAt  time.Time `json:"expires_at"`
}
