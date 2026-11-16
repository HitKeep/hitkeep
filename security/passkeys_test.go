package security_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	webauthnlib "github.com/go-webauthn/webauthn/webauthn"

	"hitkeep/config"
	"hitkeep/security"
	"hitkeep/testutil"
)

func TestAndroidPasskeySignedAssertionTrustBoundary(t *testing.T) {
	fingerprint := strings.TrimSuffix(strings.Repeat("AB:", 32), ":")
	androidOrigin := "android:apk-key-hash:" + base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0xab}, 32))
	wrongOrigin := "android:apk-key-hash:" + base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0xcd}, 32))
	for _, tc := range []struct {
		name, origin, rpID                                        string
		configured, uv, badChallenge, badSignature, expired, want bool
	}{
		{name: "configured release", origin: androidOrigin, rpID: "example.test", configured: true, uv: true, want: true},
		{name: "default disabled", origin: androidOrigin, rpID: "example.test", uv: true},
		{name: "different certificate", origin: wrongOrigin, rpID: "example.test", configured: true, uv: true},
		{name: "web preserved", origin: "https://example.test", rpID: "example.test", configured: true, uv: true, want: true},
		{name: "other web origin", origin: "https://other.test", rpID: "example.test", configured: true, uv: true},
		{name: "wrong RP", origin: androidOrigin, rpID: "other.test", configured: true, uv: true},
		{name: "missing UV", origin: androidOrigin, rpID: "example.test", configured: true},
		{name: "wrong challenge", origin: androidOrigin, rpID: "example.test", configured: true, uv: true, badChallenge: true},
		{name: "invalid signature", origin: androidOrigin, rpID: "example.test", configured: true, uv: true, badSignature: true},
		{name: "expired challenge", origin: androidOrigin, rpID: "example.test", configured: true, uv: true, expired: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conf := &config.Config{PublicURL: "https://example.test"}
			if tc.configured {
				conf.PasskeyAndroidPackageName = "test.example.release"
				conf.PasskeyAndroidSHA256Fingerprints = fingerprint
			}
			webAuthn, err := security.NewWebAuthn(conf, nil)
			if err != nil {
				t.Fatal(err)
			}
			fixture, err := testutil.NewPasskeyFixture()
			if err != nil {
				t.Fatal(err)
			}
			user := &security.WebAuthnUser{UserID: [16]byte{1}, Name: "user@example.test", Credentials: []webauthnlib.Credential{fixture.Credential()}}
			options, session, err := webAuthn.BeginLogin(user)
			if err != nil {
				t.Fatal(err)
			}
			if options.Response.UserVerification != protocol.VerificationRequired || options.Response.RelyingPartyID != "example.test" {
				t.Fatal("RP/UV requirements changed")
			}
			challenge := options.Response.Challenge
			if tc.badChallenge {
				challenge = protocol.URLEncodedBase64([]byte("different challenge"))
			}
			assertion, err := fixture.AssertionResponse(challenge, tc.origin, tc.rpID, user.WebAuthnID(), 1, tc.uv)
			if err != nil {
				t.Fatal(err)
			}
			if tc.badSignature {
				assertion.AssertionResponse.Signature[0] ^= 0xff
			}
			if tc.expired {
				session.Expires = time.Now().Add(-time.Minute)
			}
			body, err := json.Marshal(assertion)
			if err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequest(http.MethodPost, "https://example.test/api/auth/passkey/login/finish", bytes.NewReader(body))
			_, err = webAuthn.FinishLogin(user, *session, r)
			if (err == nil) != tc.want {
				t.Fatalf("signed assertion accepted=%v, want=%v: %v", err == nil, tc.want, err)
			}
		})
	}
}

func TestWebAuthnRejectsInvalidAndroidConfiguration(t *testing.T) {
	conf := &config.Config{PublicURL: "https://example.test", PasskeyAndroidPackageName: "test.example.release", PasskeyAndroidSHA256Fingerprints: "*"}
	if webAuthn, err := security.NewWebAuthn(conf, nil); err == nil || webAuthn != nil {
		t.Fatal("invalid Android trust configuration must not construct WebAuthn")
	}
}
