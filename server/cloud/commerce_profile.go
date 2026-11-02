//go:build billing

package cloud

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	json "hitkeep/jsonapi"
	"hitkeep/webhooks"
)

// Fetch only the public profile; it is capability metadata, never billing authority.
func fetchCommerceProfile(ctx context.Context, header string) (int, error) {
	raw, ok := strings.CutPrefix(strings.TrimSpace(header), "profile=")
	if !ok || len(raw) > 2048 {
		return http.StatusBadRequest, errors.New("UCP-Agent profile is required")
	}
	if len(raw) < 2 || raw[0] != '"' || raw[len(raw)-1] != '"' {
		return http.StatusBadRequest, errors.New("Invalid UCP-Agent profile")
	}
	profile := raw[1 : len(raw)-1]
	for _, character := range profile {
		if character < 0x21 || character > 0x7e || character == '"' || character == '\\' {
			return http.StatusBadRequest, errors.New("Invalid UCP-Agent profile")
		}
	}
	target, err := url.Parse(profile)
	if err != nil || target.Scheme != "https" || target.Hostname() == "" || target.User != nil || target.Path != "/.well-known/ucp" || target.RawQuery != "" || target.ForceQuery || strings.Contains(profile, "#") || (target.Port() != "" && target.Port() != "443") {
		return http.StatusBadRequest, errors.New("UCP profile must be public HTTPS at /.well-known/ucp")
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13}, DisableKeepAlives: true, MaxResponseHeaderBytes: 16 << 10, TLSHandshakeTimeout: 3 * time.Second, ResponseHeaderTimeout: 3 * time.Second,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
			if err != nil || len(ips) == 0 {
				return nil, errors.New("Profile DNS unavailable")
			}
			for _, ip := range ips {
				if ip.Zone() != "" || !webhooks.AddressAllowed(ip) {
					return nil, errors.New("Profile destination is not public")
				}
			}
			return (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ips[0].Unmap().String(), port))
		},
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Timeout: 5 * time.Second, Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return http.StatusBadRequest, errors.New("Invalid profile request")
	}
	req.Header.Set("Accept", "application/json")
	response, err := client.Do(req) // #nosec G704 -- public IPs are checked and pinned in DialContext; HTTPS, no proxies or redirects.
	if err != nil {
		return http.StatusFailedDependency, errors.New("UCP profile unavailable")
	}
	defer response.Body.Close()
	media, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || media != "application/json" || response.StatusCode != http.StatusOK || response.Header.Get("Content-Encoding") != "" {
		return http.StatusUnprocessableEntity, errors.New("UCP profile must return JSON without redirects")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 64<<10+1))
	if err != nil || len(body) > 64<<10 {
		return http.StatusUnprocessableEntity, errors.New("UCP profile exceeds bounded response")
	}
	return validateCommerceProfile(body)
}

func validateCommerceProfile(body []byte) (int, error) {
	var profile struct {
		UCP struct {
			Version      string `json:"version"`
			Capabilities map[string][]struct {
				Version string `json:"version"`
			} `json:"capabilities"`
		} `json:"ucp"`
	}
	if err := json.Unmarshal(body, &profile); err != nil || profile.UCP.Version != ucpVersion {
		return http.StatusUnprocessableEntity, errors.New("Unsupported UCP profile version")
	}
	for _, capability := range profile.UCP.Capabilities["dev.ucp.shopping.checkout"] {
		if capability.Version == ucpVersion {
			return 0, nil
		}
	}
	return http.StatusUnprocessableEntity, errors.New("UCP checkout capability version is not supported")
}
