package crawl

import (
	"bytes"
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cpcf/araneae/internal/report"
)

func TestSequentialCrawlerDuplicateCounts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<a href="/dup">Once</a><a href="/dup">Once</a><a href="/dup">Twice</a>`))
		case "/dup":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<p>ok</p>`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	reportData := runScan(t, server.URL+"/", 10, 1)

	if got := reportData.Summary.LinksDiscovered; got != 1 {
		t.Fatalf("links discovered = %d; want 1", got)
	}
	link := reportData.Links[0]
	if link.Count != 3 {
		t.Fatalf("duplicate count = %d; want 3", link.Count)
	}
	if len(link.Sources) != 1 {
		t.Fatalf("sources = %d; want 1", len(link.Sources))
	}
	if len(link.Sources[0].Texts) != 2 {
		t.Fatalf("source text snippets = %d; want 2", len(link.Sources[0].Texts))
	}
}

func TestSequentialCrawlerSkipsExternalLinks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<a href="/internal">Internal</a><a href="https://external.example.com/">External</a>`))
		case "/internal":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte("<p>ok</p>"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	reportData := runScan(t, server.URL+"/", 10, 1)

	if got := reportData.Summary.SkippedLinks; got != 1 {
		t.Fatalf("skipped links = %d; want 1", got)
	}
	if got := reportData.Summary.SkippedExternal; got != 1 {
		t.Fatalf("skipped external links = %d; want 1", got)
	}
	if got := reportData.SkippedLinks[0].Reason; got != ScopeReasonExternalOrigin {
		t.Fatalf("skip reason = %q; want %q", got, ScopeReasonExternalOrigin)
	}
}

func TestSequentialCrawlerAvoidsCycles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<a href="/cycle">cycle</a>`))
		case "/cycle":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<a href="/">home</a><a href="/cycle">again</a>`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	reportData := runScan(t, server.URL+"/", 10, 1)

	if reportData.Summary.FetchesAttempted != 2 {
		t.Fatalf("fetches attempted = %d; want 2", reportData.Summary.FetchesAttempted)
	}
	if got := reportData.Summary.Truncated; got {
		t.Fatal("expected full crawl without truncation")
	}
}

func TestSequentialCrawlerMaxPagesTruncates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<a href="/first">first</a><a href="/second">second</a>`))
		case "/first":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte("<p>first</p>"))
		case "/second":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte("<p>second</p>"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	reportData := runScan(t, server.URL+"/", 1, 1)

	if !reportData.Summary.Truncated {
		t.Fatal("expected report to be truncated")
	}
	if reportData.Summary.UnvisitedURLs != 2 {
		t.Fatalf("unvisited urls = %d; want 2", reportData.Summary.UnvisitedURLs)
	}
	if reportData.Summary.FetchesAttempted != 1 {
		t.Fatalf("fetches attempted = %d; want 1", reportData.Summary.FetchesAttempted)
	}
}

