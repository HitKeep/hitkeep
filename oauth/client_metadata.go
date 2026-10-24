package oauth

import (
	"context"
	"encoding/json/v2"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"uuid"
)

const (
	metadataMaxBytes   = 64 << 10
	metadataMaxCache   = 128
	metadataMaxFetches = 8
	metadataTimeout    = 5 * time.Second
)

var errMetadataBusy = errors.New("OAuth client metadata fetch capacity exhausted")

// ClientMetadata is a validated public-client snapshot; remote key/logo URLs are never fetched.
type ClientMetadata struct {
	RegistrationTeamID  uuid.UUID `json:"registration_team_id,omitzero"`
	RegistrationVersion string    `json:"registration_version,omitempty"`
	ID                  string    `json:"client_id"`
	Name                string    `json:"client_name"`
	URI                 string    `json:"client_uri,omitempty"`
	RedirectURIs        []string  `json:"redirect_uris"`
	GrantTypes          []string  `json:"grant_types,omitempty"`
	ResponseTypes       []string  `json:"response_types,omitempty"`
	AuthMethod          string    `json:"token_endpoint_auth_method,omitempty"`
	AuthMethods         []string  `json:"token_endpoint_auth_methods_supported,omitempty"`
}

type cachedClient struct {
	metadata  ClientMetadata
	expiresAt time.Time
}
type clientFetch struct {
	done     chan struct{}
	metadata ClientMetadata
	err      error
}
type clientResolver struct {
	client     *http.Client
	registered map[string]ClientMetadata
	mu         sync.Mutex
	cache      map[string]cachedClient
	pending    map[string]*clientFetch
	fetches    chan struct{}
}

func newClientResolver(registered []ClientMetadata) (*clientResolver, error) {
	if len(registered) > metadataMaxCache {
		return nil, errors.New("too many preregistered OAuth clients")
	}
	r := &clientResolver{registered: make(map[string]ClientMetadata), cache: make(map[string]cachedClient), pending: make(map[string]*clientFetch), fetches: make(chan struct{}, metadataMaxFetches)}
	for _, m := range registered {
		m.RegistrationVersion = ""
		m.RegistrationTeamID = uuid.Nil()
		if err := validateClientMetadata(m, m.ID, true); err != nil {
			return nil, err
		}
		if _, exists := r.registered[m.ID]; exists {
			return nil, errors.New("duplicate preregistered OAuth client")
		}
		r.registered[m.ID] = cloneClientMetadata(m)
	}
	r.client = &http.Client{Transport: metadataTransport(net.DefaultResolver.LookupNetIP, (&net.Dialer{Timeout: 3 * time.Second}).DialContext), Timeout: metadataTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return r, nil
}

func cloneClientMetadata(m ClientMetadata) ClientMetadata {
	m.RedirectURIs = slices.Clone(m.RedirectURIs)
	m.GrantTypes = slices.Clone(m.GrantTypes)
	m.ResponseTypes = slices.Clone(m.ResponseTypes)
	m.AuthMethods = slices.Clone(m.AuthMethods)
	return m
}

func (r *clientResolver) resolve(ctx context.Context, id string) (ClientMetadata, error) {
	if m, ok := r.registered[id]; ok {
		return cloneClientMetadata(m), nil
	}
	if _, err := metadataURL(id); err != nil {
		return ClientMetadata{}, err
	}
	r.mu.Lock()
	if c, ok := r.cache[id]; ok && time.Now().Before(c.expiresAt) {
		r.mu.Unlock()
		return cloneClientMetadata(c.metadata), nil
	}
	if pending, ok := r.pending[id]; ok {
		r.mu.Unlock()
		select {
		case <-ctx.Done():
			return ClientMetadata{}, ctx.Err()
		case <-pending.done:
			return cloneClientMetadata(pending.metadata), pending.err
		}
	}
	select {
	case r.fetches <- struct{}{}:
	default:
		r.mu.Unlock()
		return ClientMetadata{}, errMetadataBusy
	}
	pending := &clientFetch{done: make(chan struct{})}
	r.pending[id] = pending
	r.mu.Unlock()
	m, ttl, err := r.fetch(ctx, id)
	r.mu.Lock()
	if err == nil && ttl > 0 {
		if len(r.cache) >= metadataMaxCache {
			var oldest string
			var expiry time.Time
			for key, value := range r.cache {
				if oldest == "" || value.expiresAt.Before(expiry) {
					oldest = key
					expiry = value.expiresAt
				}
			}
			delete(r.cache, oldest)
		}
		r.cache[id] = cachedClient{metadata: cloneClientMetadata(m), expiresAt: time.Now().Add(ttl)}
	}
	pending.metadata = cloneClientMetadata(m)
	pending.err = err
	delete(r.pending, id)
	close(pending.done)
	<-r.fetches
	r.mu.Unlock()
	return m, err
}

func (r *clientResolver) fetch(ctx context.Context, id string) (ClientMetadata, time.Duration, error) {
	ctx, cancel := context.WithTimeout(ctx, metadataTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, id, nil) // #nosec G704 -- resolve validates an HTTPS metadata URL; the pinned transport rejects every nonpublic DNS answer.
	if err != nil {
		return ClientMetadata{}, 0, errors.New("invalid client metadata request")
	}
	req.Header.Set("Accept", "application/json")
	response, err := r.client.Do(req) // #nosec G704 -- The custom transport pins validated public IPs, verifies the original TLS hostname, and disables proxies and redirects.
	if err != nil {
		return ClientMetadata{}, 0, errors.New("OAuth client metadata unavailable")
	}
	defer response.Body.Close()
	media, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if response.StatusCode != http.StatusOK || err != nil || media != "application/json" || response.Header.Get("Content-Encoding") != "" {
		return ClientMetadata{}, 0, errors.New("OAuth client metadata must be a JSON document without redirects")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, metadataMaxBytes+1))
	if err != nil || len(body) > metadataMaxBytes {
		return ClientMetadata{}, 0, errors.New("OAuth client metadata exceeds bounded response")
	}
	var m ClientMetadata
	if err := json.Unmarshal(body, &m); err != nil {
		return ClientMetadata{}, 0, errors.New("invalid OAuth client metadata JSON")
	}
	m.RegistrationVersion = ""
	m.RegistrationTeamID = uuid.Nil()
	if err := validateClientMetadata(m, id, false); err != nil {
		return ClientMetadata{}, 0, err
	}
	return m, metadataCacheTTL(response.Header.Get("Cache-Control")), nil
}

