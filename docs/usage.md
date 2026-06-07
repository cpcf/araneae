<!-- vale Google.Headings = NO -->
# Use Araneae
<!-- vale Google.Headings = YES -->

This page describes the command-line behavior that most users need after the
README quick start: command options, crawl scope, authenticated previews,
sitemaps, safety limits, and CI check policy.

A baseline is a previous Araneae JSON report. Araneae uses a baseline to
classify issues as new, existing, or resolved.

## Commands

Run a scan:

```sh
araneae scan https://docs.example.com/ --out report.json
```

Run a CI check:

```sh
araneae check https://docs.example.com/ \
  --out report.json \
  --fail-on-dead \
  --fail-on-non-200 \
  --fail-on-truncated
```

Open the report in the local UI:

```sh
araneae serve report.json
```

Flags can appear before or after the positional argument.

## Scan options

```text
araneae scan [ENTRY_URL] [FLAGS]
```

Replace `ENTRY_URL` with the first page that Araneae crawls.

General flags:

- `--config araneae.yaml`: load options from a YAML config file.
  If you omit this flag, Araneae reads `araneae.yaml` or `.araneae.yaml`
  when either file exists.
- `--out araneae-report.json`: write the JSON report to this path.
- `--user-agent "araneae/0.1"`: send this HTTP user agent.

Crawl limits:

- `--max-pages 500`: check up to this many in-scope crawl targets.
- `--timeout 15s`: set the per-request timeout.
- `--concurrency 8`: set the number of fetch workers.
- `--max-requests-per-second 0`: limit request starts per second.
  A value of `0` means unlimited.
- `--max-response-bytes 5242880`: limit HTML response body bytes.
  A value of `0` means unlimited.
- `--retries 0`: retry transient fetch failures.
  A value of `0` disables retries.
- `--retry-backoff 500ms`: wait this long between retry attempts.

Scope and discovery:

- `--header "Name: value"`: send an HTTP request header.
  You can repeat this flag.
- `--allow-host https://www.example.com`: allow another exact origin.
  You can repeat this flag.
- `--path-prefix /docs/`: restrict same-scope links to this path prefix.
- `--local-root public`: seed the crawl with local `.html` and `.htm` files.
- `--sitemap https://docs.example.com/sitemap.xml`: seed from a sitemap.
  You can repeat this flag.
- `--max-sitemap-urls 5000`: seed up to this many in-scope sitemap URLs.

Scan policy:

- `--fail-on-dead`: exit non-zero after writing the report if Araneae finds
  dead links.
- `--fail-on-non-200`: exit non-zero after writing the report if Araneae finds
  non-200 links.

Example:

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

## Authenticated previews

To scan a preview with a bearer token, set `DOCS_PREVIEW_TOKEN` to the token
for your preview site and run:

```sh
araneae scan https://preview.example.com/docs/ \
  --header "Authorization: Bearer $DOCS_PREVIEW_TOKEN"
```

To scan a preview that uses cookie-based access, set `DOCS_PREVIEW_COOKIE` to
the preview cookie and run:

```sh
araneae scan https://preview.example.com/docs/ \
  --header "Cookie: preview_session=$DOCS_PREVIEW_COOKIE"
```

Quote the whole header argument so the shell passes the colon and spaces as one
value. In CI, store token and cookie values in the platform's secret store and
expand them through environment variables.

Araneae doesn't write configured request header names or values to the JSON
report. Araneae sends headers only to the entry URL origin and same-origin
redirects. It doesn't send them to `--allow-host` origins or cross-origin
redirect targets.

Araneae doesn't support `Host` through `--header`. If you provide
`User-Agent` with `--header`, the `--user-agent` flag value wins.

## Scope rules

By default, Araneae crawls only the final entry URL origin after redirects.
Origin means scheme, host, and port.

For example, this scan:

```sh
araneae scan https://docs.example.com/
```

crawls `https://docs.example.com/...`, but it doesn't crawl:

- `https://www.example.com/...`
- `https://api.example.com/...`
- `http://docs.example.com/...`

To allow another safe origin, pass `--allow-host`. The match is exact by
origin:

```sh
araneae scan https://docs.example.com/ \
  --allow-host https://www.example.com
```