func TestSequentialCrawlerNon200Handled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<a href="/not-found">Not found</a>`))
		case "/not-found":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("missing"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	reportData := runScan(t, server.URL+"/", 10, 1)

	if got := reportData.Summary.Non200Links; got != 1 {
		t.Fatalf("non-200 links = %d; want 1", got)
	}
	if got := reportData.Links[0].Dead; got != true {
		t.Fatalf("dead = %v; want true", got)
	}
	if got := reportData.Links[0].Problem; got != "http_status" {
		t.Fatalf("problem = %q; want http_status", got)
	}
}

func TestCrawlerRetriesTransientStatusThenReportsFinalOK(t *testing.T) {
	var unstableAttempts int32
	retrySleeps := []time.Duration{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<a href="/unstable">Unstable</a>`))
		case "/unstable":
			attempt := atomic.AddInt32(&unstableAttempts, 1)
			if attempt == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte("try again"))
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte("<p>ok</p>"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	reportData, err := Run(context.Background(), ScanOptions{
		EntryURL:     server.URL + "/",
		MaxPages:     10,
		Timeout:      2 * time.Second,
		UserAgent:    "araneae-test",
		Concurrency:  1,
		Retries:      1,
		RetryBackoff: 25 * time.Millisecond,
		retrySleep: func(_ context.Context, d time.Duration) error {
			retrySleeps = append(retrySleeps, d)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("run scanner: %v", err)
	}

	if got := atomic.LoadInt32(&unstableAttempts); got != 2 {
		t.Fatalf("unstable attempts = %d; want 2", got)
	}
	if len(retrySleeps) != 1 || retrySleeps[0] != 25*time.Millisecond {
		t.Fatalf("retry sleeps = %#v; want [25ms]", retrySleeps)
	}
	if reportData.Limits.Retries != 1 {
		t.Fatalf("report retries = %d; want 1", reportData.Limits.Retries)
	}
	if reportData.Limits.RetryBackoffMS != 25 {
		t.Fatalf("report retry_backoff_ms = %d; want 25", reportData.Limits.RetryBackoffMS)
	}

	link := findLinkByURL(reportData, server.URL+"/unstable")
	if link == nil {
		t.Fatalf("unstable link not found")
	}
	if !link.OK || link.Dead || link.Non200 || link.StatusCode != http.StatusOK || link.Error != "" {
		t.Fatalf("unstable health = ok:%v dead:%v non200:%v status:%d error:%q; want final OK", link.OK, link.Dead, link.Non200, link.StatusCode, link.Error)
	}

	fetch := findFetchByURL(reportData, server.URL+"/unstable")
	if fetch == nil {
		t.Fatalf("unstable fetch not found")
	}
	if fetch.StatusCode != http.StatusOK || fetch.Error != "" {
		t.Fatalf("unstable fetch status/error = %d/%q; want 200/empty", fetch.StatusCode, fetch.Error)
	}
}

func TestCrawlerSendsConfiguredRequestHeaders(t *testing.T) {
	var sawRequest atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawRequest.Store(true)
		if got := r.Header.Get("Authorization"); got != "Bearer secret-token" {
			t.Errorf("Authorization = %q; want Bearer secret-token", got)
		}
		if got := r.Header.Values("X-Preview"); len(got) != 2 || got[0] != "one" || got[1] != "two" {
			t.Errorf("X-Preview values = %#v; want [one two]", got)
		}
		if got := r.UserAgent(); got != "flag-agent" {
			t.Errorf("User-Agent = %q; want flag-agent", got)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<p>ok</p>"))
	}))
	defer server.Close()

	_, err := Run(context.Background(), ScanOptions{
		EntryURL:    server.URL + "/",
		MaxPages:    1,
		Timeout:     2 * time.Second,
		UserAgent:   "flag-agent",
		Concurrency: 1,
		Headers: []RequestHeader{
			{Name: "Authorization", Value: "Bearer secret-token"},
			{Name: "X-Preview", Value: "one"},
			{Name: "X-Preview", Value: "two"},
			{Name: "User-Agent", Value: "header-agent"},
		},
	})
	if err != nil {
		t.Fatalf("run scanner: %v", err)
	}
	if !sawRequest.Load() {
		t.Fatal("server did not receive request")
	}
}

func TestCrawlerReportDoesNotIncludeRequestHeaders(t *testing.T) {
	const secret = "araneae-test-secret-7d77d8b5"
	var sawSecret atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer "+secret {
			sawSecret.Store(true)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<p>ok</p>"))
	}))
	defer server.Close()

	reportData, err := Run(context.Background(), ScanOptions{
		EntryURL:    server.URL + "/",
		MaxPages:    1,
		Timeout:     2 * time.Second,
		UserAgent:   "araneae-test",
		Concurrency: 1,
		Headers: []RequestHeader{
			{Name: "Authorization", Value: "Bearer " + secret},
			{Name: "X-Araneae-Secret", Value: secret},
		},
	})
	if err != nil {
		t.Fatalf("run scanner: %v", err)
	}
	if !sawSecret.Load() {
		t.Fatal("server did not receive secret Authorization header")
	}

	var output bytes.Buffer
	if err := report.Write(&output, reportData); err != nil {
		t.Fatalf("write report: %v", err)
	}
	if strings.Contains(output.String(), secret) {
		t.Fatalf("report JSON contains secret header value")
	}
	if strings.Contains(output.String(), "Authorization") || strings.Contains(output.String(), "X-Araneae-Secret") {
		t.Fatalf("report JSON contains request header names")
	}
}

func TestCrawlerDoesNotSendHeadersAfterCrossOriginEntryRedirect(t *testing.T) {
	var targetHeaderLeaks int32
	var sawFollowup atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			atomic.AddInt32(&targetHeaderLeaks, 1)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/final":
			_, _ = w.Write([]byte(`<a href="/next">Next</a>`))
		case "/next":
			sawFollowup.Store(true)
			_, _ = w.Write([]byte("<p>next</p>"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer target.Close()

	entry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/final", http.StatusFound)
	}))
	defer entry.Close()

	_, err := Run(context.Background(), ScanOptions{
		EntryURL:    entry.URL + "/",
		MaxPages:    3,
		Timeout:     2 * time.Second,
		UserAgent:   "araneae-test",
		Concurrency: 1,
		Headers: []RequestHeader{
			{Name: "Authorization", Value: "Bearer entry-secret"},
			{Name: "Cookie", Value: "preview_session=entry-secret"},
		},
	})
	if err != nil {
		t.Fatalf("run scanner: %v", err)
	}
	if !sawFollowup.Load() {
		t.Fatal("follow-up page was not fetched")
	}
	if got := atomic.LoadInt32(&targetHeaderLeaks); got != 0 {
		t.Fatalf("cross-origin target received configured headers %d times; want 0", got)
	}
}

