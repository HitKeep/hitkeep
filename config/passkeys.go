package config

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
)

var androidPackageName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*(\.[A-Za-z][A-Za-z0-9_]*)+$`)

// AndroidPasskeyOrigins derives exact opaque origins from explicitly trusted app-signing certificates.
// Operators must publish matching package/certificate metadata in Digital Asset Links.
func (c *Config) AndroidPasskeyOrigins() ([]string, error) {
	if c == nil {
		return nil, nil
	}
	packageName := strings.TrimSpace(c.PasskeyAndroidPackageName)
	fingerprints := strings.TrimSpace(c.PasskeyAndroidSHA256Fingerprints)
	if packageName == "" && fingerprints == "" {
		return nil, nil
	}
	if !androidPackageName.MatchString(packageName) || fingerprints == "" {
		return nil, fmt.Errorf("passkey configuration: Android passkeys require an explicit package name and release SHA-256 certificate fingerprints")
	}
	publicURL, err := url.Parse(c.PublicURL)
	if err != nil || publicURL.Scheme != "https" || publicURL.Hostname() == "" || publicURL.User != nil || publicURL.RawQuery != "" || publicURL.Fragment != "" || (publicURL.Path != "" && publicURL.Path != "/") {
		return nil, fmt.Errorf("passkey configuration: Android passkeys require a canonical HTTPS public URL")
	}
	var origins []string
	for fingerprint := range strings.SplitSeq(fingerprints, ",") {
		parts := strings.Split(strings.TrimSpace(fingerprint), ":")
		if len(parts) != 32 {
			return nil, fmt.Errorf("passkey configuration: Android passkey certificate fingerprints must contain 32 colon-separated hexadecimal bytes")
		}
		for _, part := range parts {
			if len(part) != 2 {
				return nil, fmt.Errorf("passkey configuration: Android passkey certificate fingerprints must contain 32 colon-separated hexadecimal bytes")
			}
		}
		digest, err := hex.DecodeString(strings.Join(parts, ""))
		if err != nil {
			return nil, fmt.Errorf("passkey configuration: Android passkey certificate fingerprints must contain 32 colon-separated hexadecimal bytes")
		}
		origin := "android:apk-key-hash:" + base64.RawURLEncoding.EncodeToString(digest)
		if !slices.Contains(origins, origin) {
			origins = append(origins, origin)
		}
	}
	return origins, nil
}
