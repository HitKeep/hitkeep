package oauth

import (
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestMetadataRejectsNonpublicAddresses(t *testing.T) {
	for _, ip := range []string{"127.0.0.1", "10.1.2.3", "172.16.0.1", "192.168.1.1", "169.254.169.254", "100.64.0.1", "0.0.0.0", "192.0.2.1", "198.18.0.1", "203.0.113.2", "224.0.0.1", "240.0.0.1", "::1", "::", "fc00::1", "fe80::1", "fec0::1", "3fff::1", "2001:db8::1", "2002:7f00:1::", "64:ff9b::7f00:1", "::ffff:127.0.0.1"} {
		if publicMetadataIP(netip.MustParseAddr(ip)) {
			t.Errorf("accepted nonpublic %s", ip)
		}
	}
	for _, ip := range []string{"8.8.8.8", "1.1.1.1", "2606:4700:4700::1111"} {
		if !publicMetadataIP(netip.MustParseAddr(ip)) {
			t.Errorf("rejected public %s", ip)
		}
	}
}

func TestClientMetadataExactIdentityMethodsAndRedirects(t *testing.T) {
	good := ClientMetadata{ID: "https://example.test/client", Name: "Example", RedirectURIs: []string{"https://example.test/admin-post.php?action=callback"}, AuthMethod: "private_key_jwt", AuthMethods: []string{"private_key_jwt", "none"}}
	if err := validateClientMetadata(good, good.ID, false); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*ClientMetadata){
		func(m *ClientMetadata) { m.ID = "https://evil.test/client" },
		func(m *ClientMetadata) { m.RedirectURIs = []string{"https://example.test/#fragment"} },
		func(m *ClientMetadata) { m.RedirectURIs = []string{"https://user:password@example.test/callback"} },
		func(m *ClientMetadata) { m.RedirectURIs = []string{"http://127.0.0.1:1234/callback"} },
		func(m *ClientMetadata) { m.AuthMethods = []string{"private_key_jwt"} },
		func(m *ClientMetadata) { m.Name = "Example\nspoof" },
	} {
		m := good
		change(&m)
		if err := validateClientMetadata(m, good.ID, false); err == nil {
			t.Errorf("accepted invalid metadata %+v", m)
		}
	}
	loopback := ClientMetadata{ID: "native-client", Name: "Native", RedirectURIs: []string{"http://127.0.0.1:1243/callback"}, AuthMethod: "none"}
	if err := validateClientMetadata(loopback, loopback.ID, true); err != nil {
		t.Fatal(err)
	}
	if redirectAllowed(good, "https://example.test/admin-post.php?action=callback&extra=1") {
		t.Fatal("redirect match was not exact")
	}
	if !redirectAllowed(good, good.RedirectURIs[0]) {
		t.Fatal("exact redirect rejected")
	}
}

func metadataTLSServer(t *testing.T, h http.Handler) (*httptest.Server, *x509.CertPool) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cert := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "example.test"}, DNSNames: []string{"example.test"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, public, private)
	if err != nil {
		t.Fatal(err)
	}
	key, err := x509.MarshalPKCS8PrivateKey(private)
	if err != nil {
		t.Fatal(err)
	}
	pair, err := tls.X509KeyPair(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key}))
	if err != nil {
		t.Fatal(err)
	}
	s := httptest.NewUnstartedServer(h)
	s.TLS = &tls.Config{Certificates: []tls.Certificate{pair}, MinVersion: tls.VersionTLS12}
	s.StartTLS()
	t.Cleanup(s.Close)
	roots := x509.NewCertPool()
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	roots.AddCert(parsed)
	return s, roots
}

func testResolver(t *testing.T, h http.Handler) (*clientResolver, string, *atomic.Int64) {
	t.Helper()
	s, roots := metadataTLSServer(t, h)
	var lookups atomic.Int64
	lookup := func(context.Context, string, string) ([]netip.Addr, error) {
		lookups.Add(1)
		return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
	}
	dial := func(ctx context.Context, network, address string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		if host != "8.8.8.8" {
			return nil, fmt.Errorf("dial was not pinned: %s", host)
		}
		return (&net.Dialer{}).DialContext(ctx, network, s.Listener.Addr().String())
	}
	transport := metadataTransport(lookup, dial)
	transport.TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
	t.Cleanup(transport.CloseIdleConnections)
	r, err := newClientResolver(nil)
	if err != nil {
		t.Fatal(err)
	}
	r.client.Transport = transport
	return r, "https://example.test/client", &lookups
}

