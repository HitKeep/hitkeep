package auth

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
	"uuid"

	"github.com/golang-jwt/jwt/v5"
)

const embedTokenPrefix = "hk_embed_" // #nosec G101 -- Public credential-kind marker, not a secret.
const EmbedTokenLifetime = 5 * time.Minute

type EmbedClaims struct {
	GrantID      uuid.UUID `json:"grant_id,omitzero"`
	SiteID       uuid.UUID `json:"site_id"`
	ParentOrigin string    `json:"parent_origin"`
	jwt.RegisteredClaims
}

func IsEmbedToken(token string) bool { return strings.HasPrefix(token, embedTokenPrefix) }

// ValidEmbedParentOrigin requires an exact origin suitable for CSP and postMessage.
func ValidEmbedParentOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.ForceQuery || u.Opaque != "" || origin != u.Scheme+"://"+u.Host || u.Host != strings.ToLower(u.Host) || strings.ContainsAny(u.Host, "* ;'\"\\%\t\r\n") || strings.HasSuffix(u.Host, ":") {
		return false
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 || strconv.Itoa(n) != port || (u.Scheme == "https" && port == "443") || (u.Scheme == "http" && port == "80") {
			return false
		}
	}
	if u.Scheme == "https" {
		return true
	}
	ip := net.ParseIP(u.Hostname())
	return u.Scheme == "http" && (u.Hostname() == "localhost" || (ip != nil && ip.IsLoopback()))
}

func GenerateEmbedToken(secret, issuer string, grantID, siteID uuid.UUID, parentOrigin string, expiresAt time.Time) (string, error) {
	if grantID == uuid.Nil() {
		return "", fmt.Errorf("missing embed grant")
	}
	return signEmbedClaims(secret, issuer, grantID, siteID, parentOrigin, expiresAt, "/embed/api", embedTokenPrefix)
}

// GenerateEmbedLaunch signs public framing metadata; it carries no API authority.
func GenerateEmbedLaunch(secret, issuer string, siteID uuid.UUID, parentOrigin string, expiresAt time.Time) (string, error) {
	return signEmbedClaims(secret, issuer, uuid.Nil(), siteID, parentOrigin, expiresAt, "/embed/launch", "")
}

func signEmbedClaims(secret, issuer string, grantID, siteID uuid.UUID, parentOrigin string, expiresAt time.Time, audience, prefix string) (string, error) {
	if secret == "" || issuer == "" || siteID == uuid.Nil() || !ValidEmbedParentOrigin(parentOrigin) {
		return "", fmt.Errorf("invalid embed configuration")
	}
	claims := EmbedClaims{GrantID: grantID, SiteID: siteID, ParentOrigin: parentOrigin, ExpiresAt: jwt.NewNumericDate(expiresAt), IssuedAt: jwt.NewNumericDate(time.Now()), Issuer: issuer, Audience: jwt.ClaimStrings{issuer + audience}}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		return "", err
	}
	return prefix + signed, nil
}

func ValidateEmbedToken(token, secret, issuer string) (*EmbedClaims, error) {
	body, ok := strings.CutPrefix(token, embedTokenPrefix)
	if !ok {
		return nil, fmt.Errorf("invalid embed credential")
	}
	claims, err := validateEmbedClaims(body, secret, issuer, "/embed/api")
	if err != nil {
		return nil, err
	}
	if claims.GrantID == uuid.Nil() {
		return nil, fmt.Errorf("missing embed grant")
	}
	return claims, nil
}

func ValidateEmbedLaunch(token, secret, issuer string) (*EmbedClaims, error) {
	claims, err := validateEmbedClaims(token, secret, issuer, "/embed/launch")
	if err != nil {
		return nil, err
	}
	if claims.GrantID != uuid.Nil() {
		return nil, fmt.Errorf("invalid embed launch")
	}
	return claims, nil
}

func validateEmbedClaims(token, secret, issuer, audience string) (*EmbedClaims, error) {
	if secret == "" || issuer == "" {
		return nil, fmt.Errorf("invalid embed configuration")
	}
	parsed, err := jwt.ParseWithClaims(token, &EmbedClaims{}, func(*jwt.Token) (any, error) { return []byte(secret), nil }, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithIssuer(issuer), jwt.WithAudience(issuer+audience), jwt.WithExpirationRequired(), jwt.WithIssuedAt())
	if err != nil {
		return nil, err
	}
	claims, ok := parsed.Claims.(*EmbedClaims)
	if !ok || !parsed.Valid || claims.SiteID == uuid.Nil() || !ValidEmbedParentOrigin(claims.ParentOrigin) {
		return nil, fmt.Errorf("invalid embed claims")
	}
	return claims, nil
}
