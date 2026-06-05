# Araneae

Araneae is a link checker for documentation websites. Point it at one entry URL, and it crawls links that are safe for that site, checks each discovered target once, counts every link occurrence, and writes a JSON report. It also includes a small local web UI for triaging the report.

The primary audience is technical writers and docs maintainers who need to validate a published docs site or preview environment before release.

## What It Checks

Araneae:

- Fetches the entry URL first.
- Parses HTML pages for `<a href="...">` links.
- Crawls only links in the entry URL origin by default.
- Accepts additional exact origins with `--allow-host`.
- Optionally restricts crawling to a path prefix with `--path-prefix`.
- Can seed a local docs build with `--local-root` so orphaned HTML pages are checked.
- Counts duplicate link occurrences.
- Fetches each normalized target URL once, even if multiple fragment links point to it.
- Reports dead links, missing fragments, and non-200 HTTP responses.
- Records skipped out-of-scope links separately.
- Writes a stable JSON report and serves it in a local UI.

Araneae does not execute JavaScript, authenticate to private sites, crawl external sites by default, or check image/script/style assets in the first version.

## Dependencies

For most technical writers, the recommended install path is a compiled release binary. Release binaries do not require Go or any other runtime dependency.

If you install from source or build Araneae yourself, you need Go. This repository currently targets Go 1.26.2.

