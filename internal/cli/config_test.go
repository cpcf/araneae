package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	checkeval "github.com/cpcf/araneae/internal/check"
)

func TestParseCheckArgsLoadsConfigFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "araneae.yaml")
	writeConfigFixture(t, configPath, `
schema_version: 1
entry_url: https://docs.example.com/
out: config-report.json
max_pages: 1000
timeout: 3s
concurrency: 2
max_requests_per_second: 4.5
max_response_bytes: 123456
retries: 2
retry_backoff: 750ms
headers:
  - name: Authorization
    value_env: DOCS_AUTH_HEADER
  - name: X-Preview
    value: enabled
allow_hosts:
  - https://www.example.com
path_prefix: /docs/
local_root: public/docs
sitemaps:
  - https://docs.example.com/sitemap.xml
  - https://docs.example.com/sitemap-api.xml
max_sitemap_urls: 250
user_agent: config-agent
fail_on_dead: true
fail_on_non_200: true
fail_on_truncated: true
summary: markdown
ci: true
github_step_summary: summary.md
baseline: baseline.json
fail_on: new
comparison_out: comparison.json
`)

	opts, err := parseCheckArgs([]string{"--config", configPath}, mapEnvLookup(map[string]string{
		"DOCS_AUTH_HEADER": "Bearer config-token",
	}))
	if err != nil {
		t.Fatalf("parseCheckArgs() error = %v", err)
	}

	if opts.scan.entryURL != "https://docs.example.com/" {
		t.Fatalf("entryURL = %q", opts.scan.entryURL)
	}
	if opts.scan.out != "config-report.json" {
		t.Fatalf("out = %q", opts.scan.out)
	}
	if opts.scan.maxPages != 1000 {
		t.Fatalf("maxPages = %d", opts.scan.maxPages)
	}
	if opts.scan.timeout != 3*time.Second {
		t.Fatalf("timeout = %s", opts.scan.timeout)
	}
	if opts.scan.concurrency != 2 {
		t.Fatalf("concurrency = %d", opts.scan.concurrency)
	}
	if opts.scan.maxReqPerSec != 4.5 {
		t.Fatalf("maxReqPerSec = %f", opts.scan.maxReqPerSec)
	}
	if opts.scan.maxResponseBytes != 123456 {
		t.Fatalf("maxResponseBytes = %d", opts.scan.maxResponseBytes)
	}
	if opts.scan.retries != 2 {
		t.Fatalf("retries = %d", opts.scan.retries)
	}
	if opts.scan.retryBackoff != 750*time.Millisecond {
		t.Fatalf("retryBackoff = %s", opts.scan.retryBackoff)
	}
	if len(opts.scan.headers) != 2 {
		t.Fatalf("headers = %#v; want 2", opts.scan.headers)
	}
	if opts.scan.headers[0] != (requestHeader{Name: "Authorization", Value: "Bearer config-token"}) {
		t.Fatalf("first header = %#v", opts.scan.headers[0])
	}
	if opts.scan.headers[1] != (requestHeader{Name: "X-Preview", Value: "enabled"}) {
		t.Fatalf("second header = %#v", opts.scan.headers[1])
	}
	if len(opts.scan.allowHosts) != 1 || opts.scan.allowHosts[0] != "https://www.example.com" {
		t.Fatalf("allowHosts = %#v", opts.scan.allowHosts)
	}
	if opts.scan.pathPrefix != "/docs/" {
		t.Fatalf("pathPrefix = %q", opts.scan.pathPrefix)
	}
	if opts.scan.localRoot != "public/docs" {
		t.Fatalf("localRoot = %q", opts.scan.localRoot)
	}
	if len(opts.scan.sitemapURLs) != 2 || opts.scan.sitemapURLs[0] != "https://docs.example.com/sitemap.xml" || opts.scan.sitemapURLs[1] != "https://docs.example.com/sitemap-api.xml" {
		t.Fatalf("sitemapURLs = %#v", opts.scan.sitemapURLs)
	}
	if opts.scan.maxSitemapURLs != 250 {
		t.Fatalf("maxSitemapURLs = %d", opts.scan.maxSitemapURLs)
	}
	if opts.scan.userAgent != "config-agent" {
		t.Fatalf("userAgent = %q", opts.scan.userAgent)
	}
	if !opts.policy.FailOnDead || !opts.policy.FailOnNon200 || !opts.policy.FailOnTruncated {
		t.Fatalf("policy = %#v; want all fail flags", opts.policy)
	}
	if opts.policy.FailMode != checkeval.FailModeNew {
		t.Fatalf("fail mode = %q", opts.policy.FailMode)
	}
	if opts.summaryFormat != "markdown" || !opts.ci || opts.githubStepSummary != "summary.md" {
		t.Fatalf("summary/ci settings = %q %t %q", opts.summaryFormat, opts.ci, opts.githubStepSummary)
	}
	if opts.baselinePath != "baseline.json" || opts.comparisonOut != "comparison.json" {
		t.Fatalf("baseline/comparison = %q %q", opts.baselinePath, opts.comparisonOut)
	}
}

