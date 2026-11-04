package oauth

import (
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"time"
	"uuid"

	"github.com/golang-jwt/jwt/v5"

	"hitkeep/database"
)

const agentAssertionLifetime = time.Hour

func (p *Provider) agentAssertionKey() []byte {
	mac := hmac.New(sha256.New, p.browserKey)
	mac.Write([]byte("hitkeep/oauth/agent-assertion/v1\x00" + p.urls.Issuer))
	return mac.Sum(nil)
}

func (p *Provider) signAgentAssertion(grant *database.OAuthGrant) (string, time.Time, error) {
	now := time.Now().UTC()
	expires := now.Add(agentAssertionLifetime)
	if grant.ExpiresAt.Before(expires) {
		expires = grant.ExpiresAt
	}
	expires = expires.Truncate(time.Second)
	if grant.ID == uuid.Nil() || !now.Before(expires) {
		return "", time.Time{}, ErrInvalidToken
	}
	claims := jwt.RegisteredClaims{Issuer: p.urls.Issuer, Subject: grant.ID.String(), Audience: jwt.ClaimStrings{p.urls.Issuer}, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(expires), ID: uuid.NewV7().String()}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	token.Header["typ"] = "oauth-id-jag+jwt"
	raw, err := token.SignedString(p.agentAssertionKey())
	return raw, expires, err
}

// Own assertions are reusable refresh credentials, not incoming provider ID-JAGs.
func (p *Provider) parseAgentAssertion(raw string) (uuid.UUID, error) {
	if len(raw) == 0 || len(raw) > 4096 {
		return uuid.Nil(), ErrInvalidToken
	}
	var claims jwt.RegisteredClaims
	token, err := jwt.ParseWithClaims(raw, &claims, func(token *jwt.Token) (any, error) {
		if token.Header["typ"] != "oauth-id-jag+jwt" || token.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("invalid agent assertion type")
		}
		return p.agentAssertionKey(), nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer(p.urls.Issuer), jwt.WithAudience(p.urls.Issuer), jwt.WithExpirationRequired(), jwt.WithIssuedAt())
	if err != nil || token == nil || !token.Valid || claims.IssuedAt == nil || claims.ID == "" || len(claims.Audience) != 1 || claims.Audience[0] != p.urls.Issuer {
		return uuid.Nil(), ErrInvalidToken
	}
	id, err := uuid.Parse(claims.Subject)
	if err != nil || id == uuid.Nil() {
		return uuid.Nil(), ErrInvalidToken
	}
	return id, nil
}
