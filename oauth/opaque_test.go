package oauth

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ory/fosite"
)

func TestOpaqueCredentialsAreTypedNonrecoverableAndExpire(t *testing.T) {
	strategy := opaqueStrategy{}
	req := fosite.NewRequest()
	req.SetSession(&fosite.DefaultSession{})
	req.GetSession().SetExpiresAt(fosite.AccessToken, time.Now().Add(time.Minute))
	token, signature, err := strategy.GenerateAccessToken(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !IsAccessToken(token) || !IsOAuthToken(token) || len(signature) != 64 || strings.Contains(signature, token) || strings.Contains(token, signature) {
		t.Fatal("opaque token/signature contract failed")
	}
	if signature != strategy.AccessTokenSignature(t.Context(), token) {
		t.Fatal("signature is unstable")
	}
	if err := strategy.ValidateAccessToken(t.Context(), req, token); err != nil {
		t.Fatal(err)
	}
	req.GetSession().SetExpiresAt(fosite.AccessToken, time.Now().Add(-time.Second))
	if err := strategy.ValidateAccessToken(t.Context(), req, token); err == nil {
		t.Fatal("expired token accepted")
	}
	req.GetSession().SetExpiresAt(fosite.AccessToken, time.Now().Add(time.Minute))
	if err := strategy.ValidateAccessToken(t.Context(), req, strings.Replace(token, accessTokenPrefix, refreshTokenPrefix, 1)); err == nil {
		t.Fatal("refresh token accepted as access token")
	}
	if IsOAuthToken("0123456789abcdef") {
		t.Fatal("legacy token misclassified")
	}
	for _, raw := range []string{accessTokenPrefix, refreshTokenPrefix, authorizeCodePrefix} {
		if !IsOAuthToken(raw) {
			t.Fatal("malformed recognized credential would fall back")
		}
	}
}

func TestOpaqueCredentialsDoNotNeedSigningKeyAcrossRestart(t *testing.T) {
	req := fosite.NewRequest()
	req.SetSession(&fosite.DefaultSession{})
	req.GetSession().SetExpiresAt(fosite.RefreshToken, time.Now().Add(time.Minute))
	first := opaqueStrategy{}
	token, signature, err := first.GenerateRefreshToken(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	restarted := opaqueStrategy{}
	if signature != restarted.RefreshTokenSignature(t.Context(), token) {
		t.Fatal("signature changed after restart")
	}
	if err := restarted.ValidateRefreshToken(t.Context(), req, token); err != nil {
		t.Fatal(err)
	}
}
