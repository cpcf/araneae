package crawl

import (
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestHTTPFetcherLimitsHTMLResponseBody(t *testing.T) {
	fetcher := &HTTPFetcher{
		userAgent:        "araneae-test",
		maxResponseBytes: 5,
		client: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				headers := make(http.Header)
				headers.Set("Content-Type", "text/html; charset=utf-8")
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     headers,
					Body:       io.NopCloser(strings.NewReader("123456")),
					Request:    req,
				}, nil
			}),
		},
	}

	result, err := fetcher.Fetch(context.Background(), "https://docs.example.test/large")
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if result.Error != problemResponseTooLarge {
		t.Fatalf("error = %q; want %q", result.Error, problemResponseTooLarge)
	}
	if result.StatusCode != http.StatusOK {
		t.Fatalf("status code = %d; want 200", result.StatusCode)
	}
	if result.FinalURL != "https://docs.example.test/large" {
		t.Fatalf("final URL = %q", result.FinalURL)
	}
	if result.ContentType != "text/html; charset=utf-8" {
		t.Fatalf("content type = %q", result.ContentType)
	}
	if len(result.Body) != 0 {
		t.Fatalf("body length = %d; want 0 for oversized response", len(result.Body))
	}
}

func TestHTTPFetcherSkipsNonHTMLResponseBody(t *testing.T) {
	body := &readTrackingBody{}
	fetcher := &HTTPFetcher{
		userAgent:        "araneae-test",
		maxResponseBytes: 5,
		client: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				headers := make(http.Header)
				headers.Set("Content-Type", "application/pdf")
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     headers,
					Body:       body,
					Request:    req,
				}, nil
			}),
		},
	}

	result, err := fetcher.Fetch(context.Background(), "https://docs.example.test/file.pdf")
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if result.Error != "" {
		t.Fatalf("error = %q; want empty", result.Error)
	}
	if result.StatusCode != http.StatusOK {
		t.Fatalf("status code = %d; want 200", result.StatusCode)
	}
	if result.FinalURL != "https://docs.example.test/file.pdf" {
		t.Fatalf("final URL = %q", result.FinalURL)
	}
	if result.ContentType != "application/pdf" {
		t.Fatalf("content type = %q", result.ContentType)
	}
	if result.Body != nil {
		t.Fatalf("body = %q; want nil", string(result.Body))
	}
	if body.reads != 0 {
		t.Fatalf("body reads = %d; want 0", body.reads)
	}
	if !body.closed {
		t.Fatal("body was not closed")
	}
}

func TestHTTPFetcherSkipsNonOKHTMLResponseBody(t *testing.T) {
	body := &readTrackingBody{}
	fetcher := &HTTPFetcher{
		userAgent:        "araneae-test",
		maxResponseBytes: 5,
		client: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				headers := make(http.Header)
				headers.Set("Content-Type", "text/html; charset=utf-8")
				return &http.Response{
					StatusCode: http.StatusInternalServerError,
					Header:     headers,
					Body:       body,
					Request:    req,
				}, nil
			}),
		},
	}

	result, err := fetcher.Fetch(context.Background(), "https://docs.example.test/error")
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if result.Error != "" {
		t.Fatalf("error = %q; want empty", result.Error)
	}
	if result.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status code = %d; want 500", result.StatusCode)
	}
	if result.FinalURL != "https://docs.example.test/error" {
		t.Fatalf("final URL = %q", result.FinalURL)
	}
	if result.ContentType != "text/html; charset=utf-8" {
		t.Fatalf("content type = %q", result.ContentType)
	}
	if result.Body != nil {
		t.Fatalf("body = %q; want nil", string(result.Body))
	}
	if body.reads != 0 {
		t.Fatalf("body reads = %d; want 0", body.reads)
	}
	if !body.closed {
		t.Fatal("body was not closed")
	}
}

func TestHTTPFetcherRecordsDurationForCompletedRequest(t *testing.T) {
	start := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	fetcher := &HTTPFetcher{
		userAgent:        "araneae-test",
		maxResponseBytes: 1024,
		now:              sequenceClock(start, start.Add(1375*time.Millisecond)),
		client: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				headers := make(http.Header)
				headers.Set("Content-Type", "text/html; charset=utf-8")
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     headers,
					Body:       io.NopCloser(strings.NewReader("<p>ok</p>")),
					Request:    req,
				}, nil
			}),
		},
	}

	result, err := fetcher.Fetch(context.Background(), "https://docs.example.test/")
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if result.Duration != 1375*time.Millisecond {
		t.Fatalf("duration = %s; want 1.375s", result.Duration)
	}
	if !result.CheckedAt.Equal(start.Add(1375 * time.Millisecond)) {
		t.Fatalf("checked_at = %s; want %s", result.CheckedAt, start.Add(1375*time.Millisecond))
	}
}

func TestHTTPFetcherRecordsDurationForFetchError(t *testing.T) {
	start := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	fetcher := &HTTPFetcher{
		userAgent: "araneae-test",
		now:       sequenceClock(start, start.Add(25*time.Millisecond)),
		client: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return nil, errors.New("dial failed")
			}),
		},
	}

	result, err := fetcher.Fetch(context.Background(), "https://docs.example.test/")
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if result.Error != "network_error" {
		t.Fatalf("error = %q; want network_error", result.Error)
	}
	if result.Duration != 25*time.Millisecond {
		t.Fatalf("duration = %s; want 25ms", result.Duration)
	}
}

