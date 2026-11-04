package oauth

import (
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/golang-jwt/jwt/v5"

	"hitkeep/database"
)

func TestAgentAssertionsAreOwnPurposeBoundAndReusableUntilExpiry(t *testing.T) {
	f := newProviderFixture(t)
	grant := &database.OAuthGrant{ID: uuid.NewV7(), ExpiresAt: time.Now().UTC().Add(10 * time.Minute)}
	raw, expires, err := f.p.signAgentAssertion(grant)
	if err != nil || expires.After(grant.ExpiresAt) {
		t.Fatalf("sign assertion: %v, expiry %s", err, expires)
	}
	for range 2 {
		id, err := f.p.parseAgentAssertion(raw)
		if err != nil || id != grant.ID {
			t.Fatalf("own assertion refresh: %s %v", id, err)
		}
	}
	other := &Provider{urls: issuerURLs{Issuer: "https://other.example"}, browserKey: f.p.browserKey}
	if _, err := other.parseAgentAssertion(raw); err == nil {
		t.Fatal("cross-issuer assertion accepted")
	}
	pieces := strings.Split(raw, ".")
	pieces[2] = strings.Repeat("A", len(pieces[2]))
	if _, err := f.p.parseAgentAssertion(strings.Join(pieces, ".")); err == nil {
		t.Fatal("tampered signature accepted")
	}
}

func TestAgentAssertionRejectsWrongAlgorithmTypeClaimsAndKey(t *testing.T) {
	f := newProviderFixture(t)
	for _, fault := range []string{"algorithm", "type", "issuer", "audience", "extra-audience", "expiry", "missing-expiry", "future-issued", "missing-issued", "subject", "key", "browser-key"} {
		t.Run(fault, func(t *testing.T) {
			now := time.Now().UTC()
			claims := jwt.RegisteredClaims{Issuer: f.p.Issuer(), Subject: uuid.NewV7().String(), Audience: jwt.ClaimStrings{f.p.Issuer()}, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)), ID: uuid.NewV7().String()}
			method := jwt.SigningMethodHS256
			key := f.p.agentAssertionKey()
			switch fault {
			case "algorithm":
				method = jwt.SigningMethodHS384
			case "issuer":
				claims.Issuer = "https://other.example"
			case "audience":
				claims.Audience = jwt.ClaimStrings{f.p.Resource("api")}
			case "extra-audience":
				claims.Audience = append(claims.Audience, "https://other.example")
			case "expiry":
				claims.ExpiresAt = jwt.NewNumericDate(now.Add(-time.Minute))
			case "missing-expiry":
				claims.ExpiresAt = nil
			case "future-issued":
				claims.IssuedAt = jwt.NewNumericDate(now.Add(time.Hour))
			case "missing-issued":
				claims.IssuedAt = nil
			case "subject":
				claims.Subject = "not-a-registration"
			case "key":
				key = []byte(strings.Repeat("x", 32))
			case "browser-key":
				key = f.p.browserKey
			}
			token := jwt.NewWithClaims(method, claims)
			token.Header["typ"] = "oauth-id-jag+jwt"
			if fault == "type" {
				token.Header["typ"] = "JWT"
			}
			raw, err := token.SignedString(key)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.p.parseAgentAssertion(raw); err == nil {
				t.Fatal("invalid assertion accepted")
			}
		})
	}
}
