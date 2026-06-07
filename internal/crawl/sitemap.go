package crawl

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	DefaultMaxSitemapURLs = 5000
	maxChildSitemaps      = 100
)

type sitemapDocument struct {
	pageURLs    []string
	sitemapURLs []string
}

type sitemapLoc struct {
	Loc string `xml:"loc"`
}

type sitemapURLSet struct {
	URLs []sitemapLoc `xml:"url"`
}

type sitemapIndex struct {
	Sitemaps []sitemapLoc `xml:"sitemap"`
}

type sitemapSeed struct {
	linkURL  string
	fetchURL string
	source   string
}

type sitemapSkip struct {
	url    string
	reason string
	source string
}

type bodyFetcher interface {
	FetchBody(context.Context, string) (FetchResult, error)
}

func parseSitemap(r io.Reader) (sitemapDocument, error) {
	decoder := xml.NewDecoder(r)
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return sitemapDocument{}, fmt.Errorf("missing sitemap root element")
		}
		if err != nil {
			return sitemapDocument{}, err
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}

		switch start.Name.Local {
		case "urlset":
			var parsed sitemapURLSet
			if err := decoder.DecodeElement(&parsed, &start); err != nil {
				return sitemapDocument{}, err
			}
			return sitemapDocument{pageURLs: locValues(parsed.URLs)}, nil
		case "sitemapindex":
			var parsed sitemapIndex
			if err := decoder.DecodeElement(&parsed, &start); err != nil {
				return sitemapDocument{}, err
			}
			return sitemapDocument{sitemapURLs: locValues(parsed.Sitemaps)}, nil
		default:
			return sitemapDocument{}, fmt.Errorf("unsupported sitemap root element <%s>", start.Name.Local)
		}
	}
}

func locValues(entries []sitemapLoc) []string {
	values := make([]string, 0, len(entries))
	for _, entry := range entries {
		loc := strings.TrimSpace(entry.Loc)
		if loc != "" {
			values = append(values, loc)
		}
	}
	return values
}

func (c *Crawler) sitemapSeeds(ctx context.Context, requestGate requestGate, roots []string, scope *Scope) ([]sitemapSeed, []sitemapSkip, error) {
	if len(roots) == 0 {
		return nil, nil, nil
	}

	maxURLs := c.opts.MaxSitemapURLs
	if maxURLs <= 0 {
		maxURLs = DefaultMaxSitemapURLs
	}

	queue := append([]string{}, roots...)
	seenSitemaps := map[string]struct{}{}
	seenPages := map[string]struct{}{}
	childSitemaps := 0
	seeds := make([]sitemapSeed, 0)
	skips := make([]sitemapSkip, 0)

	for len(queue) > 0 {
		sitemapURL := queue[0]
		queue = queue[1:]

		normalizedSitemap, err := NormalizeLink(sitemapURL, sitemapURL)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid sitemap URL %q: %w", sitemapURL, err)
		}
		sitemapURL = normalizedSitemap.LinkURL
		if _, ok := seenSitemaps[sitemapURL]; ok {
			continue
		}
		seenSitemaps[sitemapURL] = struct{}{}

		document, err := c.fetchSitemap(ctx, requestGate, sitemapURL)
		if err != nil {
			return nil, nil, err
		}

		for _, child := range document.sitemapURLs {
			normalizedChild, err := NormalizeLink(child, sitemapURL)
			if err != nil {
				return nil, nil, fmt.Errorf("invalid child sitemap URL %q from %s: %w", child, sitemapURL, err)
			}
			childSitemaps++
			if childSitemaps > maxChildSitemaps {
				return nil, nil, fmt.Errorf("sitemap index exceeds child sitemap limit of %d", maxChildSitemaps)
			}
			queue = append(queue, normalizedChild.LinkURL)
		}

		for _, rawPageURL := range document.pageURLs {
			normalizedPage, err := NormalizeLink(rawPageURL, sitemapURL)
			if err != nil {
				return nil, nil, fmt.Errorf("invalid sitemap page URL %q from %s: %w", rawPageURL, sitemapURL, err)
			}
			if _, ok := seenPages[normalizedPage.LinkURL]; ok {
				continue
			}
			seenPages[normalizedPage.LinkURL] = struct{}{}

			decision, err := scope.Check(normalizedPage.FetchURL)
			if err != nil {
				return nil, nil, fmt.Errorf("check sitemap page URL %q: %w", normalizedPage.LinkURL, err)
			}
			if !decision.Allowed {
				skips = append(skips, sitemapSkip{
					url:    normalizedPage.LinkURL,
					reason: decision.Reason,
					source: sitemapURL,
				})
				continue
			}
			if len(seeds) >= maxURLs {
				return nil, nil, fmt.Errorf("sitemap page URL count exceeds --max-sitemap-urls limit of %d", maxURLs)
			}
			seeds = append(seeds, sitemapSeed{
				linkURL:  normalizedPage.LinkURL,
				fetchURL: normalizedPage.FetchURL,
				source:   sitemapURL,
			})
		}
	}

	return seeds, skips, nil
}

func (c *Crawler) fetchSitemap(ctx context.Context, requestGate requestGate, sitemapURL string) (sitemapDocument, error) {
	fetch, err := c.fetchBodyWithRetries(ctx, requestGate, sitemapURL)
	if err != nil {
		return sitemapDocument{}, fmt.Errorf("fetch sitemap %s: %w", sitemapURL, err)
	}
	if fetch.Error != "" {
		return sitemapDocument{}, fmt.Errorf("fetch sitemap %s: %s", sitemapURL, normalizeProblem(fetch.Error))
	}
	if fetch.StatusCode != http.StatusOK {
		return sitemapDocument{}, fmt.Errorf("fetch sitemap %s: status %d", sitemapURL, fetch.StatusCode)
	}
	if len(fetch.Body) == 0 {
		return sitemapDocument{}, fmt.Errorf("fetch sitemap %s: empty response body", sitemapURL)
	}

	document, err := parseSitemap(bytes.NewReader(fetch.Body))
	if err != nil {
		return sitemapDocument{}, fmt.Errorf("parse sitemap %s: %w", sitemapURL, err)
	}
	return document, nil
}

func (c *Crawler) fetchBodyWithRetries(ctx context.Context, requestGate requestGate, fetchURL string) (FetchResult, error) {
	if fetcher, ok := c.fetcher.(bodyFetcher); ok {
		return c.fetchWithRetriesFunc(ctx, requestGate, fetchURL, fetcher.FetchBody)
	}
	return c.fetchWithRetries(ctx, requestGate, fetchURL)
}
