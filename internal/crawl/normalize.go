package crawl

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

var (
	ErrEmptyHref         = errors.New("empty href")
	ErrUnsupportedScheme = errors.New("unsupported href scheme")
)

type NormalizedLink struct {
	LinkURL  string
	FetchURL string
}

type Scope struct {
	Origin        *url.URL
	AllowedOrigin []*url.URL
	PathPrefix    string
}

type ScopeDecision struct {
	Allowed bool
	Reason  string
}

const (
	ScopeReasonExternalOrigin    = "external_origin"
	ScopeReasonOutsidePathPrefix = "outside_path_prefix"
)

func normalizeOrigin(urlString string) (string, error) {
	parsed, err := url.Parse(urlString)
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("origin must have scheme and host: %q", urlString)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("unsupported scheme: %q", parsed.Scheme)
	}
	normalized := normalizeURL(parsed)
	normalized.Path = ""
	normalized.RawPath = ""
	normalized.ForceQuery = false
	normalized.RawQuery = ""
	normalized.Fragment = ""

	return normalized.String(), nil
}

func NewScope(entryOrigin string, allowed []string, pathPrefix string) (*Scope, error) {
	origin, err := url.Parse(entryOrigin)
	if err != nil {
		return nil, err
	}
	origin = normalizeURL(origin)
	if origin.Scheme != "http" && origin.Scheme != "https" || origin.Host == "" {
		return nil, fmt.Errorf("entry origin must include http/https scheme and host: %q", entryOrigin)
	}
	origin.Path = ""
	origin.RawPath = ""
	origin.RawQuery = ""
	origin.Fragment = ""

	allowedURLs := make([]*url.URL, 0, len(allowed))
	for _, raw := range allowed {
		normalized, err := normalizeOrigin(raw)
		if err != nil {
			return nil, err
		}
		parsed, err := url.Parse(normalized)
		if err != nil {
			return nil, err
		}
		allowedURLs = append(allowedURLs, parsed)
	}

	if pathPrefix != "" && !strings.HasPrefix(pathPrefix, "/") {
		pathPrefix = "/" + pathPrefix
	}

	return &Scope{
		Origin:        origin,
		AllowedOrigin: allowedURLs,
		PathPrefix:    pathPrefix,
	}, nil
}

func NormalizeLink(rawHref, sourceURL string) (NormalizedLink, error) {
	rawHref = strings.TrimSpace(rawHref)
	if rawHref == "" {
		return NormalizedLink{}, ErrEmptyHref
	}

	source, err := url.Parse(sourceURL)
	if err != nil {
		return NormalizedLink{}, err
	}
	href, err := url.Parse(rawHref)
	if err != nil {
		return NormalizedLink{}, err
	}

	if !href.IsAbs() {
		if href.Scheme == "" {
			// Relative URL or bare fragment; resolve using source page.
			href = source.ResolveReference(href)
		} else if href.Host == "" {
			// Handles odd cases like `mailto:` etc before we can filter cleanly.
			return NormalizedLink{}, ErrUnsupportedScheme
		}
	}

	if href.Scheme != "" && href.Scheme != "http" && href.Scheme != "https" {
		return NormalizedLink{}, ErrUnsupportedScheme
	}

	if href.Host == "" {
		return NormalizedLink{}, ErrUnsupportedScheme
	}

	linkURL := normalizeURL(href)
	fetchURL := normalizeURL(href)
	fetchURL.Fragment = ""

	normalized := NormalizedLink{
		LinkURL:  linkURL.String(),
		FetchURL: fetchURL.String(),
	}

	return normalized, nil
}

func (s *Scope) Check(normalizedFetchURL string) (ScopeDecision, error) {
	if s == nil {
		return ScopeDecision{Allowed: false, Reason: ScopeReasonExternalOrigin}, nil
	}

	parsed, err := url.Parse(normalizedFetchURL)
	if err != nil {
		return ScopeDecision{}, err
	}
	parsed = normalizeURL(parsed)

	if parsed.Scheme != "http" && parsed.Scheme != "https" || parsed.Host == "" {
		return ScopeDecision{}, fmt.Errorf("unexpected normalized URL: %q", normalizedFetchURL)
	}

	if !sameOrigin(parsed, s.Origin) && !sameOriginAgainstList(parsed, s.AllowedOrigin) {
		return ScopeDecision{Allowed: false, Reason: ScopeReasonExternalOrigin}, nil
	}

	if s.PathPrefix != "" && !strings.HasPrefix(parsed.Path, s.PathPrefix) {
		return ScopeDecision{Allowed: false, Reason: ScopeReasonOutsidePathPrefix}, nil
	}

	return ScopeDecision{Allowed: true}, nil
}

func sameOrigin(lhs, rhs *url.URL) bool {
	if lhs == nil || rhs == nil {
		return false
	}
	return lhs.Scheme == rhs.Scheme &&
		normalizeHostPort(lhs.Hostname(), lhs.Port(), lhs.Scheme) ==
			normalizeHostPort(rhs.Hostname(), rhs.Port(), rhs.Scheme)
}

func sameOriginAgainstList(target *url.URL, origins []*url.URL) bool {
	for _, origin := range origins {
		if sameOrigin(target, origin) {
			return true
		}
	}
	return false
}

func normalizeURL(parsed *url.URL) *url.URL {
	normalized := *parsed
	normalized.Scheme = strings.ToLower(normalized.Scheme)
	normalized.Host = normalizeHostPort(normalized.Hostname(), normalized.Port(), normalized.Scheme)

	if normalized.Path == "" {
		normalized.Path = "/"
	}

	return &normalized
}

func normalizeHostPort(hostname, port, scheme string) string {
	if hostname == "" {
		return ""
	}
	hostname = strings.ToLower(hostname)
	isIPv6 := strings.Count(hostname, ":") > 1
	if isIPv6 {
		if port == "" {
			return "[" + hostname + "]"
		}
		if isDefaultPort(scheme, port) {
			return "[" + hostname + "]"
		}
		return "[" + hostname + "]:" + port
	}
	if port == "" {
		return hostname
	}
	if isDefaultPort(scheme, port) {
		return hostname
	}
	return hostname + ":" + port
}

func isDefaultPort(scheme, port string) bool {
	switch scheme {
	case "http":
		return port == "80"
	case "https":
		return port == "443"
	default:
		return false
	}
}
