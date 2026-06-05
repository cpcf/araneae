package crawl

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cpcf/araneae/internal/report"
)

const sameSitePolicy = "exact_origin_with_allowlist"

type ScanOptions struct {
	EntryURL             string
	MaxPages             int
	Timeout              time.Duration
	Concurrency          int
	MaxRequestsPerSecond float64
	MaxResponseBytes     int64
	Retries              int
	RetryBackoff         time.Duration
	AllowHosts           []string
	PathPrefix           string
	LocalRoot            string
	UserAgent            string
	Fetcher              Fetcher
	Parser               Parser
	ForceNow             func() time.Time
	requestGate          requestGate
	retrySleep           func(context.Context, time.Duration) error
}

type Crawler struct {
	opts    ScanOptions
	fetcher Fetcher
	parser  Parser
}

type sourceState struct {
	Count int
	Texts map[string]struct{}
}

type linkState struct {
	URL      string
	FetchURL string
	Fragment string
	Count    int
	Sources  map[string]*sourceState
}

type skippedState struct {
	URL     string
	Reason  string
	Count   int
	Sources map[string]*sourceState
}

func Run(ctx context.Context, opts ScanOptions) (report.Report, error) {
	c := NewCrawler(opts)
	return c.Run(ctx)
}

func NewCrawler(opts ScanOptions) *Crawler {
	fetcher := opts.Fetcher
	if fetcher == nil {
		fetcher = NewHTTPFetcher(opts.Timeout, opts.UserAgent, opts.MaxResponseBytes)
	}
	parser := opts.Parser
	if parser == nil {
		parser = HTMLParser{}
	}
	if opts.MaxPages <= 0 {
		opts.MaxPages = 1
	}
	if opts.Concurrency <= 0 {
		opts.Concurrency = 1
	}
	if opts.Retries < 0 {
		opts.Retries = 0
	}
	if opts.RetryBackoff < 0 {
		opts.RetryBackoff = 0
	}
	return &Crawler{
		opts:    opts,
		fetcher: fetcher,
		parser:  parser,
	}
}

func (c *Crawler) now() time.Time {
	if c.opts.ForceNow != nil {
		return c.opts.ForceNow()
	}
	return time.Now().UTC()
}