func TestCrawlerDoesNotSendHeadersToAllowHost(t *testing.T) {
	var sawEntryHeader atomic.Bool
	var allowedHeaderLeaks int32
	allowed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			atomic.AddInt32(&allowedHeaderLeaks, 1)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<p>allowed</p>"))
	}))
	defer allowed.Close()

	entry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer entry-secret" {
			sawEntryHeader.Store(true)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<a href="` + allowed.URL + `/allowed">Allowed</a>`))
	}))
	defer entry.Close()

	_, err := Run(context.Background(), ScanOptions{
		EntryURL:    entry.URL + "/",
		MaxPages:    2,
		Timeout:     2 * time.Second,
		UserAgent:   "araneae-test",
		Concurrency: 1,
		AllowHosts:  []string{allowed.URL},
		Headers: []RequestHeader{
			{Name: "Authorization", Value: "Bearer entry-secret"},
			{Name: "Cookie", Value: "preview_session=entry-secret"},
		},
	})
	if err != nil {
		t.Fatalf("run scanner: %v", err)
	}
	if !sawEntryHeader.Load() {
		t.Fatal("entry origin did not receive configured Authorization header")
	}
	if got := atomic.LoadInt32(&allowedHeaderLeaks); got != 0 {
		t.Fatalf("allow-host received configured headers %d times; want 0", got)
	}
}

func TestCrawlerDoesNotRetryNotFound(t *testing.T) {
	var notFoundAttempts int32
	var sleepCalls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<a href="/not-found">Missing</a>`))
		case "/not-found":
			atomic.AddInt32(&notFoundAttempts, 1)
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("missing"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	reportData, err := Run(context.Background(), ScanOptions{
		EntryURL:     server.URL + "/",
		MaxPages:     10,
		Timeout:      2 * time.Second,
		UserAgent:    "araneae-test",
		Concurrency:  1,
		Retries:      3,
		RetryBackoff: 25 * time.Millisecond,
		retrySleep: func(_ context.Context, d time.Duration) error {
			atomic.AddInt32(&sleepCalls, 1)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("run scanner: %v", err)
	}

	if got := atomic.LoadInt32(&notFoundAttempts); got != 1 {
		t.Fatalf("not found attempts = %d; want 1", got)
	}
	if got := atomic.LoadInt32(&sleepCalls); got != 0 {
		t.Fatalf("retry sleep calls = %d; want 0", got)
	}

	link := findLinkByURL(reportData, server.URL+"/not-found")
	if link == nil {
		t.Fatalf("not found link not found")
	}
	if link.OK || !link.Dead || !link.Non200 || link.Problem != "http_status" {
		t.Fatalf("not found health = ok:%v dead:%v non200:%v problem:%q; want dead non-200 http_status", link.OK, link.Dead, link.Non200, link.Problem)
	}
}

func TestCrawlerReportsRetryCycleDurationMilliseconds(t *testing.T) {
	entry := "https://docs.example.test/"
	unstableURL := "https://docs.example.test/unstable"
	fetcher := newSequencedFetcher(map[string][]FetchResult{
		entry: {
			{
				URL:         entry,
				StatusCode:  http.StatusOK,
				FinalURL:    entry,
				ContentType: "text/html; charset=utf-8",
				Body:        []byte(`<a href="/unstable">Unstable</a>`),
				Duration:    3 * time.Millisecond,
				CheckedAt:   time.Now().UTC(),
			},
		},
		unstableURL: {
			{
				URL:         unstableURL,
				StatusCode:  http.StatusServiceUnavailable,
				FinalURL:    unstableURL,
				ContentType: "text/html; charset=utf-8",
				Duration:    11 * time.Millisecond,
				CheckedAt:   time.Now().UTC(),
			},
			{
				URL:         unstableURL,
				StatusCode:  http.StatusOK,
				FinalURL:    unstableURL,
				ContentType: "text/html; charset=utf-8",
				Body:        []byte("<p>ok</p>"),
				Duration:    7 * time.Millisecond,
				CheckedAt:   time.Now().UTC(),
			},
		},
	})

	reportData, err := Run(context.Background(), ScanOptions{
		EntryURL:     entry,
		MaxPages:     10,
		Timeout:      2 * time.Second,
		UserAgent:    "araneae-test",
		Concurrency:  1,
		Fetcher:      fetcher,
		Retries:      1,
		RetryBackoff: 25 * time.Millisecond,
		retrySleep: func(_ context.Context, _ time.Duration) error {
			return nil
		},
	})
	if err != nil {
		t.Fatalf("run scanner: %v", err)
	}

	unstableFetch := findFetchByURL(reportData, unstableURL)
	if unstableFetch == nil {
		t.Fatalf("unstable fetch not found")
	}
	if unstableFetch.DurationMS != 43 {
		t.Fatalf("unstable duration_ms = %d; want 43", unstableFetch.DurationMS)
	}
	if got := fetcher.requestCount(unstableURL); got != 2 {
		t.Fatalf("unstable attempts = %d; want 2", got)
	}
}

func TestCrawlerStopsZeroBackoffRetriesWhenContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fetcher := &cancelingRetryFetcher{cancel: cancel}

	_, err := Run(ctx, ScanOptions{
		EntryURL:     "https://docs.example.test/",
		MaxPages:     1,
		Timeout:      2 * time.Second,
		UserAgent:    "araneae-test",
		Concurrency:  1,
		Fetcher:      fetcher,
		Retries:      3,
		RetryBackoff: 0,
	})
	if err == nil {
		t.Fatal("run scanner error = nil; want context cancellation")
	}
	if !strings.Contains(err.Error(), context.Canceled.Error()) {
		t.Fatalf("run scanner error = %v; want context cancellation", err)
	}
	if got := fetcher.requestCount(); got != 1 {
		t.Fatalf("fetch attempts = %d; want 1", got)
	}
}

func TestCrawlerMaxIntRetriesDoesNotOverflowBeforeFirstAttempt(t *testing.T) {
	entry := "https://docs.example.test/"
	fetcher := newSequencedFetcher(map[string][]FetchResult{
		entry: {
			{
				URL:         entry,
				StatusCode:  http.StatusOK,
				FinalURL:    entry,
				ContentType: "text/html; charset=utf-8",
				Body:        []byte("<p>ok</p>"),
				CheckedAt:   time.Now().UTC(),
			},
		},
	})

	reportData, err := Run(context.Background(), ScanOptions{
		EntryURL:     entry,
		MaxPages:     1,
		Timeout:      2 * time.Second,
		UserAgent:    "araneae-test",
		Concurrency:  1,
		Fetcher:      fetcher,
		Retries:      math.MaxInt,
		RetryBackoff: 0,
	})
	if err != nil {
		t.Fatalf("run scanner: %v", err)
	}
	if got := fetcher.requestCount(entry); got != 1 {
		t.Fatalf("entry attempts = %d; want 1", got)
	}
	if reportData.Summary.FetchesAttempted != 1 {
		t.Fatalf("fetches attempted = %d; want 1", reportData.Summary.FetchesAttempted)
	}
}

func TestSequentialCrawlerRedirectChainRecorded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<a href="/old">Old</a>`))
		case "/old":
			http.Redirect(w, r, "/new", http.StatusFound)
		case "/new":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte("<p>new</p>"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	reportData := runScan(t, server.URL+"/", 10, 1)

	if len(reportData.Fetches) != 2 {
		t.Fatalf("fetches = %d; want 2", len(reportData.Fetches))
	}
	if got := reportData.Links[0].FetchURL; !strings.Contains(got, "/old") {
		t.Fatalf("link fetch URL = %q; want /old", got)
	}
	if got := reportData.Links[0].FinalURL; !strings.Contains(got, "/new") {
		t.Fatalf("final URL = %q; want /new", got)
	}
	if got := len(reportData.Links[0].FetchURL); got == 0 {
		t.Fatal("missing link")
	}
	oldFetch := findFetchByURL(reportData, server.URL+"/old")
	if oldFetch == nil {
		t.Fatalf("old fetch missing")
	}
	if got := len(oldFetch.RedirectChain); got == 0 {
		t.Fatalf("expected redirect chain to be present")
	}
}

func TestSequentialCrawlerFragments(t *testing.T) {
	page := `<h1 id="present">Present</h1><a name="named">Named</a>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<a href="/frag#present">present</a><a href="/frag#missing">missing</a>`))
		case "/frag":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(page))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	reportData := runScan(t, server.URL+"/", 10, 1)

	if got := len(reportData.Links); got != 2 {
		t.Fatalf("links = %d; want 2", got)
	}

	missing := findLinkByURL(reportData, server.URL+"/frag#missing")
	if missing == nil {
		t.Fatalf("missing fragment link not found")
	}
	if !missing.Dead || missing.Problem != "missing_fragment" {
		t.Fatalf("missing fragment should be dead; got dead=%v problem=%q", missing.Dead, missing.Problem)
	}
	present := findLinkByURL(reportData, server.URL+"/frag#present")
	if present == nil {
		t.Fatalf("present fragment link not found")
	}
	if present.Dead {
		t.Fatalf("present fragment should be ok")
	}
}

func TestSequentialCrawlerParsesOnlyHTML(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<a href="/text">Text</a><a href="/next">Next</a>`))
		case "/text":
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = w.Write([]byte(`<a href="/ignored">ignore</a>`))
		case "/next":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<a href="/found">Found</a>`))
		case "/found":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<p>found</p>`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	reportData := runScan(t, server.URL+"/", 10, 1)

	if reportData.Summary.FetchesAttempted != 4 {
		t.Fatalf("fetches attempted = %d; want 4", reportData.Summary.FetchesAttempted)
	}
	textLink := findLinkByURL(reportData, server.URL+"/text")
	if textLink == nil || textLink.ContentType != "text/plain; charset=utf-8" {
		t.Fatalf("expected text link to be recorded with plain text content type")
	}
	if findLinkByURL(reportData, server.URL+"/ignored") != nil {
		t.Fatalf("did parse anchors from non-HTML response")
	}
}