func TestReadResponseBodyAllowsMaxInt64Limit(t *testing.T) {
	body, tooLarge, err := readResponseBody(strings.NewReader("<p>ok</p>"), math.MaxInt64)
	if err != nil {
		t.Fatalf("readResponseBody() error = %v", err)
	}
	if tooLarge {
		t.Fatal("readResponseBody() tooLarge = true; want false")
	}
	if string(body) != "<p>ok</p>" {
		t.Fatalf("body = %q; want <p>ok</p>", string(body))
	}
}

func TestHTTPFetcherKeepsConfiguredHeadersOnSameOriginRedirect(t *testing.T) {
	var sawFinal atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/start":
			http.Redirect(w, r, "/final", http.StatusFound)
		case "/final":
			sawFinal.Store(true)
			if got := r.Header.Get("Authorization"); got != "Bearer same-origin" {
				t.Errorf("Authorization = %q; want Bearer same-origin", got)
			}
			if got := r.Header.Get("X-Preview-Token"); got != "same-origin" {
				t.Errorf("X-Preview-Token = %q; want same-origin", got)
			}
			if got := r.UserAgent(); got != "flag-agent" {
				t.Errorf("User-Agent = %q; want flag-agent", got)
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte("<p>ok</p>"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	fetcher := NewHTTPFetcher(2*time.Second, "flag-agent", 1024, []RequestHeader{
		{Name: "Authorization", Value: "Bearer same-origin"},
		{Name: "X-Preview-Token", Value: "same-origin"},
		{Name: "User-Agent", Value: "header-agent"},
	})
	result, err := fetcher.Fetch(context.Background(), server.URL+"/start")
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if result.Error != "" || result.StatusCode != http.StatusOK {
		t.Fatalf("result status/error = %d/%q; want 200/empty", result.StatusCode, result.Error)
	}
	if !sawFinal.Load() {
		t.Fatal("final redirect target was not requested")
	}
}

func TestHTTPFetcherDropsConfiguredHeadersOnCrossOriginRedirect(t *testing.T) {
	var sawTarget atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawTarget.Store(true)
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("Authorization = %q; want empty", got)
		}
		if got := r.Header.Get("X-Preview-Token"); got != "" {
			t.Errorf("X-Preview-Token = %q; want empty", got)
		}
		if got := r.UserAgent(); got != "flag-agent" {
			t.Errorf("User-Agent = %q; want flag-agent", got)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<p>ok</p>"))
	}))
	defer target.Close()

	start := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/final", http.StatusFound)
	}))
	defer start.Close()

	fetcher := NewHTTPFetcher(2*time.Second, "flag-agent", 1024, []RequestHeader{
		{Name: "Authorization", Value: "Bearer cross-origin"},
		{Name: "X-Preview-Token", Value: "cross-origin"},
		{Name: "User-Agent", Value: "header-agent"},
	})
	result, err := fetcher.Fetch(context.Background(), start.URL+"/start")
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if result.Error != "" || result.StatusCode != http.StatusOK {
		t.Fatalf("result status/error = %d/%q; want 200/empty", result.StatusCode, result.Error)
	}
	if !sawTarget.Load() {
		t.Fatal("cross-origin redirect target was not requested")
	}
}

func TestRetryableFetchResult(t *testing.T) {
	tests := []struct {
		name   string
		result FetchResult
		want   bool
	}{
		{
			name:   "network error",
			result: FetchResult{Error: "network_error"},
			want:   true,
		},
		{
			name:   "timeout",
			result: FetchResult{Error: "timeout"},
			want:   true,
		},
		{
			name:   "too many requests",
			result: FetchResult{StatusCode: http.StatusTooManyRequests},
			want:   true,
		},
		{
			name:   "server error",
			result: FetchResult{StatusCode: http.StatusServiceUnavailable},
			want:   true,
		},
		{
			name:   "not found",
			result: FetchResult{StatusCode: http.StatusNotFound},
			want:   false,
		},
		{
			name:   "gone",
			result: FetchResult{StatusCode: http.StatusGone},
			want:   false,
		},
		{
			name:   "response too large",
			result: FetchResult{StatusCode: http.StatusOK, Error: problemResponseTooLarge},
			want:   false,
		},
		{
			name:   "parsing error",
			result: FetchResult{StatusCode: http.StatusOK, Error: "parsing_error: broken html"},
			want:   false,
		},
		{
			name:   "tls error",
			result: FetchResult{Error: "tls_error"},
			want:   false,
		},
		{
			name:   "ok",
			result: FetchResult{StatusCode: http.StatusOK},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isRetryableFetchResult(tt.result); got != tt.want {
				t.Fatalf("isRetryableFetchResult() = %v; want %v", got, tt.want)
			}
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

type readTrackingBody struct {
	reads  int
	closed bool
}

func (b *readTrackingBody) Read(_ []byte) (int, error) {
	b.reads++
	return 0, errors.New("body should not be read")
}

func (b *readTrackingBody) Close() error {
	b.closed = true
	return nil
}

func sequenceClock(times ...time.Time) func() time.Time {
	index := 0
	return func() time.Time {
		if index >= len(times) {
			return times[len(times)-1]
		}
		current := times[index]
		index++
		return current
	}
}
