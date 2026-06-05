package crawl

import (
	"context"
	"crypto/x509"
	"errors"
	"io"
	"math"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type FetchResult struct {
	URL           string
	StatusCode    int
	FinalURL      string
	ContentType   string
	Error         string
	RedirectChain []string
	CheckedAt     time.Time
	Duration      time.Duration
	Body          []byte
}

type Fetcher interface {
	Fetch(ctx context.Context, url string) (FetchResult, error)
}

type RequestHeader struct {
	Name  string
	Value string
}

type HTTPFetcher struct {
	client           *http.Client
	userAgent        string
	maxResponseBytes int64
	headers          []RequestHeader
	headerOrigin     *url.URL
	now              func() time.Time
}

var errTooManyRedirects = errors.New("too many redirects")

const problemResponseTooLarge = "response_too_large"

func NewHTTPFetcher(timeout time.Duration, userAgent string, maxResponseBytes int64, headers []RequestHeader, headerOrigin string) *HTTPFetcher {
	return &HTTPFetcher{
		userAgent:        userAgent,
		maxResponseBytes: maxResponseBytes,
		headers:          append([]RequestHeader{}, headers...),
		headerOrigin:     parseHeaderOrigin(headerOrigin),
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

func (f *HTTPFetcher) Fetch(ctx context.Context, fetchURL string) (FetchResult, error) {
	startedAt := f.nowUTC()
	result := FetchResult{
		URL:       fetchURL,
		CheckedAt: startedAt,
	}
	finish := func() FetchResult {
		finishedAt := f.nowUTC()
		result.CheckedAt = finishedAt
		result.Duration = finishedAt.Sub(startedAt)
		if result.Duration < 0 {
			result.Duration = 0
		}
		return result
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, fetchURL, nil)
	if err != nil {
		result.Error = "network_error"
		return finish(), nil
	}
	f.addConfiguredHeaders(request)
	request.Header.Set("User-Agent", f.userAgent)

	redirects := []string{}
	client := *f.client
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		redirects = append(redirects, req.URL.String())
		if len(via) >= 10 {
			return errTooManyRedirects
		}
		f.dropConfiguredHeadersOnCrossOriginRedirect(req, via)
		return nil
	}

	response, err := client.Do(request)
	result.RedirectChain = append([]string{}, redirects...)
	if err != nil {
		result.Error = classifyFetchError(err)
		return finish(), nil
	}
	defer response.Body.Close()

	result.StatusCode = response.StatusCode
	result.FinalURL = response.Request.URL.String()
	result.ContentType = response.Header.Get("Content-Type")

	if result.StatusCode != http.StatusOK || !isHTMLContentType(result.ContentType) {
		return finish(), nil
	}

	body, tooLarge, err := readResponseBody(response.Body, f.maxResponseBytes)
	if err != nil {
		result.Error = "network_error"
		return finish(), nil
	}
	if tooLarge {
		result.Error = problemResponseTooLarge
		return finish(), nil
	}
	result.Body = body
	return finish(), nil
}

func parseHeaderOrigin(raw string) *url.URL {
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil
	}
	normalized := normalizeURL(parsed)
	if normalized.Scheme != "http" && normalized.Scheme != "https" || normalized.Host == "" {
		return nil
	}
	return normalized
}

func (f *HTTPFetcher) addConfiguredHeaders(req *http.Request) {
	if f.headerOrigin == nil || !sameOrigin(req.URL, f.headerOrigin) {
		return
	}
	for _, header := range f.headers {
		req.Header.Add(header.Name, header.Value)
	}
}

func (f *HTTPFetcher) dropConfiguredHeadersOnCrossOriginRedirect(req *http.Request, via []*http.Request) {
	if len(via) == 0 || sameOrigin(req.URL, via[0].URL) {
		return
	}
	for _, header := range f.headers {
		req.Header.Del(header.Name)
	}
	req.Header.Set("User-Agent", f.userAgent)
}

func (f *HTTPFetcher) nowUTC() time.Time {
	if f.now != nil {
		return f.now().UTC()
	}
	return time.Now().UTC()
}

func readResponseBody(body io.Reader, maxBytes int64) ([]byte, bool, error) {
	if maxBytes <= 0 {
		read, err := io.ReadAll(body)
		return read, false, err
	}
	if maxBytes == math.MaxInt64 {
		read, err := io.ReadAll(body)
		return read, false, err
	}

	read, err := io.ReadAll(io.LimitReader(body, maxBytes+1))
	if err != nil {
		return nil, false, err
	}
	if int64(len(read)) > maxBytes {
		return nil, true, nil
	}
	return read, false, nil
}

func isHTMLContentType(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err == nil {
		return strings.EqualFold(mediaType, "text/html")
	}
	return strings.Contains(strings.ToLower(contentType), "text/html")
}

func isRetryableFetchResult(result FetchResult) bool {
	if result.Error != "" {
		return result.Error == "network_error" || result.Error == "timeout"
	}
	return result.StatusCode == http.StatusTooManyRequests || (result.StatusCode >= 500 && result.StatusCode <= 599)
}

func classifyFetchError(err error) string {
	if errors.Is(err, errTooManyRedirects) {
		return "too_many_redirects"
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		if isTimeoutErr(urlErr.Err) {
			return "timeout"
		}
		if _, ok := urlErr.Err.(x509.UnknownAuthorityError); ok {
			return "tls_error"
		}
		if _, ok := urlErr.Err.(x509.HostnameError); ok {
			return "tls_error"
		}
		if strings.Contains(strings.ToLower(urlErr.Err.Error()), "certificate") {
			return "tls_error"
		}
	}
	return "network_error"
}

func isTimeoutErr(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		return true
	}
	return false
}