func TestMetadataFetchPinsDNSCachesAndClones(t *testing.T) {
	var count atomic.Int64
	r, id, lookups := testResolver(t, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		count.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "max-age=999999")
		fmt.Fprint(w, `{"client_id":"https://example.test/client","client_name":"Example","redirect_uris":["https://example.test/callback"],"token_endpoint_auth_method":"private_key_jwt","token_endpoint_auth_methods_supported":["none","private_key_jwt"],"jwks_uri":"http://169.254.169.254/credentials"}`)
	}))
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	first, err := r.resolve(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	first.RedirectURIs[0] = "https://evil.test/callback"
	second, err := r.resolve(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if count.Load() != 1 || lookups.Load() != 1 || second.RedirectURIs[0] != "https://example.test/callback" {
		t.Fatalf("cache/snapshot violation: requests=%d lookups=%d metadata=%+v", count.Load(), lookups.Load(), second)
	}
	r.mu.Lock()
	ttl := time.Until(r.cache[id].expiresAt)
	r.mu.Unlock()
	if ttl > time.Hour || ttl < 59*time.Minute {
		t.Fatalf("cache freshness not capped: %s", ttl)
	}
}

func TestMetadataFetchRejectsRedirectHTMLDuplicateAndOversize(t *testing.T) {
	for _, tc := range []struct {
		name, contentType, body string
		status                  int
	}{
		{"redirect", "application/json", "", 302},
		{"html", "text/html", "<h1>client</h1>", 200},
		{"duplicate", "application/json", `{"client_id":"https://example.test/client","client_id":"https://evil.test/client"}`, 200},
		{"oversize", "application/json", strings.Repeat(" ", metadataMaxBytes+1), 200},
		{"wrong identity", "application/json", `{"client_id":"https://evil.test/client","client_name":"Example","redirect_uris":["https://example.test/callback"],"token_endpoint_auth_method":"none"}`, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var hits atomic.Int64
			r, id, _ := testResolver(t, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				hits.Add(1)
				w.Header().Set("Content-Type", tc.contentType)
				w.Header().Set("Location", "https://example.test/redirected")
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			if _, err := r.resolve(t.Context(), id); err == nil {
				t.Fatal("accepted malicious metadata")
			}
			if hits.Load() != 1 {
				t.Fatalf("followed redirect/nested fetch: %d", hits.Load())
			}
		})
	}
}

func TestMetadataTransportRejectsMixedDNSBeforeDial(t *testing.T) {
	var dials atomic.Int64
	tr := metadataTransport(func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("127.0.0.1")}, nil
	}, func(context.Context, string, string) (net.Conn, error) {
		dials.Add(1)
		return nil, errors.New("must not dial")
	})
	_, err := tr.DialContext(t.Context(), "tcp", "example.test:443")
	if err == nil || dials.Load() != 0 {
		t.Fatalf("mixed DNS reached dial: err=%v dials=%d", err, dials.Load())
	}
}

