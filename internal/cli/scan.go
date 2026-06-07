package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/cpcf/araneae/internal/crawl"
	"github.com/cpcf/araneae/internal/report"
)

type scanOptions struct {
	entryURL         string
	out              string
	maxPages         int
	timeout          time.Duration
	concurrency      int
	maxReqPerSec     float64
	maxResponseBytes int64
	retries          int
	retryBackoff     time.Duration
	headers          []requestHeader
	allowHosts       []string
	pathPrefix       string
	localRoot        string
	sitemapURLs      []string
	maxSitemapURLs   int
	userAgent        string
	failOnDead       bool
	failOnNon200     bool
}

type stringSliceValue []string

func (s *stringSliceValue) String() string {
	return strings.Join(*s, ",")
}

func (s *stringSliceValue) Set(v string) error {
	*s = append(*s, v)
	return nil
}

type requestHeader = crawl.RequestHeader

const (
	defaultMaxResponseBytes int64 = 5 * 1024 * 1024
	maxRetries                    = 100
)

var crawlRun = crawl.Run

type scanFlagState struct {
	configPath     string
	setFlags       map[string]bool
	positionals    []string
	rawHeaders     []string
	allowHosts     []string
	rawSitemapURLs []string
}

func newScanOptions() scanOptions {
	return scanOptions{maxSitemapURLs: crawl.DefaultMaxSitemapURLs}
}

func parseRequestHeader(raw string) (requestHeader, error) {
	name, value, ok := strings.Cut(raw, ":")
	if !ok {
		return requestHeader{}, fmt.Errorf("header must be in Name: value form")
	}
	if strings.ContainsAny(name, "\r\n") || strings.ContainsAny(value, "\r\n") {
		return requestHeader{}, fmt.Errorf("header name and value must not contain newlines")
	}
	if !validRawHeaderName(name) {
		return requestHeader{}, fmt.Errorf("header name contains invalid characters")
	}
	if !validHeaderFieldValue(value) {
		return requestHeader{}, fmt.Errorf("header value contains invalid characters")
	}
	name = strings.TrimSpace(name)
	value = strings.TrimSpace(value)
	if name == "" {
		return requestHeader{}, fmt.Errorf("header name must not be empty")
	}
	if !validHeaderFieldName(name) {
		return requestHeader{}, fmt.Errorf("header name contains invalid characters")
	}
	if strings.EqualFold(name, "Host") {
		return requestHeader{}, fmt.Errorf("Host header is not supported")
	}
	return requestHeader{Name: name, Value: value}, nil
}

func validRawHeaderName(name string) bool {
	for _, r := range name {
		if r == ' ' {
			continue
		}
		if r < 0x21 || r > 0x7e {
			return false
		}
	}
	return true
}

func validHeaderFieldName(name string) bool {
	for _, r := range name {
		if r > 127 || !strings.ContainsRune("!#$%&'*+-.^_`|~0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz", r) {
			return false
		}
	}
	return true
}