func TestSequentialCrawlerReportsTooLargeHTMLResponseAsDead(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<a href="/large">Large</a>`))
		case "/large":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(strings.Repeat("x", 65)))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	reportData, err := Run(context.Background(), ScanOptions{
		EntryURL:         server.URL + "/",
		MaxPages:         10,
		Timeout:          2 * time.Second,
		UserAgent:        "araneae-test",
		Concurrency:      1,
		MaxResponseBytes: 64,
	})
	if err != nil {
		t.Fatalf("run scanner: %v", err)
	}

	if reportData.Limits.MaxResponseBytes != 64 {
		t.Fatalf("max response bytes = %d; want 64", reportData.Limits.MaxResponseBytes)
	}
	if reportData.Summary.DeadLinks != 1 {
		t.Fatalf("dead links = %d; want 1", reportData.Summary.DeadLinks)
	}
	if reportData.Summary.OKLinks != 0 {
		t.Fatalf("ok links = %d; want 0", reportData.Summary.OKLinks)
	}
	if reportData.Summary.Non200Links != 0 {
		t.Fatalf("non-200 links = %d; want 0", reportData.Summary.Non200Links)
	}

	link := findLinkByURL(reportData, server.URL+"/large")
	if link == nil {
		t.Fatalf("large link not found")
	}
	if !link.Dead || link.OK || link.Non200 {
		t.Fatalf("link health = dead:%v ok:%v non200:%v; want dead only", link.Dead, link.OK, link.Non200)
	}
	if link.Problem != problemResponseTooLarge || link.Error != problemResponseTooLarge {
		t.Fatalf("link problem/error = %q/%q; want %q", link.Problem, link.Error, problemResponseTooLarge)
	}

	fetch := findFetchByURL(reportData, server.URL+"/large")
	if fetch == nil {
		t.Fatalf("large fetch not found")
	}
	if fetch.Error != problemResponseTooLarge {
		t.Fatalf("fetch error = %q; want %q", fetch.Error, problemResponseTooLarge)
	}
}

func TestSequentialCrawlerRecordsNonHTMLStatusWithoutParsing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<a href="/file.pdf">PDF</a><a href="/download.bin">Download</a>`))
		case "/file.pdf":
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write([]byte(`<a href="/ignored">ignored</a>`))
		case "/download.bin":
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(strings.Repeat("x", 128)))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	reportData := runScan(t, server.URL+"/", 10, 1)

	pdf := findLinkByURL(reportData, server.URL+"/file.pdf")
	if pdf == nil {
		t.Fatalf("pdf link not found")
	}
	if !pdf.OK || pdf.Dead || pdf.Non200 {
		t.Fatalf("pdf health = ok:%v dead:%v non200:%v; want ok only", pdf.OK, pdf.Dead, pdf.Non200)
	}
	if pdf.ContentType != "application/pdf" {
		t.Fatalf("pdf content type = %q", pdf.ContentType)
	}

	download := findLinkByURL(reportData, server.URL+"/download.bin")
	if download == nil {
		t.Fatalf("download link not found")
	}
	if download.OK || download.Dead || !download.Non200 || download.Problem != "http_status" {
		t.Fatalf("download health = ok:%v dead:%v non200:%v problem:%q; want non-200 http_status", download.OK, download.Dead, download.Non200, download.Problem)
	}
	if download.StatusCode != http.StatusInternalServerError {
		t.Fatalf("download status = %d; want 500", download.StatusCode)
	}
	if download.ContentType != "application/octet-stream" {
		t.Fatalf("download content type = %q", download.ContentType)
	}
	if findLinkByURL(reportData, server.URL+"/ignored") != nil {
		t.Fatalf("did parse anchors from non-HTML response")
	}
}