func (c *Crawler) Run(ctx context.Context) (report.Report, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	parsedEntry, err := url.Parse(c.opts.EntryURL)
	if err != nil {
		return report.Report{}, fmt.Errorf("invalid entry URL: %w", err)
	}
	normalizedEntry := normalizeURL(parsedEntry)
	normalizedEntry.Fragment = ""
	entryURL := normalizedEntry.String()

	requestGate := c.opts.requestGate
	if requestGate == nil {
		requestGate = newRequestGate(c.opts.MaxRequestsPerSecond)
	}
	entryFetch, err := c.fetchWithRetries(ctx, requestGate, entryURL)
	if err != nil {
		return report.Report{}, fmt.Errorf("entry fetch request failed: %w", err)
	}

	scopeOrigin := entryURL
	if entryFetch.FinalURL != "" {
		scopeOrigin = entryFetch.FinalURL
	}

	scope, err := NewScope(scopeOrigin, c.opts.AllowHosts, c.opts.PathPrefix)
	if err != nil {
		return report.Report{}, fmt.Errorf("scope init failed: %w", err)
	}

	fetches := map[string]report.FetchResult{}
	fragmentTargets := map[string]map[string]struct{}{}
	links := map[string]*linkState{}
	skipped := map[string]*skippedState{}
	visited := map[string]bool{}
	queued := map[string]bool{}
	queue := make([]string, 0)
	type fetchOutcome struct {
		fetch FetchResult
		err   error
	}

	taskCh := make(chan string, c.opts.Concurrency)
	resultCh := make(chan fetchOutcome, c.opts.Concurrency)
	var workerWG sync.WaitGroup

	startWorkers := func() {
		for i := 0; i < c.opts.Concurrency; i++ {
			workerWG.Go(func() {
				for fetchURL := range taskCh {
					fetch, err := c.fetchWithRetries(ctx, requestGate, fetchURL)
					resultCh <- fetchOutcome{fetch: fetch, err: err}
				}
			})
		}
	}

	popQueue := func() (string, bool) {
		for len(queue) > 0 {
			fetchURL := queue[0]
			queue = queue[1:]
			if !queued[fetchURL] {
				continue
			}
			delete(queued, fetchURL)
			return fetchURL, true
		}
		return "", false
	}

	addToQueue := func(fetchURL string) {
		if fetchURL == "" {
			return
		}
		if visited[fetchURL] || queued[fetchURL] {
			return
		}
		queue = append(queue, fetchURL)
		queued[fetchURL] = true
	}

	addLocalRootSeeds := func() error {
		if c.opts.LocalRoot == "" {
			return nil
		}
		seeds, err := localRootSeedURLs(scopeOrigin, c.opts.LocalRoot)
		if err != nil {
			return err
		}
		for _, seed := range seeds {
			decision, err := scope.Check(seed)
			if err != nil {
				return err
			}
			if decision.Allowed {
				addToQueue(seed)
			}
		}
		return nil
	}

	recordSource := func(dst map[string]*sourceState, pageURL, text string) {
		st, ok := dst[pageURL]
		if !ok {
			st = &sourceState{
				Texts: map[string]struct{}{},
			}
			dst[pageURL] = st
		}
		st.Count++
		if text != "" {
			st.Texts[text] = struct{}{}
		}
	}

	recordLink := func(source string, raw LinkOccurrence, decision ScopeDecision) {
		if !decision.Allowed {
			existing, ok := skipped[raw.LinkURL]
			if !ok {
				existing = &skippedState{
					URL:     raw.LinkURL,
					Reason:  decision.Reason,
					Sources: map[string]*sourceState{},
				}
				skipped[raw.LinkURL] = existing
			}
			existing.Count++
			recordSource(existing.Sources, source, shortenText(raw.Text))
			return
		}

		existing, ok := links[raw.LinkURL]
		if !ok {
			existing = &linkState{
				URL:      raw.LinkURL,
				FetchURL: raw.FetchURL,
				Fragment: raw.Fragment,
				Sources:  map[string]*sourceState{},
			}
			links[raw.LinkURL] = existing
		}
		existing.Count++
		recordSource(existing.Sources, source, shortenText(raw.Text))
		addToQueue(raw.FetchURL)
	}

	processParsed := func(source string, parsed ParseResult) {
		for _, link := range parsed.Links {
			decision, err := scope.Check(link.FetchURL)
			if err != nil {
				continue
			}
			recordLink(source, link, decision)
		}
	}

	recordFetch := func(fetch FetchResult) {
		fetches[fetch.URL] = report.FetchResult{
			URL:           fetch.URL,
			StatusCode:    fetch.StatusCode,
			FinalURL:      fetch.FinalURL,
			ContentType:   fetch.ContentType,
			Error:         fetch.Error,
			RedirectChain: append([]string{}, fetch.RedirectChain...),
			CheckedAt:     fetch.CheckedAt,
			DurationMS:    fetch.Duration.Milliseconds(),
		}
	}

	processFetch := func(fetch FetchResult) {
		recordFetch(fetch)
		sourceURL := firstNonEmpty(fetch.FinalURL, fetch.URL)
		if fetch.Error != "" || fetch.StatusCode != 200 || !isHTMLContentType(fetch.ContentType) {
			return
		}
		parsed, err := c.parser.Parse(sourceURL, bytes.NewReader(fetch.Body))
		if err != nil {
			current := fetches[fetch.URL]
			current.Error = "parsing_error: " + err.Error()
			fetches[fetch.URL] = current
			return
		}
		fragmentTargets[fetch.URL] = parsed.FragmentIDs
		processParsed(sourceURL, parsed)
	}

	visited[entryFetch.URL] = true
	processFetch(entryFetch)
	if err := addLocalRootSeeds(); err != nil {
		return report.Report{}, fmt.Errorf("local root discovery failed: %w", err)
	}

	startWorkers()

	attempted := 1
	inFlight := 0

	for {
		for c.opts.MaxPages > attempted && inFlight < c.opts.Concurrency && len(queue) > 0 {
			next, ok := popQueue()
			if !ok {
				continue
			}
			if visited[next] {
				continue
			}
			visited[next] = true
			inFlight++
			attempted++
			taskCh <- next
		}

		if inFlight == 0 {
			if len(queue) == 0 || c.opts.MaxPages <= attempted {
				break
			}
		}

		if inFlight == 0 {
			continue
		}

		outcome := <-resultCh
		inFlight--

		if outcome.err != nil {
			cancel()
			close(taskCh)
			workerWG.Wait()
			close(resultCh)
			return report.Report{}, fmt.Errorf("fetch request failed: %w", outcome.err)
		}
		processFetch(outcome.fetch)
	}

	close(taskCh)
	workerWG.Wait()
	close(resultCh)

	summary := report.ReportSummary{
		LinksDiscovered:  len(links),
		LinkOccurrences:  totalLinkOccurrences(links),
		FetchesAttempted: len(fetches),
		Truncated:        attempted >= c.opts.MaxPages && len(queue) > 0,
		UnvisitedURLs:    len(queue),
	}

	reportLinks := make([]report.LinkResult, 0, len(links))
	for _, link := range links {
		sources := make([]report.ReportSource, 0, len(link.Sources))
		for page, source := range link.Sources {
			texts := make([]string, 0, len(source.Texts))
			for text := range source.Texts {
				texts = append(texts, text)
			}
			sort.Strings(texts)
			sources = append(sources, report.ReportSource{
				PageURL: page,
				Count:   source.Count,
				Texts:   texts,
			})
		}
		sort.Slice(sources, func(i, j int) bool {
			return sources[i].PageURL < sources[j].PageURL
		})

		fetch, ok := fetches[link.FetchURL]
		result := report.LinkResult{
			URL:      link.URL,
			FetchURL: link.FetchURL,
			Count:    link.Count,
			Sources:  sources,
		}

		if ok {
			result.StatusCode = fetch.StatusCode
			result.FinalURL = fetch.FinalURL
			result.ContentType = fetch.ContentType
			result.Error = fetch.Error
			result.Problem, result.Dead, result.Non200, result.OK = linkProblemAndHealth(link, fetch, fragmentTargets)
			if result.Dead {
				summary.DeadLinks++
			}
			if result.Non200 {
				summary.Non200Links++
			}
			if result.OK {
				summary.OKLinks++
			}
		}
		reportLinks = append(reportLinks, result)
	}
	sort.Slice(reportLinks, func(i, j int) bool {
		return reportLinks[i].URL < reportLinks[j].URL
	})

	reportSkips := make([]report.SkippedLink, 0, len(skipped))
	for _, skip := range skipped {
		sources := make([]report.ReportSource, 0, len(skip.Sources))
		for page, source := range skip.Sources {
			texts := make([]string, 0, len(source.Texts))
			for text := range source.Texts {
				texts = append(texts, text)
			}
			sort.Strings(texts)
			sources = append(sources, report.ReportSource{
				PageURL: page,
				Count:   source.Count,
				Texts:   texts,
			})
		}
		sort.Slice(sources, func(i, j int) bool {
			return sources[i].PageURL < sources[j].PageURL
		})
		if skip.Reason == ScopeReasonExternalOrigin {
			summary.SkippedExternal++
		}
		summary.SkippedLinks++
		reportSkips = append(reportSkips, report.SkippedLink{
			URL:     skip.URL,
			Reason:  skip.Reason,
			Count:   skip.Count,
			Sources: sources,
		})
	}
	sort.Slice(reportSkips, func(i, j int) bool {
		return reportSkips[i].URL < reportSkips[j].URL
	})

	reportFetches := make([]report.FetchResult, 0, len(fetches))
	for _, fetch := range fetches {
		reportFetches = append(reportFetches, fetch)
	}
	sort.Slice(reportFetches, func(i, j int) bool {
		return reportFetches[i].URL < reportFetches[j].URL
	})

	allowed := make([]string, 0, len(scope.AllowedOrigin))
	for _, origin := range scope.AllowedOrigin {
		allowed = append(allowed, origin.String())
	}
	sort.Strings(allowed)

	entryURLForReport := firstNonEmpty(entryFetch.FinalURL, entryURL)

	return report.Report{
		SchemaVersion: report.SchemaVersion,
		GeneratedAt:   c.now(),
		EntryURL:      entryURLForReport,
		Scope: report.ReportScope{
			Origin:         scope.Origin.String(),
			AllowedOrigins: allowed,
			Policy:         sameSitePolicy,
			PathPrefix:     scope.PathPrefix,
		},
		Limits: report.ReportLimits{
			MaxPages:             c.opts.MaxPages,
			RequestTimeoutSec:    int(c.opts.Timeout.Seconds()),
			MaxConcurrency:       c.opts.Concurrency,
			MaxRequestsPerSecond: c.opts.MaxRequestsPerSecond,
			MaxResponseBytes:     c.opts.MaxResponseBytes,
			Retries:              c.opts.Retries,
			RetryBackoffMS:       c.opts.RetryBackoff.Milliseconds(),
		},
		Summary:      summary,
		Links:        reportLinks,
		Fetches:      reportFetches,
		SkippedLinks: reportSkips,
	}, nil
}