func validHeaderFieldValue(value string) bool {
	for _, r := range value {
		if r == '\t' {
			continue
		}
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func parseRequestHeaders(raw []string) ([]requestHeader, error) {
	headers := make([]requestHeader, 0, len(raw))
	for i, value := range raw {
		header, err := parseRequestHeader(value)
		if err != nil {
			return nil, fmt.Errorf("--header %d: %w", i+1, err)
		}
		headers = append(headers, header)
	}
	return headers, nil
}

func parseSitemapURLs(raw []string) ([]string, error) {
	sitemaps := make([]string, 0, len(raw))
	for i, value := range raw {
		parsed, err := url.Parse(value)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return nil, fmt.Errorf("--sitemap %d: invalid sitemap URL", i+1)
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return nil, fmt.Errorf("--sitemap %d: unsupported sitemap scheme %q", i+1, parsed.Scheme)
		}
		sitemaps = append(sitemaps, value)
	}
	return sitemaps, nil
}

func registerScanFlags(fs *flag.FlagSet, opts *scanOptions, rawHeaders, allowHosts, sitemapURLs *stringSliceValue) {
	fs.StringVar(&opts.out, "out", "araneae-report.json", "output report path")
	fs.IntVar(&opts.maxPages, "max-pages", 500, "maximum checked same-site fetch URLs")
	fs.DurationVar(&opts.timeout, "timeout", 15*time.Second, "per-request timeout")
	fs.IntVar(&opts.concurrency, "concurrency", 8, "fetch concurrency")
	fs.Float64Var(&opts.maxReqPerSec, "max-requests-per-second", 0, "maximum request starts per second; 0 means unlimited")
	fs.Int64Var(&opts.maxResponseBytes, "max-response-bytes", defaultMaxResponseBytes, "maximum HTML response body bytes to read; 0 means unlimited")
	fs.IntVar(&opts.retries, "retries", 0, "retry count for transient fetch failures; 0 disables retries")
	fs.DurationVar(&opts.retryBackoff, "retry-backoff", 500*time.Millisecond, "delay between retry attempts")
	fs.Var(rawHeaders, "header", "HTTP request header in 'Name: value' form; can be repeated")
	fs.Var(allowHosts, "allow-host", "additional exact origins allowed for crawl")
	fs.StringVar(&opts.pathPrefix, "path-prefix", "", "optional path prefix restriction")
	fs.StringVar(&opts.localRoot, "local-root", "", "local static site root to seed crawl with every HTML page")
	fs.Var(sitemapURLs, "sitemap", "XML sitemap URL to seed crawl; can be repeated")
	fs.IntVar(&opts.maxSitemapURLs, "max-sitemap-urls", opts.maxSitemapURLs, "maximum in-scope page URLs to seed from sitemaps")
	fs.StringVar(&opts.userAgent, "user-agent", "araneae/0.1", "user-agent string")
}

func parseScanCoreFlags(cmd string, args []string, opts *scanOptions, registerExtra func(*flag.FlagSet, *scanOptions)) (scanFlagState, error) {
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var state scanFlagState
	var allowHosts stringSliceValue
	var rawHeaders stringSliceValue
	var rawSitemapURLs stringSliceValue
	fs.StringVar(&state.configPath, "config", "", "YAML config path")
	registerScanFlags(fs, opts, &rawHeaders, &allowHosts, &rawSitemapURLs)
	if registerExtra != nil {
		registerExtra(fs, opts)
	}

	orderedArgs, err := interspersePositionals(fs, args)
	if err != nil {
		return state, fmt.Errorf("%s: %w", cmd, err)
	}
	if err := fs.Parse(orderedArgs); err != nil {
		return state, fmt.Errorf("%s: %w", cmd, sanitizeFlagParseError(err))
	}

	state.setFlags = flagSet(fs)
	state.positionals = append(state.positionals, fs.Args()...)
	state.rawHeaders = append(state.rawHeaders, rawHeaders...)
	state.allowHosts = append(state.allowHosts, allowHosts...)
	state.rawSitemapURLs = append(state.rawSitemapURLs, rawSitemapURLs...)
	return state, nil
}

func parseScanCoreArgs(cmd string, args []string, registerExtra func(*flag.FlagSet, *scanOptions), applyExtraConfig func(configFile, map[string]bool), lookupEnv envLookup) (scanOptions, error) {
	opts := newScanOptions()
	state, err := parseScanCoreFlags(cmd, args, &opts, registerExtra)
	if err != nil {
		return opts, err
	}

	cfg, _, err := loadCLIConfig(state.configPath)
	if err != nil {
		return opts, fmt.Errorf("%s: %w", cmd, err)
	}
	if cfg != nil {
		configHeaders, configAllowHosts, configSitemaps, err := applyScanConfig(&opts, *cfg, state.setFlags, state.positionals, lookupEnv)
		if err != nil {
			return opts, fmt.Errorf("%s: %w", cmd, err)
		}
		if len(configHeaders) > 0 {
			state.rawHeaders = configHeaders
		}
		if len(configAllowHosts) > 0 {
			state.allowHosts = configAllowHosts
		}
		if len(configSitemaps) > 0 {
			state.rawSitemapURLs = configSitemaps
		}
		if applyExtraConfig != nil {
			applyExtraConfig(*cfg, state.setFlags)
		}
	}

	opts.allowHosts = append(opts.allowHosts, state.allowHosts...)
	opts.sitemapURLs, err = parseSitemapURLs(state.rawSitemapURLs)
	if err != nil {
		return opts, fmt.Errorf("%s: %w", cmd, err)
	}
	opts.headers, err = parseRequestHeaders(state.rawHeaders)
	if err != nil {
		return opts, fmt.Errorf("%s: %w", cmd, err)
	}
	if len(state.positionals) > 1 {
		return opts, fmt.Errorf("%s: expected <entry-url>", cmd)
	}
	if len(state.positionals) == 1 {
		opts.entryURL = state.positionals[0]
	}
	if opts.entryURL == "" {
		return opts, fmt.Errorf("%s: expected <entry-url>", cmd)
	}

	parsed, err := url.Parse(opts.entryURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return opts, fmt.Errorf("%s: invalid entry URL", cmd)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return opts, fmt.Errorf("%s: unsupported entry scheme %q", cmd, parsed.Scheme)
	}
	if opts.maxReqPerSec < 0 || math.IsNaN(opts.maxReqPerSec) || math.IsInf(opts.maxReqPerSec, 0) {
		return opts, fmt.Errorf("%s: --max-requests-per-second must be a finite value >= 0", cmd)
	}
	if opts.maxResponseBytes < 0 {
		return opts, fmt.Errorf("%s: --max-response-bytes must be >= 0", cmd)
	}
	if opts.retries < 0 {
		return opts, fmt.Errorf("%s: --retries must be >= 0", cmd)
	}
	if opts.retries > maxRetries {
		return opts, fmt.Errorf("%s: --retries must be <= %d", cmd, maxRetries)
	}
	if opts.retryBackoff < 0 {
		return opts, fmt.Errorf("%s: --retry-backoff must be >= 0", cmd)
	}
	if opts.maxSitemapURLs <= 0 {
		return opts, fmt.Errorf("%s: --max-sitemap-urls must be > 0", cmd)
	}

	return opts, nil
}

func ParseScanArgs(args []string) (scanOptions, error) {
	return parseScanArgs(args, os.LookupEnv)
}

func parseScanArgs(args []string, lookupEnv envLookup) (scanOptions, error) {
	return parseScanCoreArgs("scan", args, func(fs *flag.FlagSet, opts *scanOptions) {
		fs.BoolVar(&opts.failOnDead, "fail-on-dead", false, "exit non-zero when dead links exist")
		fs.BoolVar(&opts.failOnNon200, "fail-on-non-200", false, "exit non-zero when non-200 links exist")
	}, nil, lookupEnv)
}

func RunScan(args []string) error {
	return runScanCommand(args, os.Stdout)
}

func runScanCommand(args []string, stdout io.Writer) error {
	opts, err := ParseScanArgs(args)
	if err != nil {
		if helpRequested(err) {
			return writeHelp(stdout, scanUsage())
		}
		return err
	}

	reportData, err := runScan(opts)
	if err != nil {
		return err
	}
	if err := writeReportFile(opts.out, reportData); err != nil {
		return err
	}

	if opts.failOnDead && reportData.Summary.DeadLinks > 0 {
		return fmt.Errorf("scan found dead links: %d", reportData.Summary.DeadLinks)
	}
	if opts.failOnNon200 && reportData.Summary.Non200Links > 0 {
		return fmt.Errorf("scan found non-200 links: %d", reportData.Summary.Non200Links)
	}

	return nil
}

func scanUsage() string {
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	opts := newScanOptions()
	var allowHosts stringSliceValue
	var rawHeaders stringSliceValue
	var rawSitemapURLs stringSliceValue
	var configPath string
	fs.StringVar(&configPath, "config", "", "YAML config path")
	registerScanFlags(fs, &opts, &rawHeaders, &allowHosts, &rawSitemapURLs)
	fs.BoolVar(&opts.failOnDead, "fail-on-dead", false, "exit non-zero when dead links exist")
	fs.BoolVar(&opts.failOnNon200, "fail-on-non-200", false, "exit non-zero when non-200 links exist")
	return flagUsage("scan", "[entry-url]", fs)
}

func runScan(opts scanOptions) (report.Report, error) {
	return crawlRun(context.Background(), scanCrawlerOptions(opts))
}

func scanCrawlerOptions(opts scanOptions) crawl.ScanOptions {
	return crawl.ScanOptions{
		EntryURL:             opts.entryURL,
		MaxPages:             opts.maxPages,
		Timeout:              opts.timeout,
		Concurrency:          opts.concurrency,
		MaxRequestsPerSecond: opts.maxReqPerSec,
		MaxResponseBytes:     opts.maxResponseBytes,
		Retries:              opts.retries,
		RetryBackoff:         opts.retryBackoff,
		Headers:              opts.headers,
		AllowHosts:           opts.allowHosts,
		PathPrefix:           opts.pathPrefix,
		LocalRoot:            opts.localRoot,
		SitemapURLs:          opts.sitemapURLs,
		MaxSitemapURLs:       opts.maxSitemapURLs,
		UserAgent:            opts.userAgent,
	}
}

func writeReportFile(path string, reportData report.Report) error {
	outputFile, err := os.Create(path)
	if err != nil {
		return err
	}
	defer outputFile.Close()

	return report.Write(outputFile, reportData)
}
