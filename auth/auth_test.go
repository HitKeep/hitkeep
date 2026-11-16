package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func TestValidateTokenClaimsAcceptsOnlyHS256(t *testing.T) {
	const (
		secret = "test-secret"
		issuer = "https://example.test"
	)

	userID := uuid.New()
	hs256Token, _, err := GenerateTokenWithDuration(secret, issuer, userID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := ValidateTokenClaims(hs256Token, secret, issuer)
	if err != nil {
		t.Fatalf("ValidateTokenClaims() error = %v", err)
	}
	if claims == nil || claims.UserID != userID {
		t.Fatalf("ValidateTokenClaims() claims = %#v, want user %s", claims, userID)
	}

	for _, method := range []*jwt.SigningMethodHMAC{jwt.SigningMethodHS384, jwt.SigningMethodHS512} {
		t.Run(method.Alg(), func(t *testing.T) {
			now := time.Now()
			token := jwt.NewWithClaims(method, Claims{
				UserID:    userID,
				ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
				IssuedAt:  jwt.NewNumericDate(now),
				Issuer:    issuer,
				Audience:  jwt.ClaimStrings{issuer},
			})
			tokenString, err := token.SignedString([]byte(secret))
			if err != nil {
				t.Fatal(err)
			}

			claims, err := ValidateTokenClaims(tokenString, secret, issuer)
			if err == nil {
				t.Fatal("ValidateTokenClaims() error = nil, want rejected signing method")
			}
			if claims != nil {
				t.Fatalf("ValidateTokenClaims() claims = %#v, want nil", claims)
			}
			if strings.Contains(err.Error(), tokenString) {
				t.Fatal("ValidateTokenClaims() error leaked token")
			}
		})
	}
}

func TestFreshOrdinarySessionsHaveDistinctIDs(t *testing.T) {
	id := uuid.New()
	first, err := GenerateToken("test-secret", "https://example.test/prefix", id)
	if err != nil {
		t.Fatal(err)
	}
	second, err := GenerateToken("test-secret", "https://example.test/prefix", id)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("fresh sessions reused the same JWT")
	}
	claims, err := ValidateTokenClaims(second, "test-secret", "https://example.test/prefix")
	if err != nil || claims.ID == "" || claims.UserID != id {
		t.Fatalf("claims=%+v err=%v", claims, err)
	}
}

func TestCookieSecure(t *testing.T) {
	for _, tc := range []struct {
		url    string
		secure bool
	}{
		{"https://example.test/prefix", true}, {"HTTPS://EXAMPLE.test:443/prefix", true}, {" HtTpS://example.test/prefix ", true}, {"http://localhost:8080", false}, {"", false},
	} {
		if got := CookieSecure(tc.url); got != tc.secure {
			t.Errorf("CookieSecure(%q)=%v, want %v", tc.url, got, tc.secure)
		}
	}
}