func metadataCacheTTL(control string) time.Duration {
	ttl := time.Hour
	for directive := range strings.SplitSeq(control, ",") {
		directive = strings.TrimSpace(directive)
		if strings.EqualFold(directive, "no-store") || strings.EqualFold(directive, "no-cache") {
			return 0
		}
		key, value, ok := strings.Cut(directive, "=")
		if ok && strings.EqualFold(strings.TrimSpace(key), "max-age") {
			seconds, err := strconv.ParseInt(strings.Trim(strings.TrimSpace(value), "\""), 10, 64)
			if err != nil || seconds <= 0 {
				return 0
			}
			if seconds < 3600 {
				ttl = time.Duration(seconds) * time.Second
			}
		}
	}
	return ttl
}

func metadataURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 2048 || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Opaque != "" || u.Fragment != "" || u.RawFragment != "" || strings.ContainsAny(raw, "#\\\r\n\t") || strings.ContainsFunc(raw, unicode.IsSpace) {
		return nil, errors.New("OAuth client ID must identify an HTTPS metadata document")
	}
	if port := u.Port(); port != "" {
		n, e := strconv.Atoi(port)
		if e != nil || n < 1 || n > 65535 {
			return nil, errors.New("invalid client metadata port")
		}
	}
	return u, nil
}

func validateClientMetadata(m ClientMetadata, id string, registered bool) error {
	if m.ID != id || id == "" || len(id) > 2048 {
		return errors.New("OAuth metadata client ID does not match its document")
	}
	if !registered {
		if _, err := metadataURL(id); err != nil {
			return err
		}
	}
	if strings.TrimSpace(m.Name) == "" || len(m.Name) > 128 || strings.ContainsFunc(m.Name, unicode.IsControl) {
		return errors.New("invalid OAuth client name")
	}
	if m.URI != "" {
		if _, err := metadataURL(m.URI); err != nil {
			return errors.New("invalid OAuth client information URL")
		}
	}
	if len(m.AuthMethods) > 0 {
		if !slices.Contains(m.AuthMethods, "none") {
			return errors.New("OAuth client does not support public PKCE")
		}
	} else if m.AuthMethod != "none" {
		return errors.New("OAuth client does not support public PKCE")
	}
	if len(m.RedirectURIs) == 0 || len(m.RedirectURIs) > 16 {
		return errors.New("OAuth client must supply bounded exact redirects")
	}
	seen := make(map[string]bool, len(m.RedirectURIs))
	for _, raw := range m.RedirectURIs {
		if _, err := callbackURL(raw); err != nil || seen[raw] {
			return errors.New("invalid OAuth client redirect")
		}
		seen[raw] = true
	}
	if len(m.GrantTypes) > 0 && !slices.Contains(m.GrantTypes, "authorization_code") {
		return errors.New("OAuth client does not support authorization codes")
	}
	if len(m.ResponseTypes) > 0 && !slices.Contains(m.ResponseTypes, "code") {
		return errors.New("OAuth client does not support code responses")
	}
	return nil
}