func (c *Crawler) fetchWithRetries(ctx context.Context, requestGate requestGate, fetchURL string) (FetchResult, error) {
	attempts := c.opts.Retries + 1
	for attempt := 0; attempt < attempts; attempt++ {
		if err := requestGate.Wait(ctx); err != nil {
			return FetchResult{}, err
		}
		fetch, err := c.fetcher.Fetch(ctx, fetchURL)
		if err != nil {
			return fetch, err
		}
		if attempt == attempts-1 || !isRetryableFetchResult(fetch) {
			return fetch, nil
		}
		if err := c.sleepBeforeRetry(ctx); err != nil {
			return FetchResult{}, err
		}
	}
	return FetchResult{}, nil
}

func (c *Crawler) sleepBeforeRetry(ctx context.Context) error {
	if c.opts.RetryBackoff <= 0 {
		return nil
	}
	if c.opts.retrySleep != nil {
		return c.opts.retrySleep(ctx, c.opts.RetryBackoff)
	}
	timer := time.NewTimer(c.opts.RetryBackoff)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func shortenText(text string) string {
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if text == "" {
		return ""
	}
	if len(text) > 120 {
		return text[:120]
	}
	return text
}

func totalLinkOccurrences(links map[string]*linkState) int {
	total := 0
	for _, link := range links {
		total += link.Count
	}
	return total
}

func linkProblemAndHealth(link *linkState, fetch report.FetchResult, fragmentsByFetch map[string]map[string]struct{}) (problem string, dead bool, non200 bool, ok bool) {
	if fetch.Error != "" {
		if fetch.StatusCode != 0 && fetch.StatusCode != 200 {
			non200 = true
		}
		return normalizeProblem(fetch.Error), true, non200, false
	}
	if fetch.StatusCode != 200 {
		non200 = true
		problem = "http_status"
		if fetch.StatusCode == 404 || fetch.StatusCode == 410 {
			dead = true
		}
		return problem, dead, non200, false
	}
	if link.Fragment != "" && isHTMLContentType(fetch.ContentType) {
		fragments := fragmentsByFetch[fetch.URL]
		if _, ok := fragments[link.Fragment]; !ok {
			return "missing_fragment", true, false, false
		}
	}
	return "", false, false, true
}

func normalizeProblem(err string) string {
	switch {
	case err == "":
		return ""
	case strings.Contains(err, "timeout"):
		return "timeout"
	case strings.Contains(err, "tls_error"):
		return "tls_error"
	case strings.Contains(err, "too_many_redirects"):
		return "too_many_redirects"
	case strings.Contains(err, "parsing_error"):
		return "parsing_error"
	case strings.Contains(err, problemResponseTooLarge):
		return problemResponseTooLarge
	case strings.Contains(err, "network_error"):
		return "network_error"
	case strings.Contains(err, "redirect"):
		return "http_status"
	}
	return "network_error"
}
