package analyticstools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDocsClientBlocksOtherOriginsAndCachesMarkdown(t *testing.T) {
	requests := 0
	docsTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if !strings.Contains(r.Header.Get("Accept"), "text/markdown") {
			t.Errorf("expected Accept header to include text/markdown, got %q", r.Header.Get("Accept"))
		}
		if r.URL.Path != "/guides/integrations/mcp/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/markdown")
		_, _ = w.Write([]byte("# MCP Integration\n\nUse the official server.\n"))
	}))
	defer docsTS.Close()

	client := NewDocs(docsTS.URL, time.Hour)
	for i := range 2 {
		page, err := client.GetMarkdown(context.Background(), "/guides/integrations/mcp")
		if err != nil {
			t.Fatalf("GetMarkdown attempt %d: %v", i+1, err)
		}
		if page.Path != "/guides/integrations/mcp/" || !strings.Contains(page.Markdown, "# MCP Integration") {
			t.Fatalf("unexpected page: %+v", page)
		}
	}
	if requests != 1 {
		t.Fatalf("expected cached second docs fetch, got %d requests", requests)
	}
	if _, err := client.GetMarkdown(context.Background(), "https://example.com/guides/integrations/mcp/"); err == nil {
		t.Fatalf("expected other docs origin to be rejected")
	}
	if _, err := client.GetMarkdown(context.Background(), "/guides/%2e%2e/secret"); err == nil {
		t.Fatalf("expected encoded parent traversal to be rejected")
	}
}

func TestDocsClientBlocksRedirectsToOtherOrigins(t *testing.T) {
	var forbiddenRequests atomic.Int32
	forbidden := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		forbiddenRequests.Add(1)
		_, _ = w.Write([]byte("# Untrusted content"))
	}))
	defer forbidden.Close()

	docsTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start/" {
			http.Redirect(w, r, "/redirect/", http.StatusFound)
			return
		}
		http.Redirect(w, r, forbidden.URL+"/private/?secret=redirect-marker", http.StatusTemporaryRedirect)
	}))
	defer docsTS.Close()

	page, err := NewDocs(docsTS.URL, time.Hour).GetMarkdown(t.Context(), "/start/")
	if err == nil || err.Error() != "docs redirect rejected" {
		t.Errorf("redirect error = %v, want a static rejection without the destination URL", err)
	}
	if page != (DocPage{}) {
		t.Errorf("blocked redirect returned a page: %+v", page)
	}
	if got := forbiddenRequests.Load(); got != 0 {
		t.Errorf("forbidden destination received %d requests, want zero", got)
	}
}

func TestDocsClientRejectsMalformedRedirectWithoutDetails(t *testing.T) {
	docsTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "http://[::1/?secret=redirect-marker")
		w.WriteHeader(http.StatusFound)
	}))
	defer docsTS.Close()

	page, err := NewDocs(docsTS.URL, time.Hour).GetMarkdown(t.Context(), "/start/")
	if err == nil || err.Error() != "fetch docs markdown failed" {
		t.Fatal("malformed redirect did not return a bounded fetch error")
	}
	if page != (DocPage{}) {
		t.Fatal("malformed redirect returned a page")
	}
}

func TestDocsClientFollowsSameOriginRedirects(t *testing.T) {
	var requests atomic.Int32
	docsTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path == "/start/" {
			http.Redirect(w, r, "/guide/", http.StatusMovedPermanently)
			return
		}
		_, _ = w.Write([]byte("# Official guide"))
	}))
	defer docsTS.Close()

	client := NewDocs(docsTS.URL, time.Hour)
	for range 2 {
		page, err := client.GetMarkdown(t.Context(), "/start/")
		if err != nil || page.Markdown != "# Official guide" {
			t.Fatalf("same-origin redirect = %+v, %v", page, err)
		}
	}
	if got := requests.Load(); got != 2 {
		t.Errorf("same-origin redirect and cached read made %d requests, want two", got)
	}
}

func TestDocsClientBoundsRedirectLoops(t *testing.T) {
	var requests atomic.Int32
	docsTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Redirect(w, r, "/loop/?secret=redirect-marker", http.StatusFound)
	}))
	defer docsTS.Close()

	page, err := NewDocs(docsTS.URL, time.Hour).GetMarkdown(t.Context(), "/loop/")
	if err == nil || err.Error() != "docs redirect rejected" {
		t.Errorf("redirect loop error = %v, want a static rejection without the destination URL", err)
	}
	if page != (DocPage{}) {
		t.Errorf("redirect loop returned a page: %+v", page)
	}
	if got := requests.Load(); got != 10 {
		t.Errorf("redirect loop made %d requests, want ten", got)
	}
}

func TestDocsClientCoalescesConcurrentFetches(t *testing.T) {
	var requests atomic.Int32
	docsTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		time.Sleep(50 * time.Millisecond)
		w.Header().Set("Content-Type", "text/markdown")
		_, _ = w.Write([]byte("# MCP Integration\n"))
	}))
	defer docsTS.Close()

	client := NewDocs(docsTS.URL, time.Hour)
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for range 20 {
		wg.Go(func() {
			_, err := client.GetMarkdown(context.Background(), "/guides/integrations/mcp/")
			errs <- err
		})
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("GetMarkdown: %v", err)
		}
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("expected concurrent requests to coalesce to one fetch, got %d", got)
	}
}

func TestDocsClientCapsCachedPages(t *testing.T) {
	docsTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/markdown")
		_, _ = w.Write([]byte("# " + r.URL.Path + "\n"))
	}))
	defer docsTS.Close()

	client := NewDocs(docsTS.URL, time.Hour)
	for i := range maxDocCacheEntries + 5 {
		if _, err := client.GetMarkdown(context.Background(), "/guides/page-"+strconv.Itoa(i)+"/"); err != nil {
			t.Fatalf("GetMarkdown page %d: %v", i, err)
		}
	}

	if got := client.pages.Len(); got != maxDocCacheEntries {
		t.Fatalf("expected docs cache to cap at %d entries, got %d", maxDocCacheEntries, got)
	}
}

func TestDocsSearchDeduplicatesNormalizedPathsBeforeLimit(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path == "/llms.txt" {
			_, _ = w.Write([]byte("- [MCP](/guide): MCP\n- [MCP](/guide/): MCP\n- [Other](/other/): MCP bearer\n"))
			return
		}
		_, _ = w.Write([]byte("# Guide\nMCP bearer authorization\n"))
	}))
	defer server.Close()
	client := NewDocs(server.URL, time.Hour)
	if _, err := client.GetMarkdown(t.Context(), "/guide"); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		results, err := client.Search(t.Context(), "MCP bearer authorization", 2)
		if err != nil || len(results) != 2 {
			t.Fatalf("results = %+v, %v", results, err)
		}
		if results[0].Path != "/guide/" || results[0].Score != 3 || results[0].Snippet != "MCP bearer authorization" || results[1].Path != "/other/" {
			t.Fatalf("deduplicated results = %+v", results)
		}
	}
	if requests.Load() != 2 {
		t.Fatalf("search made extra fetches: %d", requests.Load())
	}
}