func TestCrawlerReportsFetchDurationMilliseconds(t *testing.T) {
	entry := "https://docs.example.test/"
	pageURL := "https://docs.example.test/page"
	fetcher := newScriptedFetcher(map[string]scriptedFetch{
		entry: {
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body:        `<a href="/page">Page</a>`,
			duration:    1500 * time.Millisecond,
		},
		pageURL: {
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body:        "<p>ok</p>",
			duration:    42 * time.Millisecond,
		},
	}, 0)

	reportData := runScanWithFetcher(t, entry, 10, 1, fetcher)

	entryFetch := findFetchByURL(reportData, entry)
	if entryFetch == nil {
		t.Fatalf("entry fetch not found")
	}
	if entryFetch.DurationMS != 1500 {
		t.Fatalf("entry duration_ms = %d; want 1500", entryFetch.DurationMS)
	}

	pageFetch := findFetchByURL(reportData, pageURL)
	if pageFetch == nil {
		t.Fatalf("page fetch not found")
	}
	if pageFetch.DurationMS != 42 {
		t.Fatalf("page duration_ms = %d; want 42", pageFetch.DurationMS)
	}
}

func TestCrawlerSeedsLocalRootHTMLPages(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "index.html"), "<p>home</p>")
	writeTestFile(t, filepath.Join(root, "orphan.html"), "<p>orphan</p>")
	writeTestFile(t, filepath.Join(root, "nested", "index.html"), "<p>nested</p>")
	writeTestFile(t, filepath.Join(root, "assets", "notes.txt"), "ignored")

	entry := "https://docs.example.test/docs/"
	orphanURL := "https://docs.example.test/docs/orphan.html"
	nestedURL := "https://docs.example.test/docs/nested/"
	brokenURL := "https://docs.example.test/docs/broken"
	fetcher := newScriptedFetcher(map[string]scriptedFetch{
		entry: {
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body:        `<p>entry has no orphan link</p>`,
		},
		orphanURL: {
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body:        `<a href="/docs/broken">Broken</a>`,
		},
		nestedURL: {
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body:        `<p>nested orphan</p>`,
		},
		brokenURL: {
			status:      http.StatusNotFound,
			contentType: "text/html; charset=utf-8",
			body:        "missing",
		},
	}, 0)

	reportData, err := Run(context.Background(), ScanOptions{
		EntryURL:    entry,
		MaxPages:    10,
		Timeout:     2 * time.Second,
		UserAgent:   "araneae-test",
		Concurrency: 2,
		Fetcher:     fetcher,
		PathPrefix:  "/docs/",
		LocalRoot:   root,
	})
	if err != nil {
		t.Fatalf("run scanner: %v", err)
	}

	if findFetchByURL(reportData, orphanURL) == nil {
		t.Fatalf("orphan HTML page was not fetched")
	}
	if findFetchByURL(reportData, nestedURL) == nil {
		t.Fatalf("nested index HTML page was not fetched")
	}
	if findFetchByURL(reportData, "https://docs.example.test/docs/assets/notes.txt") != nil {
		t.Fatalf("non-HTML local file was fetched")
	}
	broken := findLinkByURL(reportData, brokenURL)
	if broken == nil {
		t.Fatalf("link from orphan page was not discovered")
	}
	if !broken.Dead || broken.Problem != "http_status" {
		t.Fatalf("broken link health = dead:%v problem:%q; want dead http_status", broken.Dead, broken.Problem)
	}
	if len(broken.Sources) != 1 || broken.Sources[0].PageURL != orphanURL {
		t.Fatalf("broken link sources = %#v; want orphan source", broken.Sources)
	}
}