// callbackURL validates browser destinations independently of HTTPS-only metadata fetching.
func callbackURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 2048 || u.Hostname() == "" || u.Opaque != "" || u.User != nil || u.Fragment != "" || u.RawFragment != "" || strings.ContainsAny(raw, "#\\\r\n\t") || strings.ContainsFunc(raw, unicode.IsSpace) || strings.Contains(u.Host, "*") || strings.HasSuffix(u.Host, ":") || u.String() != raw {
		return nil, errors.New("invalid OAuth client redirect")
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return nil, errors.New("invalid OAuth redirect port")
		}
	}
	if u.Scheme == "https" {
		return u, nil
	}
	ip, ipErr := netip.ParseAddr(u.Hostname())
	if u.Scheme != "http" || (u.Hostname() != "localhost" && (ipErr != nil || !ip.IsLoopback() || ip.Zone() != "")) {
		return nil, errors.New("OAuth redirect must be HTTPS or an explicit native loopback")
	}
	host := u.Hostname()
	if u.Port() != "" {
		host = net.JoinHostPort(host, u.Port())
	} else if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if u.Host != host {
		return nil, errors.New("invalid native loopback host")
	}
	return u, nil
}

func redirectAllowed(m ClientMetadata, redirect string) bool {
	requested, err := callbackURL(redirect)
	if err != nil {
		return false
	}
	if slices.Contains(m.RedirectURIs, redirect) {
		return true
	}
	if requested.Scheme != "http" || requested.Port() == "" {
		return false
	}
	for _, raw := range m.RedirectURIs {
		registered, err := callbackURL(raw)
		if err != nil || registered.Scheme != "http" || registered.Hostname() != requested.Hostname() {
			continue
		}
		// Native clients choose a local listener port; every other byte stays declared.
		candidate := requested.Clone()
		candidate.Host = registered.Host
		if candidate.String() == raw {
			return true
		}
	}
	return false
}

var metadataDeniedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"), netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"), netip.MustParsePrefix("192.88.99.0/24"), netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("198.51.100.0/24"), netip.MustParsePrefix("203.0.113.0/24"), netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("::/96"), netip.MustParsePrefix("64:ff9b::/96"), netip.MustParsePrefix("64:ff9b:1::/48"), netip.MustParsePrefix("100::/64"), netip.MustParsePrefix("2001::/23"), netip.MustParsePrefix("2001:db8::/32"), netip.MustParsePrefix("2002::/16"), netip.MustParsePrefix("fec0::/10"), netip.MustParsePrefix("3fff::/20"),
}

func publicMetadataIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.Zone() != "" {
		return false
	}
	for _, prefix := range metadataDeniedPrefixes {
		if prefix.Contains(ip) {
			return false
		}
	}
	return true
}

func metadataTransport(lookup func(context.Context, string, string) ([]netip.Addr, error), dial func(context.Context, string, string) (net.Conn, error)) *http.Transport {
	return &http.Transport{
		Proxy: nil, DisableKeepAlives: true, MaxConnsPerHost: metadataMaxFetches, MaxResponseHeaderBytes: 16 << 10, TLSHandshakeTimeout: 3 * time.Second, ResponseHeaderTimeout: 3 * time.Second,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			hostname, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, errors.New("invalid metadata destination")
			}
			addresses, err := lookup(ctx, "ip", hostname)
			if err != nil || len(addresses) == 0 {
				return nil, errors.New("metadata DNS unavailable")
			}
			for _, ip := range addresses {
				if !publicMetadataIP(ip) {
					return nil, errors.New("metadata DNS contains a nonpublic destination")
				}
			}
			for _, ip := range addresses {
				conn, err := dial(ctx, network, net.JoinHostPort(ip.Unmap().String(), port))
				if err == nil {
					return conn, nil
				}
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
			}
			return nil, errors.New("metadata destination unavailable")
		},
	}
}