func TestMetadataConcurrentFetchIsBoundedAndCoalesced(t *testing.T) {
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	var count atomic.Int64
	r, id, _ := testResolver(t, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		count.Add(1)
		select {
		case entered <- struct{}{}:
		default:
		}
		<-release
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"client_id":"https://example.test/client","client_name":"Example","redirect_uris":["https://example.test/callback"],"token_endpoint_auth_method":"none"}`)
	}))
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for range 16 {
		wg.Go(func() { _, err := r.resolve(t.Context(), id); errs <- err })
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("fetch never started")
	}
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if count.Load() != 1 {
		t.Fatalf("duplicate concurrent metadata fetches: %d", count.Load())
	}
	for range metadataMaxFetches {
		r.fetches <- struct{}{}
	}
	if _, err := r.resolve(t.Context(), "https://different.test/client"); !errors.Is(err, errMetadataBusy) {
		t.Fatalf("unbounded metadata fetches: %v", err)
	}
	for range metadataMaxFetches {
		<-r.fetches
	}
}

func TestClientMetadataRejectsEmptyFragmentAndInvalidRedirectPorts(t *testing.T) {
	for _, redirect := range []string{"https://example.test/callback#", "https://example.test:0/callback", "https://example.test:65536/callback"} {
		m := ClientMetadata{ID: "https://example.test/client", Name: "Example", RedirectURIs: []string{redirect}, AuthMethod: "none"}
		if err := validateClientMetadata(m, m.ID, false); err == nil {
			t.Errorf("accepted invalid redirect %q", redirect)
		}
	}
	if _, err := metadataURL("https://example.test/client#"); err == nil {
		t.Error("accepted client ID fragment")
	}
}

func TestMetadataDecompressedBodyBoundAndTLSVerification(t *testing.T) {
	r, id, _ := testResolver(t, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Encoding", "gzip")
		z := gzip.NewWriter(w)
		fmt.Fprint(z, strings.Repeat(" ", metadataMaxBytes+1))
		if err := z.Close(); err != nil {
			t.Error(err)
		}
	}))
	if _, err := r.resolve(t.Context(), id); err == nil {
		t.Fatal("accepted decompression bomb")
	}
	r, id, _ = testResolver(t, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) { t.Error("untrusted TLS reached application") }))
	r.client.Transport.(*http.Transport).TLSClientConfig.RootCAs = x509.NewCertPool()
	if _, err := r.resolve(t.Context(), id); err == nil {
		t.Fatal("accepted untrusted TLS certificate")
	}
}

func TestMetadataFetchHonorsCancellation(t *testing.T) {
	r, id, _ := testResolver(t, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) { <-req.Context().Done() }))
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
	defer cancel()
	started := time.Now()
	if _, err := r.resolve(ctx, id); err == nil {
		t.Fatal("accepted stalled fetch")
	}
	if time.Since(started) > time.Second {
		t.Fatal("metadata fetch ignored cancellation")
	}
}

func TestMetadataRevalidatesDNSAfterConnection(t *testing.T) {
	var hits atomic.Int64
	r, id, _ := testResolver(t, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"client_id":"https://example.test%s","client_name":"Example","redirect_uris":["https://example.test/callback"],"token_endpoint_auth_method":"none"}`, req.URL.RequestURI())
	}))
	old := r.client.Transport.(*http.Transport)
	var lookupCalls, dials atomic.Int64
	transport := metadataTransport(func(context.Context, string, string) ([]netip.Addr, error) {
		if lookupCalls.Add(1) == 1 {
			return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
		}
		return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
	}, func(ctx context.Context, network, address string) (net.Conn, error) {
		dials.Add(1)
		return old.DialContext(ctx, network, "example.test:443")
	})
	transport.TLSClientConfig = old.TLSClientConfig
	r.client.Transport = transport
	t.Cleanup(transport.CloseIdleConnections)
	if _, err := r.resolve(t.Context(), id+"?one"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.resolve(t.Context(), id+"?two"); err == nil {
		t.Fatal("accepted DNS rebinding")
	}
	if lookupCalls.Load() != 2 || dials.Load() != 1 || hits.Load() != 1 {
		t.Fatalf("rebound destination reached dial/server: lookups=%d dials=%d hits=%d", lookupCalls.Load(), dials.Load(), hits.Load())
	}
}

func TestMetadataCacheCardinalityAndPolicy(t *testing.T) {
	r, id, _ := testResolver(t, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"client_id":"https://example.test/client","client_name":"Example","redirect_uris":["https://example.test/callback"],"token_endpoint_auth_method":"none"}`)
	}))
	for i := range metadataMaxCache {
		r.cache[fmt.Sprint(i)] = cachedClient{expiresAt: time.Now().Add(time.Hour)}
	}
	if _, err := r.resolve(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	if len(r.cache) != metadataMaxCache {
		t.Fatalf("unbounded cache cardinality: %d", len(r.cache))
	}
	for _, control := range []string{"no-store", "no-cache", "max-age=0", "max-age=-1", "max-age=invalid", "max-age=999999999999999999999"} {
		if got := metadataCacheTTL(control); got != 0 {
			t.Errorf("unsafe cache policy %q retained for %s", control, got)
		}
	}
	if got := metadataCacheTTL("max-age=30"); got != 30*time.Second {
		t.Errorf("ignored freshness: %s", got)
	}
}