func TestParseCheckArgsFlagsOverrideConfigFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "araneae.yaml")
	writeConfigFixture(t, configPath, `
entry_url: https://config.example.com/
out: config-report.json
max_pages: 1000
headers:
  - name: X-Config
    value: config
allow_hosts:
  - https://config-host.example.com
sitemaps:
  - https://config.example.com/sitemap.xml
fail_on_dead: true
fail_on: new
baseline: config-baseline.json
`)

	opts, err := parseCheckArgs([]string{
		"--config", configPath,
		"https://cli.example.com/",
		"--out", "cli-report.json",
		"--max-pages", "25",
		"--header", "X-CLI: cli",
		"--allow-host", "https://cli-host.example.com",
		"--sitemap", "https://cli.example.com/sitemap.xml",
		"--fail-on-dead=false",
		"--fail-on", "all",
		"--baseline", "",
	}, nil)
	if err != nil {
		t.Fatalf("parseCheckArgs() error = %v", err)
	}

	if opts.scan.entryURL != "https://cli.example.com/" {
		t.Fatalf("entryURL = %q", opts.scan.entryURL)
	}
	if opts.scan.out != "cli-report.json" {
		t.Fatalf("out = %q", opts.scan.out)
	}
	if opts.scan.maxPages != 25 {
		t.Fatalf("maxPages = %d", opts.scan.maxPages)
	}
	if len(opts.scan.headers) != 1 || opts.scan.headers[0] != (requestHeader{Name: "X-CLI", Value: "cli"}) {
		t.Fatalf("headers = %#v; want CLI header only", opts.scan.headers)
	}
	if len(opts.scan.allowHosts) != 1 || opts.scan.allowHosts[0] != "https://cli-host.example.com" {
		t.Fatalf("allowHosts = %#v; want CLI allow host only", opts.scan.allowHosts)
	}
	if len(opts.scan.sitemapURLs) != 1 || opts.scan.sitemapURLs[0] != "https://cli.example.com/sitemap.xml" {
		t.Fatalf("sitemapURLs = %#v; want CLI sitemap only", opts.scan.sitemapURLs)
	}
	if opts.policy.FailOnDead {
		t.Fatal("FailOnDead = true; want CLI false override")
	}
	if opts.policy.FailMode != checkeval.FailModeAll {
		t.Fatalf("fail mode = %q; want all", opts.policy.FailMode)
	}
	if opts.baselinePath != "" {
		t.Fatalf("baseline = %q; want CLI empty override", opts.baselinePath)
	}
}

func TestParseScanArgsLoadsDefaultConfigPath(t *testing.T) {
	dir := t.TempDir()
	writeConfigFixture(t, filepath.Join(dir, ".araneae.yaml"), `
entry_url: https://docs.example.com/
max_pages: 42
`)
	t.Chdir(dir)

	opts, err := parseScanArgs(nil, nil)
	if err != nil {
		t.Fatalf("parseScanArgs() error = %v", err)
	}
	if opts.entryURL != "https://docs.example.com/" || opts.maxPages != 42 {
		t.Fatalf("opts = %#v; want default config entry URL and max pages", opts)
	}
}