To keep the crawl inside a subtree, pass `--path-prefix`:

```sh
araneae scan https://example.com/docs/ --path-prefix /docs/
```

Araneae records same-origin links outside the prefix as `skipped_links` with
the reason `outside_path_prefix`.

## Local roots

To check orphaned HTML pages in a local docs build, pass `--local-root`:

```sh
araneae scan http://localhost:8000/docs/ \
  --local-root public/docs \
  --path-prefix /docs/
```

Araneae maps files in the directory to URLs under the entry URL path. It maps
`index.html` to the directory URL. For example, `guide/index.html` maps to
`/docs/guide/`.

## Sitemaps

To seed a published docs scan from a sitemap, run:

```sh
araneae scan https://docs.example.com/ \
  --sitemap https://docs.example.com/sitemap.xml
```

To seed a local preview from its generated sitemap, run:

```sh
araneae scan http://localhost:8000/docs/ \
  --sitemap http://localhost:8000/docs/sitemap.xml \
  --path-prefix /docs/
```

Explicit sitemaps can be `urlset` files or `sitemapindex` files. Araneae still
checks sitemap-listed URLs against the normal origin, `--allow-host`, and
`--path-prefix` rules. A sitemap broadens discovery, not crawl permission.

## Safety limits

For large docs sites and private preview environments, keep
`--max-response-bytes` at the default unless pages need more room. The limit
applies to HTML bodies because Araneae parses them for links and fragments.

Araneae checks non-HTML responses, such as PDFs and downloads, from status,
redirects, final URL, and media type without reading the full body. Use
`--max-response-bytes 0` only when you want unlimited HTML body reads.

Use retries only when the target environment has occasional transient failures,
such as preview hosts behind cold caches or short deploy windows. Araneae
retries network errors, timeouts, HTTP 429, and HTTP 5xx responses.

Araneae doesn't retry deterministic outcomes, such as 404, 410, missing
fragments, parsing errors, or HTML responses over `--max-response-bytes`.

## Check options

```text
araneae check [ENTRY_URL] [FLAGS]
```

`check` runs the same crawl as `scan`, writes the same JSON report, prints a
concise status summary, and exits non-zero when enabled policy flags fail.

It reuses the scan flags and adds these flags:

- `--fail-on-dead`: exit non-zero when dead links or missing fragments exist.
- `--fail-on-non-200`: exit non-zero when any checked link returns a non-200
  HTTP status.
- `--fail-on-truncated`: exit non-zero when `--max-pages` leaves queued URLs
  unvisited.
- `--baseline previous-report.json`: compare the current report with a
  baseline report.
- `--fail-on all`: fail on every enabled current issue.
  Use `new` to fail only on enabled issues absent from the baseline.
- `--comparison-out comparison.json`: write a baseline comparison artifact.
- `--summary text`: print a text summary to stdout.
  Use `markdown` to print a Markdown table with top problems.
- `--ci`: enable CI conveniences, including default GitHub step summary output
  when GitHub Actions sets `$GITHUB_STEP_SUMMARY`.
- `--github-step-summary path`: append a Markdown summary to the given file.

To use `check` in CI, run:

```sh
araneae check https://preview.example.com/docs/ \
  --out report.json \
  --fail-on-dead \
  --fail-on-non-200 \
  --fail-on-truncated \
  --summary markdown \
  --ci
```

When you pass `--ci` and GitHub Actions provides `$GITHUB_STEP_SUMMARY`,
Araneae appends a Markdown summary automatically. Outside GitHub Actions, pass
`--github-step-summary summary.md` to write the same Markdown file explicitly.

To fail only on introduced link issues, pass a baseline report from an earlier
run:

```sh
araneae check https://preview.example.com/docs/ \
  --out report.json \
  --baseline previous-report.json \
  --comparison-out comparison.json \
  --fail-on-dead \
  --fail-on-non-200 \
  --fail-on new \
  --summary markdown \
  --ci
```

Araneae identifies a baseline issue by the normalized report link URL and the
problem value. For `http_status` issues, Araneae reports the HTTP status as
detail, but it doesn't use the status as part of the issue identity.