- Official Go install instructions: [go.dev/doc/install](https://go.dev/doc/install)
- After installing Go, verify it is available:

```sh
go version
```

## Install

### Option 1: Download a release binary

For Araneae 1.0 and later, download the binary for your operating system from the [GitHub releases page](https://github.com/cpcf/araneae/releases).

1. Download the archive for your platform.
2. Unpack it.
3. Move the `araneae` executable somewhere on your `PATH`.
4. Verify the command is available:

```sh
araneae help
```

### Option 2: Install from source

Use this option if you are comfortable with Go tooling or need the latest code from the repository.

```sh
go install ./cmd/araneae
```

### Option 3: Build a local binary

```sh
go build -o araneae ./cmd/araneae
```

### Option 4: Run without installing

```sh
go run ./cmd/araneae scan https://docs.example.com/
```

## Usage

Run a scan:

```sh
araneae scan https://docs.example.com/ --out report.json
```

Open the report in the local UI:

```sh
araneae serve report.json
```

The server prints the local URL it is serving. Use `--addr` to choose an address:

```sh
araneae serve report.json --addr 127.0.0.1:8080
```

Flags may appear before or after the positional argument.

## Scan Options

```text
araneae scan <entry-url> [flags]
```

Important flags:

- `--out araneae-report.json`: output report path.
- `--max-pages 500`: maximum number of same-scope fetch URLs to check, including the entry URL.
- `--timeout 15s`: per-request timeout.
- `--concurrency 8`: number of concurrent fetch workers.
- `--max-requests-per-second 0`: maximum request starts per second across all workers. `0` means unlimited.
- `--max-response-bytes 5242880`: maximum HTML response body bytes to read. `0` means unlimited.
- `--retries 0`: retry count for transient fetch failures. `0` disables retries.
- `--retry-backoff 500ms`: delay between retry attempts.
- `--allow-host https://www.example.com`: additional exact origin that is safe to crawl. Can be repeated.
- `--path-prefix /docs/`: optional normalized path prefix that same-scope links must match.
- `--local-root public`: local static site root to seed the crawl with every `.html`/`.htm` page.
- `--user-agent "araneae/0.1"`: HTTP user agent.
- `--fail-on-dead`: exit non-zero after writing the report if dead links are found.
- `--fail-on-non-200`: exit non-zero after writing the report if any non-200 links are found.

Examples:

```sh
araneae scan https://docs.example.com/ \
  --out report.json \
  --max-pages 1000 \
  --concurrency 8 \
  --max-response-bytes 5242880 \
  --retries 2 \
  --retry-backoff 500ms \
  --max-requests-per-second 5
```

Allow a second exact origin:

```sh
araneae scan https://docs.example.com/ \
  --allow-host https://www.example.com
```

Limit crawling to a docs subtree:

```sh
araneae scan https://example.com/docs/ \
  --path-prefix /docs/
```

Check a local docs build for orphaned pages:

```sh
araneae scan http://localhost:8000/docs/ \
  --local-root public/docs \
  --path-prefix /docs/
```

`--local-root` treats the directory as being served at the entry URL path. It maps `index.html` to the directory URL, for example `guide/index.html` becomes `/docs/guide/`.

For large docs sites and private preview environments, keep `--max-response-bytes` at the default unless you know pages legitimately need more room. The limit applies to HTML bodies because those are parsed for links and fragments. Non-HTML responses such as PDFs and downloads are checked from status, redirects, final URL, and content type without reading the full body. Use `--max-response-bytes 0` only when you deliberately want unlimited HTML body reads.

Use retries only when the target environment has occasional transient failures, such as preview hosts behind cold caches or short deploy windows. Retries apply to network errors, timeouts, HTTP 429, and HTTP 5xx responses. They do not retry deterministic outcomes such as 404, 410, missing fragments, parsing errors, or HTML responses over `--max-response-bytes`.

Use in CI:

```sh
araneae scan https://preview.example.com/docs/ \
  --out report.json \
  --fail-on-dead \
  --fail-on-non-200
```

## Scope Rules

By default, Araneae crawls only the final entry URL origin after redirects. Origin means scheme, host, and port.

For example, this scan:

```sh
araneae scan https://docs.example.com/
```

will crawl `https://docs.example.com/...`, but it will not crawl:

- `https://www.example.com/...`
- `https://api.example.com/...`
- `http://docs.example.com/...`

Use `--allow-host` for additional safe origins. The match is exact by origin:

```sh
araneae scan https://docs.example.com/ \
  --allow-host https://www.example.com
```

Use `--path-prefix` to keep the crawl inside a subtree:

```sh
araneae scan https://example.com/docs/ --path-prefix /docs/
```

Same-origin links outside the prefix are recorded under `skipped_links` with reason `outside_path_prefix`.

## Expected Output

The scan writes a JSON report. The top-level shape is:

```json
{
  "schema_version": 1,
  "generated_at": "2026-05-28T15:04:46Z",
  "entry_url": "https://docs.example.com/",
  "scope": {
    "origin": "https://docs.example.com",
    "allowed_origins": [],
    "same_site_policy": "exact_origin_with_allowlist",
    "path_prefix": ""
  },
  "limits": {
    "max_pages": 500,
    "request_timeout_seconds": 15,
    "max_concurrency": 8,
    "max_requests_per_second": 0,
    "max_response_bytes": 5242880,
    "retries": 0,
    "retry_backoff_ms": 500
  },
  "summary": {
    "links_discovered": 5,
    "link_occurrences": 6,
    "fetches_attempted": 4,
    "ok_links": 3,
    "dead_links": 2,
    "non_200_links": 1,
    "skipped_links": 1,
    "skipped_external_links": 1,
    "truncated": false,
    "unvisited_urls": 0
  },
  "links": [],
  "fetches": [],
  "skipped_links": []
}
```

Each `links` entry represents one normalized navigable URL. Fragment variants are separate links but share a `fetch_url`:

```json
{
  "url": "https://docs.example.com/install#requirements",
  "fetch_url": "https://docs.example.com/install",
  "count": 4,
  "ok": false,
  "dead": true,
  "non_200": false,
  "problem": "missing_fragment",
  "status_code": 200,
  "final_url": "https://docs.example.com/install",
  "content_type": "text/html",
  "error": "",
  "sources": [
    {
      "page_url": "https://docs.example.com/",
      "count": 2,
      "texts": ["Requirements"]
    }
  ]
}
```

Each `fetches` entry records one fetched URL, including redirect metadata, when the check completed, and `duration_ms`, the elapsed fetch time in milliseconds:

```json
{
  "url": "https://docs.example.com/install",
  "status_code": 200,
  "final_url": "https://docs.example.com/install",
  "content_type": "text/html",
  "error": "",
  "redirect_chain": [],
  "checked_at": "2026-05-28T15:04:47Z",
  "duration_ms": 128
}
```

Problem values include:

- `http_status`: a received HTTP status other than 200.
- `network_error`: DNS, connection, or other network failure.
- `timeout`: request timeout.
- `tls_error`: TLS/certificate failure.
- `too_many_redirects`: redirect limit exceeded.
- `missing_fragment`: linked fragment was not found on a 200 HTML page.
- `parsing_error`: HTML parsing failed.
- `response_too_large`: an HTML response exceeded `--max-response-bytes`.

`dead` is true for network failures, timeouts, TLS errors, HTTP 404/410, missing fragments, and HTML responses that exceed `--max-response-bytes`. `non_200` is true for any received HTTP status other than 200.

`skipped_links` contains links Araneae saw but did not crawl, such as external origins or same-origin links outside `--path-prefix`.

## Web UI

The UI is served locally from the Go binary:

```sh
araneae serve report.json
```

It includes:

- Summary metrics.
- Problem links sorted by severity.
- All links table with status filters.
- Skipped links table.
- Search by link URL or source page.
- Sorting by count, status, URL, and source count.
- Link detail with sources, snippets, redirect chain, final URL, content type, and error details.
- Copy URL and copy source page actions when browser clipboard support is available.

## Local Test Site

The repository includes a small static site for manual checks:

```sh
cd examples/test-site
python3 -m http.server 8000
```

Then scan it:

```sh
araneae scan http://127.0.0.1:8000/index.html \
  --out report.json \
  --max-pages 20 \
  --concurrency 4
```

See [examples/test-site/README.md](examples/test-site/README.md) for details.

## Development

Run tests:

```sh
go test ./...
```

Run the crawler race test:

```sh
go test -race ./internal/crawl
```
