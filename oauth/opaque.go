package oauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"time"

	"github.com/ory/fosite"
)

const (
	accessTokenPrefix   = "hk_oauth_at_" // #nosec G101 -- Public credential-kind marker; each credential body uses 32 crypto/rand bytes.
	refreshTokenPrefix  = "hk_oauth_rt_" // #nosec G101 -- Public credential-kind marker; each credential body uses 32 crypto/rand bytes.
	authorizeCodePrefix = "hk_oauth_ac_" // #nosec G101 -- Public credential-kind marker; each credential body uses 32 crypto/rand bytes.
)

// IsAccessToken recognizes the credential class even when its body is malformed.
func IsAccessToken(token string) bool { return strings.HasPrefix(token, accessTokenPrefix) }

// IsOAuthToken also prevents refresh/code/unknown OAuth prefixes falling back to a browser session.
func IsOAuthToken(token string) bool { return strings.HasPrefix(token, "hk_oauth_") }

// The database lookup authenticates SHA-256(entire random credential); no recoverable secret is stored.
type opaqueStrategy struct{}

func tokenSignature(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
func (opaqueStrategy) AccessTokenSignature(_ context.Context, token string) string {
	return tokenSignature(token)
}
func (opaqueStrategy) RefreshTokenSignature(_ context.Context, token string) string {
	return tokenSignature(token)
}
func (opaqueStrategy) AuthorizeCodeSignature(_ context.Context, token string) string {
	return tokenSignature(token)
}
func newOpaqueToken(prefix string) (string, string, error) {
	var entropy [32]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return "", "", err
	}
	token := prefix + base64.RawURLEncoding.EncodeToString(entropy[:])
	return token, tokenSignature(token), nil
}
func (opaqueStrategy) GenerateAccessToken(context.Context, fosite.Requester) (string, string, error) {
	return newOpaqueToken(accessTokenPrefix)
}
func (opaqueStrategy) GenerateRefreshToken(context.Context, fosite.Requester) (string, string, error) {
	return newOpaqueToken(refreshTokenPrefix)
}
func (opaqueStrategy) GenerateAuthorizeCode(context.Context, fosite.Requester) (string, string, error) {
	return newOpaqueToken(authorizeCodePrefix)
}
func validOpaqueToken(token, prefix string) bool {
	body, ok := strings.CutPrefix(token, prefix)
	if !ok || len(body) != 43 {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(body)
	return err == nil && len(decoded) == 32
}
func validateOpaqueToken(token, prefix string, request fosite.Requester, kind fosite.TokenType) error {
	if !validOpaqueToken(token, prefix) || request == nil || request.GetSession() == nil {
		return fosite.ErrInvalidTokenFormat
	}
	expires := request.GetSession().GetExpiresAt(kind)
	if expires.IsZero() || !time.Now().UTC().Before(expires) {
		return fosite.ErrTokenExpired
	}
	return nil
}
func (opaqueStrategy) ValidateAccessToken(_ context.Context, request fosite.Requester, token string) error {
	return validateOpaqueToken(token, accessTokenPrefix, request, fosite.AccessToken)
}
func (opaqueStrategy) ValidateRefreshToken(_ context.Context, request fosite.Requester, token string) error {
	return validateOpaqueToken(token, refreshTokenPrefix, request, fosite.RefreshToken)
}
func (opaqueStrategy) ValidateAuthorizeCode(_ context.Context, request fosite.Requester, token string) error {
	return validateOpaqueToken(token, authorizeCodePrefix, request, fosite.AuthorizeCode)
}