func TestCrawlerConcurrentFetches(t *testing.T) {
	entry := "https://docs.example.test/"
	fetcher := newScriptedFetcher(map[string]scriptedFetch{
		entry: {
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body:        `<a href="/a">A</a><a href="/b">B</a><a href="/c">C</a>`,
		},
		"https://docs.example.test/a": {
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body:        "<p>alpha</p>",
		},
		"https://docs.example.test/b": {
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body:        "<p>bravo</p>",
		},
		"https://docs.example.test/c": {
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body:        "<p>charlie</p>",
		},
	}, 50*time.Millisecond)

	reportData := runScanWithFetcher(t, entry, 4, 3, fetcher)

	if reportData.Summary.FetchesAttempted != 4 {
		t.Fatalf("fetches attempted = %d; want 4", reportData.Summary.FetchesAttempted)
	}
	if got := fetcher.maxConcurrent(); got < 2 {
		t.Fatalf("max concurrent fetches = %d; want >= 2", got)
	}
}

func TestCrawlerMaxPagesRespectsConcurrencyLimit(t *testing.T) {
	entry := "https://docs.example.test/"
	fetcher := newScriptedFetcher(map[string]scriptedFetch{
		entry: {
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body:        `<a href="/a">A</a><a href="/b">B</a><a href="/c">C</a>`,
		},
		"https://docs.example.test/a": {
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body:        "<p>alpha</p>",
		},
		"https://docs.example.test/b": {
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body:        "<p>bravo</p>",
		},
		"https://docs.example.test/c": {
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body:        "<p>charlie</p>",
		},
	}, 50*time.Millisecond)

	reportData := runScanWithFetcher(t, entry, 2, 3, fetcher)

	if !reportData.Summary.Truncated {
		t.Fatal("expected report to be truncated")
	}
	if reportData.Summary.FetchesAttempted != 2 {
		t.Fatalf("fetches attempted = %d; want 2", reportData.Summary.FetchesAttempted)
	}
	if reportData.Summary.UnvisitedURLs != 2 {
		t.Fatalf("unvisited urls = %d; want 2", reportData.Summary.UnvisitedURLs)
	}
}

