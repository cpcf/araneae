package crawl

import (
	"context"
	"crypto/x509"
	"errors"
	"io"
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
	Body          []byte
}

type Fetcher interface {
	Fetch(ctx context.Context, url string) (FetchResult, error)
}

type HTTPFetcher struct {
	client    *http.Client
	userAgent string
}

var errTooManyRedirects = errors.New("too many redirects")

func NewHTTPFetcher(timeout time.Duration, userAgent string) *HTTPFetcher {
	return &HTTPFetcher{
		userAgent: userAgent,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

func (f *HTTPFetcher) Fetch(ctx context.Context, fetchURL string) (FetchResult, error) {
	result := FetchResult{
		URL:       fetchURL,
		CheckedAt: time.Now().UTC(),
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, fetchURL, nil)
	if err != nil {
		result.Error = "network_error"
		return result, nil
	}
	request.Header.Set("User-Agent", f.userAgent)

	redirects := []string{}
	client := f.client
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		redirects = append(redirects, req.URL.String())
		if len(via) >= 10 {
			return errTooManyRedirects
		}
		return nil
	}

	response, err := client.Do(request)
	result.RedirectChain = append([]string{}, redirects...)
	result.CheckedAt = time.Now().UTC()
	if err != nil {
		result.Error = classifyFetchError(err)
		return result, nil
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		result.Error = "network_error"
		return result, nil
	}
	result.StatusCode = response.StatusCode
	result.FinalURL = response.Request.URL.String()
	result.ContentType = response.Header.Get("Content-Type")
	result.Body = body
	return result, nil
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