func TestParseArgsRejectsMissingExplicitConfigFile(t *testing.T) {
	_, err := ParseScanArgs([]string{"--config", filepath.Join(t.TempDir(), "missing.yaml")})
	if err == nil {
		t.Fatal("ParseScanArgs() error = nil; want missing config error")
	}
	if !strings.Contains(err.Error(), "--config") {
		t.Fatalf("error = %q; want --config", err)
	}
}

func TestParseArgsRejectsInvalidConfigValues(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "unknown field",
			body: "entry_url: https://docs.example.com/\nunknown: true\n",
			want: "unknown",
		},
		{
			name: "unsupported schema version",
			body: "schema_version: 2\nentry_url: https://docs.example.com/\n",
			want: "schema_version",
		},
		{
			name: "invalid url",
			body: "entry_url: not-a-url\n",
			want: "invalid entry URL",
		},
		{
			name: "invalid duration",
			body: "entry_url: https://docs.example.com/\ntimeout: slow\n",
			want: "invalid duration",
		},
		{
			name: "numeric duration",
			body: "entry_url: https://docs.example.com/\ntimeout: 15\n",
			want: "duration must be a string",
		},
		{
			name: "invalid header",
			body: "entry_url: https://docs.example.com/\nheaders:\n  - name: Host\n    value: preview.example.com\n",
			want: "Host header is not supported",
		},
		{
			name: "colon in header name",
			body: "entry_url: https://docs.example.com/\nheaders:\n  - name: 'X-Bad: injected'\n    value: value\n",
			want: "header name contains invalid characters",
		},
		{
			name: "invalid sitemap",
			body: "entry_url: https://docs.example.com/\nsitemaps:\n  - /sitemap.xml\n",
			want: "--sitemap",
		},
		{
			name: "invalid fail mode",
			body: "entry_url: https://docs.example.com/\nfail_on: sometimes\n",
			want: "fail_on",
		},
		{
			name: "invalid summary",
			body: "entry_url: https://docs.example.com/\nsummary: html\n",
			want: "summary",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "araneae.yaml")
			writeConfigFixture(t, configPath, tt.body)
			_, err := parseCheckArgs([]string{"--config", configPath}, nil)
			if err == nil {
				t.Fatal("parseCheckArgs() error = nil; want config error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q; want %q", err, tt.want)
			}
			if strings.Contains(err.Error(), "preview.example.com") {
				t.Fatalf("error %q leaks header value", err)
			}
		})
	}
}

func TestRunCheckAllowsEmptyHeaderValueEnv(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "araneae.yaml")
	writeConfigFixture(t, configPath, `
entry_url: https://docs.example.com/
headers:
  - name: X-Empty
    value_env: EMPTY_HEADER
`)

	opts, err := parseCheckArgs([]string{"--config", configPath}, func(name string) (string, bool) {
		if name == "EMPTY_HEADER" {
			return "", true
		}
		t.Fatalf("unexpected env lookup %q", name)
		return "", false
	})
	if err != nil {
		t.Fatalf("parseCheckArgs() error = %v", err)
	}
	if len(opts.scan.headers) != 1 || opts.scan.headers[0] != (requestHeader{Name: "X-Empty", Value: ""}) {
		t.Fatalf("headers = %#v; want empty env-backed header", opts.scan.headers)
	}
}

func TestParseScanArgsRejectsInvalidCheckConfigEnums(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "araneae.yaml")
	writeConfigFixture(t, configPath, `
entry_url: https://docs.example.com/
fail_on: sometimes
`)

	_, err := parseScanArgs([]string{"--config", configPath}, nil)
	if err == nil {
		t.Fatal("parseScanArgs() error = nil; want invalid config enum error")
	}
	if !strings.Contains(err.Error(), "fail_on") {
		t.Fatalf("error = %q; want fail_on", err)
	}
}

