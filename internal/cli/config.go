package cli

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type envLookup func(string) (string, bool)

type configFile struct {
	SchemaVersion        *int           `yaml:"schema_version"`
	EntryURL             *string        `yaml:"entry_url"`
	Out                  *string        `yaml:"out"`
	MaxPages             *int           `yaml:"max_pages"`
	Timeout              configDuration `yaml:"timeout"`
	Concurrency          *int           `yaml:"concurrency"`
	MaxRequestsPerSecond *float64       `yaml:"max_requests_per_second"`
	MaxResponseBytes     *int64         `yaml:"max_response_bytes"`
	Retries              *int           `yaml:"retries"`
	RetryBackoff         configDuration `yaml:"retry_backoff"`
	Headers              []configHeader `yaml:"headers"`
	AllowHosts           []string       `yaml:"allow_hosts"`
	PathPrefix           *string        `yaml:"path_prefix"`
	LocalRoot            *string        `yaml:"local_root"`
	Sitemaps             []string       `yaml:"sitemaps"`
	MaxSitemapURLs       *int           `yaml:"max_sitemap_urls"`
	UserAgent            *string        `yaml:"user_agent"`
	FailOnDead           *bool          `yaml:"fail_on_dead"`
	FailOnNon200         *bool          `yaml:"fail_on_non_200"`
	FailOnTruncated      *bool          `yaml:"fail_on_truncated"`
	Summary              *string        `yaml:"summary"`
	CI                   *bool          `yaml:"ci"`
	GithubStepSummary    *string        `yaml:"github_step_summary"`
	Baseline             *string        `yaml:"baseline"`
	FailOn               *string        `yaml:"fail_on"`
	ComparisonOut        *string        `yaml:"comparison_out"`
}

type configDuration struct {
	value time.Duration
	set   bool
}

func (d *configDuration) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.ScalarNode || node.Tag != "!!str" {
		return fmt.Errorf("duration must be a string such as \"15s\"")
	}
	parsed, err := time.ParseDuration(node.Value)
	if err != nil {
		return fmt.Errorf("invalid duration")
	}
	d.value = parsed
	d.set = true
	return nil
}

type configHeader struct {
	Name     string  `yaml:"name"`
	Value    *string `yaml:"value"`
	ValueEnv *string `yaml:"value_env"`
}

func loadCLIConfig(explicitPath string) (*configFile, string, error) {
	path := explicitPath
	if path == "" {
		var err error
		path, err = defaultConfigPath()
		if err != nil {
			return nil, "", err
		}
	}
	if path == "" {
		return nil, "", nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if explicitPath != "" {
			return nil, path, fmt.Errorf("--config %s: %w", path, err)
		}
		return nil, path, fmt.Errorf("read config %s: %w", path, err)
	}

	var cfg configFile
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return nil, path, fmt.Errorf("read config %s: %w", path, err)
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		if err != nil {
			return nil, path, fmt.Errorf("read config %s: %w", path, err)
		}
		return nil, path, fmt.Errorf("read config %s: multiple YAML documents are not supported", path)
	}
	if cfg.SchemaVersion != nil && *cfg.SchemaVersion != 1 {
		return nil, path, fmt.Errorf("read config %s: unsupported schema_version %d", path, *cfg.SchemaVersion)
	}
	if err := validateConfigEnums(path, cfg); err != nil {
		return nil, path, err
	}
	return &cfg, path, nil
}

func validateConfigEnums(path string, cfg configFile) error {
	if cfg.FailOn != nil {
		switch *cfg.FailOn {
		case "all", "new":
		default:
			return fmt.Errorf("read config %s: fail_on must be one of: all, new", path)
		}
	}
	if cfg.Summary != nil {
		switch *cfg.Summary {
		case "text", "markdown":
		default:
			return fmt.Errorf("read config %s: summary must be one of: text, markdown", path)
		}
	}
	return nil
}

func defaultConfigPath() (string, error) {
	for _, path := range []string{"araneae.yaml", ".araneae.yaml"} {
		_, err := os.Stat(path)
		if err == nil {
			return path, nil
		}
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("stat config %s: %w", path, err)
		}
	}
	return "", nil
}