func TestCrawlerAvoidsDuplicateFetchesWithConcurrency(t *testing.T) {
	entry := "https://docs.example.test/"
	fetcher := newScriptedFetcher(map[string]scriptedFetch{
		entry: {
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body:        `<a href="/a">A</a><a href="/b">B</a>`,
		},
		"https://docs.example.test/a": {
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body:        `<a href="/shared">shared</a><a href="/b">to-b</a>`,
		},
		"https://docs.example.test/b": {
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body:        `<a href="/shared">shared</a><a href="/a">to-a</a>`,
		},
		"https://docs.example.test/shared": {
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body:        "<p>shared page</p>",
		},
	}, 0)

	reportData := runScanWithFetcher(t, entry, 10, 3, fetcher)

	if reportData.Summary.FetchesAttempted != 4 {
		t.Fatalf("fetches attempted = %d; want 4", reportData.Summary.FetchesAttempted)
	}
	if got := len(reportData.Fetches); got != 4 {
		t.Fatalf("fetch records = %d; want 4", got)
	}
}

func TestCrawlerRequestGateAppliesAcrossConcurrentWorkers(t *testing.T) {
	entry := "https://docs.example.test/"
	fetcher := newScriptedFetcher(map[string]scriptedFetch{
		entry: {
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body:        `<a href="/a">A</a><a href="/b">B</a><a href="/c">C</a>`,
		},
		"https://docs.example.test/a": {
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body:        "<p>alpha</p>",
		},
		"https://docs.example.test/b": {
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body:        "<p>bravo</p>",
		},
		"https://docs.example.test/c": {
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body:        "<p>charlie</p>",
		},
	}, 0)
	gate := newManualRequestGate()

	resultCh := make(chan report.Report, 1)
	errCh := make(chan error, 1)
	go func() {
		result, err := Run(context.Background(), ScanOptions{
			EntryURL:             entry,
			MaxPages:             4,
			Timeout:              2 * time.Second,
			UserAgent:            "araneae-test",
			Concurrency:          3,
			MaxRequestsPerSecond: 10,
			Fetcher:              fetcher,
			requestGate:          gate,
		})
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- result
	}()

	gate.waitForCalls(t, 1)
	if got := fetcher.requestCount(); got != 0 {
		t.Fatalf("fetches started before entry gate release = %d; want 0", got)
	}
	gate.releaseOne()
	waitUntil(t, func() bool { return fetcher.requestCount() == 1 }, "entry fetch to start")

	gate.waitForCalls(t, 4)
	if got := fetcher.requestCount(); got != 1 {
		t.Fatalf("fetches started before worker gate release = %d; want 1", got)
	}

	gate.releaseOne()
	waitUntil(t, func() bool { return fetcher.requestCount() == 2 }, "first worker fetch to start")

	gate.releaseN(2)
	var reportData report.Report
	select {
	case err := <-errCh:
		t.Fatalf("run scanner: %v", err)
	case reportData = <-resultCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for scanner")
	}

	if reportData.Summary.FetchesAttempted != 4 {
		t.Fatalf("fetches attempted = %d; want 4", reportData.Summary.FetchesAttempted)
	}
	if reportData.Limits.MaxRequestsPerSecond != 10 {
		t.Fatalf("max requests per second = %f; want 10", reportData.Limits.MaxRequestsPerSecond)
	}
	if got := gate.callCount(); got != 4 {
		t.Fatalf("request gate calls = %d; want 4", got)
	}
}

func runScan(t *testing.T, entry string, maxPages int, concurrency int) report.Report {
	return runScanWithFetcher(t, entry, maxPages, concurrency, nil)
}

func runScanWithFetcher(t *testing.T, entry string, maxPages int, concurrency int, fetcher Fetcher) report.Report {
	t.Helper()
	result, err := Run(context.Background(), ScanOptions{
		EntryURL:    entry,
		MaxPages:    maxPages,
		Timeout:     2 * time.Second,
		UserAgent:   "araneae-test",
		Concurrency: concurrency,
		Fetcher:     fetcher,
		PathPrefix:  "",
	})
	if err != nil {
		t.Fatalf("run scanner: %v", err)
	}
	return result
}

func writeTestFile(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create test dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}
}

type sequencedFetcher struct {
	mu      sync.Mutex
	results map[string][]FetchResult
	calls   map[string]int
}

func newSequencedFetcher(results map[string][]FetchResult) *sequencedFetcher {
	return &sequencedFetcher{
		results: results,
		calls:   map[string]int{},
	}
}