func TestParseArgsResolvesHeaderValueEnv(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "araneae.yaml")
	writeConfigFixture(t, configPath, `
entry_url: https://docs.example.com/
headers:
  - name: Authorization
    value_env: DOCS_AUTH_HEADER
`)

	opts, err := parseScanArgs([]string{"--config", configPath}, mapEnvLookup(map[string]string{
		"DOCS_AUTH_HEADER": "Bearer env-token",
	}))
	if err != nil {
		t.Fatalf("parseScanArgs() error = %v", err)
	}
	if len(opts.headers) != 1 || opts.headers[0] != (requestHeader{Name: "Authorization", Value: "Bearer env-token"}) {
		t.Fatalf("headers = %#v; want env-backed Authorization header", opts.headers)
	}
}

func TestParseArgsRejectsMissingHeaderValueEnv(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "araneae.yaml")
	writeConfigFixture(t, configPath, `
entry_url: https://docs.example.com/
headers:
  - name: Authorization
    value_env: DOCS_AUTH_HEADER
`)

	_, err := parseScanArgs([]string{"--config", configPath}, mapEnvLookup(nil))
	if err == nil {
		t.Fatal("parseScanArgs() error = nil; want missing env error")
	}
	if !strings.Contains(err.Error(), "DOCS_AUTH_HEADER") {
		t.Fatalf("error = %q; want env var name", err)
	}
}

func TestRunCheckCommandRejectsMissingHeaderValueEnv(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "araneae.yaml")
	writeConfigFixture(t, configPath, `
entry_url: https://docs.example.com/
headers:
  - name: Authorization
    value_env: DOCS_AUTH_HEADER
`)

	t.Setenv("DOCS_AUTH_HEADER", "")
	if err := os.Unsetenv("DOCS_AUTH_HEADER"); err != nil {
		t.Fatalf("unset env: %v", err)
	}

	err := runCheckCommand([]string{"--config", configPath}, nil, func(string) string {
		return ""
	})
	if err == nil {
		t.Fatal("runCheckCommand() error = nil; want missing env error")
	}
	if !strings.Contains(err.Error(), "DOCS_AUTH_HEADER") {
		t.Fatalf("error = %q; want env var name", err)
	}
}

func TestRunCheckCommandAllowsEmptyHeaderValueEnv(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "araneae.yaml")
	writeConfigFixture(t, configPath, `
entry_url: https://docs.example.com/
headers:
  - name: X-Empty
    value_env: EMPTY_HEADER
`)
	t.Setenv("EMPTY_HEADER", "")

	opts, err := parseCheckArgs([]string{"--config", configPath}, os.LookupEnv)
	if err != nil {
		t.Fatalf("parseCheckArgs() error = %v", err)
	}
	if len(opts.scan.headers) != 1 || opts.scan.headers[0] != (requestHeader{Name: "X-Empty", Value: ""}) {
		t.Fatalf("headers = %#v; want empty env-backed header", opts.scan.headers)
	}
}

func TestParseArgsRejectsMultipleYAMLDocuments(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "araneae.yaml")
	writeConfigFixture(t, configPath, `
entry_url: https://docs.example.com/
---
fail_on_dead: true
`)

	_, err := parseCheckArgs([]string{"--config", configPath}, nil)
	if err == nil {
		t.Fatal("parseCheckArgs() error = nil; want multi-document config error")
	}
	if !strings.Contains(err.Error(), "multiple YAML documents") {
		t.Fatalf("error = %q; want multi-document error", err)
	}
}

func TestParseArgsRejectsBadHeaderValueEnvWithoutLeakingSecret(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "araneae.yaml")
	writeConfigFixture(t, configPath, `
entry_url: https://docs.example.com/
headers:
  - name: Authorization
    value_env: DOCS_AUTH_HEADER
`)

	secret := "Bearer bad\nsecret-token"
	_, err := parseScanArgs([]string{"--config", configPath}, mapEnvLookup(map[string]string{
		"DOCS_AUTH_HEADER": secret,
	}))
	if err == nil {
		t.Fatal("parseScanArgs() error = nil; want invalid header error")
	}
	if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("error %q leaks secret", err)
	}
}

func writeConfigFixture(t *testing.T, path, body string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(strings.TrimSpace(body)+"\n"), 0o644); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}
}

func mapEnvLookup(values map[string]string) envLookup {
	return func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	}
}
