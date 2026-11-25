package sites

import (
	"context"
	"errors"
	"net/http"
	"net/http/httputil"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/publicsuffix"

	"hitkeep/server/shared"
)

func (h *handler) handleGetFavicon() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		domain := normalizeFaviconDomain(r.PathValue("domain"))
		if !isValidFaviconDomain(domain) {
			http.Error(w, "Invalid domain", http.StatusBadRequest)
			return
		}

		proxy := &httputil.ReverseProxy{
			Rewrite: func(proxyReq *httputil.ProxyRequest) {
				proxyReq.Out.URL = &url.URL{Scheme: "https", Host: "icons.duckduckgo.com", Path: "/ip3/" + domain + ".ico"}
				proxyReq.Out.Host = ""
				proxyReq.Out.Method = http.MethodGet
				proxyReq.Out.Body = nil
				proxyReq.Out.ContentLength = 0
				proxyReq.Out.Header = make(http.Header)
				for _, key := range []string{"If-None-Match", "If-Modified-Since"} {
					if value := proxyReq.In.Header.Get(key); value != "" {
						proxyReq.Out.Header.Set(key, value)
					}
				}
			},
			Transport: faviconLookup{domain: domain, upstream: faviconProxyTransport},
			ModifyResponse: func(resp *http.Response) error {
				resp.Header.Set("Cache-Control", "public, max-age=86400")
				if resp.StatusCode == http.StatusNotModified {
					// The browser revalidated its cached icon. An empty 204
					// would replace that icon with nothing for a day.
					return nil
				}
				if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
					if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError {
						resp.Header.Set("Cache-Control", "public, max-age=300")
					}
					_ = resp.Body.Close()
					resp.StatusCode = http.StatusNoContent
					resp.Status = http.StatusText(http.StatusNoContent)
					resp.Body = http.NoBody
					resp.ContentLength = 0
					resp.Header.Del("ETag")
					resp.Header.Del("Last-Modified")
					resp.Header.Del("Content-Length")
					resp.Header.Del("Content-Type")
				}
				return nil
			},
			ErrorHandler: func(rw http.ResponseWriter, req *http.Request, proxyErr error) {
				shared.LoggerFromContext(req.Context()).Warn("Failed to fetch favicon upstream", "domain", domain, "error_kind", faviconProxyErrorKind(proxyErr))
				rw.Header().Set("Cache-Control", "public, max-age=300")
				rw.WriteHeader(http.StatusNoContent)
			},
		}

		proxy.ServeHTTP(w, r)
	}
}

type faviconLookup struct {
	domain   string
	upstream http.RoundTripper
}

func (lookup faviconLookup) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := lookup.upstream.RoundTrip(req)
	if err != nil || (resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusGone) {
		return resp, err
	}

	domain := alternateFaviconDomain(lookup.domain)
	if domain == "" {
		return resp, nil
	}
	_ = resp.Body.Close()
	retry := req.Clone(req.Context())
	retry.URL.Path = "/ip3/" + domain + ".ico"
	// Conditional headers belong to the exact host's icon.
	retry.Header = make(http.Header)
	resp, err = lookup.upstream.RoundTrip(retry)
	if err == nil {
		resp.Header.Del("ETag")
		resp.Header.Del("Last-Modified")
	}
	return resp, err
}

func alternateFaviconDomain(domain string) string {
	if _, err := netip.ParseAddr(domain); err == nil {
		return ""
	}
	base, err := publicsuffix.EffectiveTLDPlusOne(domain)
	switch {
	case err != nil:
		return ""
	case base == "twitter.com":
		// Twitter redirects to X, whose icon is indexed under the new name.
		return "x.com"
	case domain != base:
		return base
	case len("www."+base) <= 253:
		return "www." + base
	default:
		return ""
	}
}

func faviconProxyErrorKind(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	default:
		return "upstream_request_failed"
	}
}

func normalizeFaviconDomain(domain string) string {
	trimmed := strings.TrimSpace(domain)
	return strings.TrimSuffix(strings.ToLower(trimmed), ".")
}

func isValidFaviconDomain(domain string) bool {
	if domain == "" {
		return false
	}
	if strings.ContainsAny(domain, `/\?#`) {
		return false
	}
	return domainRegex.MatchString(domain)
}

func newFaviconProxyTransport(timeout time.Duration) http.RoundTripper {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = timeout
	return transport
}
