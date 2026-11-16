package shared

import (
	"strings"
	"testing"

	"hitkeep/config"
)

func TestNativeAndroidPasskeyCapabilityRequiresConfiguredReleaseOrigins(t *testing.T) {
	store := newSharedTestStore(t)
	defer store.Close()
	// Synthetic digest tests configuration, never a real release identity or DAL proof.
	fingerprint := strings.TrimSuffix(strings.Repeat("AB:", 32), ":")
	for _, tc := range []struct {
		name, publicURL, packageName, fingerprints string
		unavailable, want                          bool
	}{
		{name: "default", publicURL: "https://example.test"},
		{name: "package only", publicURL: "https://example.test", packageName: "test.example.release"},
		{name: "invalid certificate", publicURL: "https://example.test", packageName: "test.example.release", fingerprints: "*"},
		{name: "explicit configured origin", publicURL: "https://example.test", packageName: "test.example.release", fingerprints: fingerprint, want: true},
		{name: "helper rejects prefixed configuration", publicURL: "https://example.test/prefix", packageName: "test.example.release", fingerprints: fingerprint},
		{name: "unavailable auth state", publicURL: "https://example.test", packageName: "test.example.release", fingerprints: fingerprint, unavailable: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &Context{Config: &config.Config{PublicURL: tc.publicURL, PasskeyAndroidPackageName: tc.packageName, PasskeyAndroidSHA256Fingerprints: tc.fingerprints}, Store: store, AuthState: NewAuthStateStore()}
			if tc.unavailable {
				c.AuthState = nil
			}
			status := c.NativeAuthStatus()
			if status == nil {
				t.Fatal("HTTPS native descriptor missing")
			}
			if status.NativeAndroidPasskeys != tc.want {
				t.Fatalf("native_android_passkeys=%v, want %v", status.NativeAndroidPasskeys, tc.want)
			}
		})
	}
}