func (f *sequencedFetcher) Fetch(_ context.Context, fetchURL string) (FetchResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	call := f.calls[fetchURL]
	f.calls[fetchURL] = call + 1

	sequence := f.results[fetchURL]
	if len(sequence) == 0 {
		return FetchResult{
			URL:         fetchURL,
			StatusCode:  http.StatusNotFound,
			FinalURL:    fetchURL,
			ContentType: "text/html; charset=utf-8",
			CheckedAt:   time.Now().UTC(),
		}, nil
	}
	if call >= len(sequence) {
		call = len(sequence) - 1
	}
	result := sequence[call]
	if result.URL == "" {
		result.URL = fetchURL
	}
	if result.FinalURL == "" {
		result.FinalURL = fetchURL
	}
	result.RedirectChain = append([]string{}, result.RedirectChain...)
	result.Body = append([]byte{}, result.Body...)
	return result, nil
}

func (f *sequencedFetcher) requestCount(fetchURL string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[fetchURL]
}

type cancelingRetryFetcher struct {
	cancel   context.CancelFunc
	requests int32
}

func (f *cancelingRetryFetcher) Fetch(_ context.Context, fetchURL string) (FetchResult, error) {
	if atomic.AddInt32(&f.requests, 1) == 1 {
		f.cancel()
	}
	return FetchResult{
		URL:       fetchURL,
		FinalURL:  fetchURL,
		Error:     "network_error",
		CheckedAt: time.Now().UTC(),
	}, nil
}

func (f *cancelingRetryFetcher) requestCount() int {
	return int(atomic.LoadInt32(&f.requests))
}

type scriptedFetch struct {
	status      int
	contentType string
	body        string
	finalURL    string
	redirects   []string
	duration    time.Duration
}

type scriptedFetcher struct {
	mu          sync.RWMutex
	pages       map[string]scriptedFetch
	delay       time.Duration
	requests    int32
	inFlight    int32
	maxInFlight int32
}

func newScriptedFetcher(pages map[string]scriptedFetch, delay time.Duration) *scriptedFetcher {
	return &scriptedFetcher{
		pages: pages,
		delay: delay,
	}
}

func (f *scriptedFetcher) Fetch(_ context.Context, fetchURL string) (FetchResult, error) {
	atomic.AddInt32(&f.requests, 1)
	current := atomic.AddInt32(&f.inFlight, 1)
	for {
		observed := atomic.LoadInt32(&f.maxInFlight)
		if current <= observed {
			break
		}
		if atomic.CompareAndSwapInt32(&f.maxInFlight, observed, current) {
			break
		}
	}
	defer atomic.AddInt32(&f.inFlight, -1)

	if f.delay > 0 {
		time.Sleep(f.delay)
	}

	f.mu.RLock()
	page, ok := f.pages[fetchURL]
	f.mu.RUnlock()

	if !ok {
		return FetchResult{
			URL:         fetchURL,
			StatusCode:  http.StatusNotFound,
			ContentType: "text/html; charset=utf-8",
			CheckedAt:   time.Now().UTC(),
		}, nil
	}

	finalURL := page.finalURL
	if finalURL == "" {
		finalURL = fetchURL
	}

	return FetchResult{
		URL:           fetchURL,
		StatusCode:    page.status,
		FinalURL:      finalURL,
		ContentType:   page.contentType,
		Body:          []byte(page.body),
		RedirectChain: append([]string{}, page.redirects...),
		CheckedAt:     time.Now().UTC(),
		Duration:      page.duration,
	}, nil
}

func (f *scriptedFetcher) maxConcurrent() int {
	return int(atomic.LoadInt32(&f.maxInFlight))
}

func (f *scriptedFetcher) requestCount() int {
	return int(atomic.LoadInt32(&f.requests))
}

type manualRequestGate struct {
	releases chan struct{}
	calls    int32
}

func newManualRequestGate() *manualRequestGate {
	return &manualRequestGate{
		releases: make(chan struct{}, 16),
	}
}

func (g *manualRequestGate) Wait(ctx context.Context) error {
	atomic.AddInt32(&g.calls, 1)
	select {
	case <-g.releases:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (g *manualRequestGate) waitForCalls(t *testing.T, want int) {
	t.Helper()
	waitUntil(t, func() bool {
		return g.callCount() >= want
	}, "request gate calls")
}

func (g *manualRequestGate) callCount() int {
	return int(atomic.LoadInt32(&g.calls))
}

func (g *manualRequestGate) releaseOne() {
	g.releases <- struct{}{}
}

func (g *manualRequestGate) releaseN(count int) {
	for range count {
		g.releaseOne()
	}
}

func waitUntil(t *testing.T, condition func() bool, description string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", description)
}

func findLinkByURL(r report.Report, url string) *report.LinkResult {
	copy := func(link report.LinkResult) *report.LinkResult {
		return &link
	}
	for _, link := range r.Links {
		if link.URL == url {
			return copy(link)
		}
	}
	return nil
}

func findFetchByURL(r report.Report, url string) *report.FetchResult {
	copy := func(fetch report.FetchResult) *report.FetchResult {
		return &fetch
	}
	for _, fetch := range r.Fetches {
		if fetch.URL == url {
			return copy(fetch)
		}
	}
	return nil
}
