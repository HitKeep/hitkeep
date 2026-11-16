package config

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/spf13/afero"
)

func TestAndroidPasskeyConfiguration(t *testing.T) {
	fingerprint := strings.TrimSuffix(strings.Repeat("AB:", 32), ":")
	for _, tc := range []struct {
		name, packageName, fingerprints, publicURL string
		wantOrigins                                int
		wantError                                  bool
	}{
		{name: "disabled"},
		{name: "release", packageName: "test.example.release", fingerprints: fingerprint, publicURL: "https://example.test", wantOrigins: 1},
		{name: "rotation", packageName: "test.example.release", fingerprints: fingerprint + "," + strings.ReplaceAll(fingerprint, "AB", "CD"), publicURL: "https://example.test", wantOrigins: 2},
		{name: "certificate only", fingerprints: fingerprint, publicURL: "https://example.test", wantError: true},
		{name: "package only", packageName: "test.example.release", publicURL: "https://example.test", wantError: true},
		{name: "wildcard package", packageName: "test.example.*", fingerprints: fingerprint, publicURL: "https://example.test", wantError: true},
		{name: "wildcard certificate", packageName: "test.example.release", fingerprints: "*", publicURL: "https://example.test", wantError: true},
		{name: "short certificate", packageName: "test.example.release", fingerprints: "AB:CD", publicURL: "https://example.test", wantError: true},
		{name: "nonhex certificate", packageName: "test.example.release", fingerprints: strings.ReplaceAll(fingerprint, "AB", "GG"), publicURL: "https://example.test", wantError: true},
		{name: "empty entry", packageName: "test.example.release", fingerprints: fingerprint + ",", publicURL: "https://example.test", wantError: true},
		{name: "insecure RP", packageName: "test.example.release", fingerprints: fingerprint, publicURL: "http://example.test", wantError: true},
		{name: "URL credentials", packageName: "test.example.release", fingerprints: fingerprint, publicURL: "https://user@example.test", wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conf := &Config{PublicURL: tc.publicURL, PasskeyAndroidPackageName: tc.packageName, PasskeyAndroidSHA256Fingerprints: tc.fingerprints}
			origins, err := conf.AndroidPasskeyOrigins()
			if (err != nil) != tc.wantError || len(origins) != tc.wantOrigins {
				t.Fatalf("origins=%v error=%v", origins, err)
			}
			if tc.wantOrigins > 0 && origins[0] != "android:apk-key-hash:"+base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0xab}, 32)) {
				t.Fatal("certificate digest was not encoded as an exact Android origin")
			}
		})
	}
}

func TestLoadAndroidPasskeyConfiguration(t *testing.T) {
	fingerprint := strings.TrimSuffix(strings.Repeat("AB:", 32), ":")
	t.Setenv("HITKEEP_PASSKEY_ANDROID_PACKAGE_NAME", "test.example.release")
	t.Setenv("HITKEEP_PASSKEY_ANDROID_SHA256_FINGERPRINTS", fingerprint)
	t.Setenv("HITKEEP_PUBLIC_URL", "https://example.test")
	conf, err := LoadArgs(afero.NewMemMapFs(), nil, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if origins, err := conf.AndroidPasskeyOrigins(); err != nil || len(origins) != 1 {
		t.Fatalf("environment configuration not loaded: %v, %v", origins, err)
	}
	t.Setenv("HITKEEP_PASSKEY_ANDROID_SHA256_FINGERPRINTS", "*")
	if _, err := LoadArgs(afero.NewMemMapFs(), nil, "", nil); err == nil {
		t.Fatal("invalid trust configuration must reject application startup")
	}
}

func TestLoadAndroidPasskeySources(t *testing.T) {
	fingerprint := strings.TrimSuffix(strings.Repeat("AB:", 32), ":")
	for _, source := range []string{"default", "environment", "file", "flag", "precedence"} {
		t.Run(source, func(t *testing.T) {
			t.Setenv("HITKEEP_PASSKEY_ANDROID_PACKAGE_NAME", "")
			t.Setenv("HITKEEP_PASSKEY_ANDROID_SHA256_FINGERPRINTS", "")
			t.Setenv("HITKEEP_PUBLIC_URL", "https://example.test")
			fs := afero.NewMemMapFs()
			var args []string
			var file, wantPackage string
			if source == "file" || source == "precedence" {
				file = "/config.yaml"
				wantPackage = "test.example.file"
				contents := "passkey-android-package-name: " + wantPackage + "\npasskey-android-sha256-fingerprints: " + fingerprint + "\n"
				if err := afero.WriteFile(fs, file, []byte(contents), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if source == "environment" || source == "precedence" {
				wantPackage = "test.example.environment"
				t.Setenv("HITKEEP_PASSKEY_ANDROID_PACKAGE_NAME", wantPackage)
				t.Setenv("HITKEEP_PASSKEY_ANDROID_SHA256_FINGERPRINTS", fingerprint)
			}
			if source == "flag" || source == "precedence" {
				wantPackage = "test.example.flag"
				args = []string{"--passkey-android-package-name=" + wantPackage, "--passkey-android-sha256-fingerprints=" + fingerprint}
			}
			conf, err := LoadArgs(fs, args, file, nil)
			if err != nil {
				t.Fatal(err)
			}
			wantFingerprint := fingerprint
			if source == "default" {
				wantFingerprint = ""
			}
			if conf.PasskeyAndroidPackageName != wantPackage || conf.PasskeyAndroidSHA256Fingerprints != wantFingerprint {
				t.Fatalf("%s did not preserve the paired trust settings", source)
			}
		})
	}
}
