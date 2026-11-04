package auth

import (
	"testing"
	"time"
	"uuid"
)

func TestEmbedCredentialAndLaunchHaveSeparateAuthority(t *testing.T) {
	site, grant := uuid.NewV7(), uuid.NewV7()
	expires := time.Now().Add(5 * time.Minute)
	token, err := GenerateEmbedToken("embed-secret", "https://hitkeep.example", grant, site, "https://wordpress.example", expires)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := ValidateEmbedToken(token, "embed-secret", "https://hitkeep.example")
	if err != nil || claims.SiteID != site || claims.GrantID != grant {
		t.Fatalf("token claims: %+v %v", claims, err)
	}
	launch, err := GenerateEmbedLaunch("embed-secret", "https://hitkeep.example", site, "https://wordpress.example", expires)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateEmbedToken(launch, "embed-secret", "https://hitkeep.example"); err == nil {
		t.Fatal("launch can authenticate API")
	}
	if _, err := ValidateEmbedLaunch(token, "embed-secret", "https://hitkeep.example"); err == nil {
		t.Fatal("API token can launch iframe")
	}
	if _, err := ValidateToken(token, "embed-secret", "https://hitkeep.example"); err == nil {
		t.Fatal("embed token can authenticate dashboard")
	}
	if _, err := ValidateEmbedToken(token, "wrong", "https://hitkeep.example"); err == nil {
		t.Fatal("wrong signing key accepted")
	}
	if _, err := ValidateEmbedToken(token, "embed-secret", "https://other.example"); err == nil {
		t.Fatal("wrong issuer accepted")
	}
	expired, err := GenerateEmbedToken("embed-secret", "https://hitkeep.example", grant, site, "https://wordpress.example", time.Now().Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateEmbedToken(expired, "embed-secret", "https://hitkeep.example"); err == nil {
		t.Fatal("expired token accepted")
	}
}

func TestEmbedParentOrigin(t *testing.T) {
	for _, origin := range []string{"https://wordpress.example", "https://wordpress.example:8443", "http://localhost:8888", "http://127.0.0.1:8080", "http://[::1]:8080"} {
		if !ValidEmbedParentOrigin(origin) {
			t.Errorf("valid origin rejected: %q", origin)
		}
	}
	for _, origin := range []string{"", "null", "*", "http://wordpress.example", "https://wordpress.example/", "https://wordpress.example/wp-admin", "https://wordpress.example?x=1", "https://user@wordpress.example", "https://wordpress.example#x", "https://wordpress.example\r\nX-Test: x", "https://wordpress.example:bad", "https://wordpress.example:", "https://wordpress.example:443", "https://wordpress.example:08443", "HTTPS://WORDPRESS.EXAMPLE"} {
		if ValidEmbedParentOrigin(origin) {
			t.Errorf("unsafe origin accepted: %q", origin)
		}
	}
}