func applyScanConfig(opts *scanOptions, cfg configFile, setFlags map[string]bool, positionals []string, lookupEnv envLookup) ([]string, []string, []string, error) {
	if cfg.EntryURL != nil && len(positionals) == 0 {
		opts.entryURL = *cfg.EntryURL
	}
	if cfg.Out != nil && !setFlags["out"] {
		opts.out = *cfg.Out
	}
	if cfg.MaxPages != nil && !setFlags["max-pages"] {
		opts.maxPages = *cfg.MaxPages
	}
	if cfg.Timeout.set && !setFlags["timeout"] {
		opts.timeout = cfg.Timeout.value
	}
	if cfg.Concurrency != nil && !setFlags["concurrency"] {
		opts.concurrency = *cfg.Concurrency
	}
	if cfg.MaxRequestsPerSecond != nil && !setFlags["max-requests-per-second"] {
		opts.maxReqPerSec = *cfg.MaxRequestsPerSecond
	}
	if cfg.MaxResponseBytes != nil && !setFlags["max-response-bytes"] {
		opts.maxResponseBytes = *cfg.MaxResponseBytes
	}
	if cfg.Retries != nil && !setFlags["retries"] {
		opts.retries = *cfg.Retries
	}
	if cfg.RetryBackoff.set && !setFlags["retry-backoff"] {
		opts.retryBackoff = cfg.RetryBackoff.value
	}
	if cfg.PathPrefix != nil && !setFlags["path-prefix"] {
		opts.pathPrefix = *cfg.PathPrefix
	}
	if cfg.LocalRoot != nil && !setFlags["local-root"] {
		opts.localRoot = *cfg.LocalRoot
	}
	if cfg.MaxSitemapURLs != nil && !setFlags["max-sitemap-urls"] {
		opts.maxSitemapURLs = *cfg.MaxSitemapURLs
	}
	if cfg.UserAgent != nil && !setFlags["user-agent"] {
		opts.userAgent = *cfg.UserAgent
	}
	if cfg.FailOnDead != nil && !setFlags["fail-on-dead"] {
		opts.failOnDead = *cfg.FailOnDead
	}
	if cfg.FailOnNon200 != nil && !setFlags["fail-on-non-200"] {
		opts.failOnNon200 = *cfg.FailOnNon200
	}

	var rawHeaders []string
	if !setFlags["header"] && len(cfg.Headers) > 0 {
		resolved, err := configHeaders(cfg.Headers, lookupEnv)
		if err != nil {
			return nil, nil, nil, err
		}
		rawHeaders = resolved
	}

	var allowHosts []string
	if !setFlags["allow-host"] && len(cfg.AllowHosts) > 0 {
		allowHosts = append(allowHosts, cfg.AllowHosts...)
	}

	var sitemapURLs []string
	if !setFlags["sitemap"] && len(cfg.Sitemaps) > 0 {
		sitemapURLs = append(sitemapURLs, cfg.Sitemaps...)
	}

	return rawHeaders, allowHosts, sitemapURLs, nil
}

func applyCheckConfig(opts *checkOptions, cfg configFile, setFlags map[string]bool) {
	if cfg.FailOnDead != nil && !setFlags["fail-on-dead"] {
		opts.policy.FailOnDead = *cfg.FailOnDead
	}
	if cfg.FailOnNon200 != nil && !setFlags["fail-on-non-200"] {
		opts.policy.FailOnNon200 = *cfg.FailOnNon200
	}
	if cfg.FailOnTruncated != nil && !setFlags["fail-on-truncated"] {
		opts.policy.FailOnTruncated = *cfg.FailOnTruncated
	}
	if cfg.Summary != nil && !setFlags["summary"] {
		opts.summaryFormat = *cfg.Summary
	}
	if cfg.CI != nil && !setFlags["ci"] {
		opts.ci = *cfg.CI
	}
	if cfg.GithubStepSummary != nil && !setFlags["github-step-summary"] {
		opts.githubStepSummary = *cfg.GithubStepSummary
	}
	if cfg.Baseline != nil && !setFlags["baseline"] {
		opts.baselinePath = *cfg.Baseline
	}
	if cfg.FailOn != nil && !setFlags["fail-on"] {
		opts.failOn = *cfg.FailOn
	}
	if cfg.ComparisonOut != nil && !setFlags["comparison-out"] {
		opts.comparisonOut = *cfg.ComparisonOut
	}
}

func configHeaders(headers []configHeader, lookupEnv envLookup) ([]string, error) {
	raw := make([]string, 0, len(headers))
	for i, header := range headers {
		value, err := headerValue(header, lookupEnv)
		if err != nil {
			return nil, fmt.Errorf("config header %d: %w", i+1, err)
		}
		parsed, err := parseConfigHeader(header.Name, value)
		if err != nil {
			return nil, fmt.Errorf("config header %d: %w", i+1, err)
		}
		raw = append(raw, parsed.Name+": "+parsed.Value)
	}
	return raw, nil
}

func parseConfigHeader(name, value string) (requestHeader, error) {
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

func headerValue(header configHeader, lookupEnv envLookup) (string, error) {
	if header.Value != nil && header.ValueEnv != nil {
		return "", fmt.Errorf("set either value or value_env, not both")
	}
	if header.Value == nil && header.ValueEnv == nil {
		return "", fmt.Errorf("set value or value_env")
	}
	if header.Value != nil {
		return *header.Value, nil
	}

	envName := strings.TrimSpace(*header.ValueEnv)
	if envName == "" {
		return "", fmt.Errorf("value_env must not be empty")
	}
	if lookupEnv == nil {
		lookupEnv = os.LookupEnv
	}
	value, ok := lookupEnv(envName)
	if !ok {
		return "", fmt.Errorf("environment variable %s is not set", envName)
	}
	return value, nil
}

func flagSet(fs *flag.FlagSet) map[string]bool {
	set := make(map[string]bool)
	fs.Visit(func(flag *flag.Flag) {
		set[flag.Name] = true
	})
	return set
}
