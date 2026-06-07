# Araneae

Araneae checks links in documentation websites. Give it one entry URL, and
Araneae crawls links that are safe for that site, checks each discovered target
once, counts every link occurrence, and writes a JSON report. You can also open
the report in a local web UI to decide what to fix first.

Technical writers and docs maintainers can use Araneae to validate a published
docs site or preview environment before release.

## Quick start

Run a scan:

```sh
araneae scan https://docs.example.com/ --out report.json
```

Run a CI-style check:

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

To choose a local address, pass `--addr`:

```sh
araneae serve report.json --addr 127.0.0.1:8080
```

## What it checks

Araneae:

- Fetches the entry URL first.
- Parses HTML pages for `<a href="...">` links.
- Crawls only links in the entry URL origin by default.
- Accepts additional exact origins with `--allow-host`.
- Restricts crawling to a path prefix when you pass `--path-prefix`.
- Seeds local docs builds with `--local-root`.
- Seeds scans from explicit XML sitemaps with `--sitemap`.
- Counts duplicate link occurrences.
- Fetches each normalized target URL once.
- Reports dead links, missing fragments, non-200 responses, and skipped links.
- Writes a stable JSON report and serves it in a local UI.

Araneae doesn't execute JavaScript, crawl external sites by default, or check
image, script, or style assets.

## Install

Download a release binary for your platform from the [GitHub releases].
Release binaries don't require Go or another runtime dependency.

If you work from a source checkout, use Go to build or run Araneae. The source
tree targets Go 1.26.2.

```sh
go build -o araneae ./cmd/araneae
./araneae help
```

For local development, run Araneae directly:

```sh
go run ./cmd/araneae scan https://docs.example.com/
```

## Common workflows

To scan an authenticated preview, set `DOCS_PREVIEW_TOKEN` to your preview
token and run:

```sh
araneae check https://preview.example.com/docs/ \
  --header "Authorization: Bearer $DOCS_PREVIEW_TOKEN" \
  --path-prefix /docs/ \
  --out report.json \
  --fail-on-dead \
  --fail-on-non-200 \
  --fail-on-truncated
```

To keep CI commands short, create `araneae.yaml`:

```yaml
entry_url: https://docs.example.com/
out: araneae-report.json
max_pages: 1000
path_prefix: /docs/
sitemaps:
  - https://docs.example.com/sitemap.xml
fail_on_dead: true
fail_on_non_200: true
fail_on_truncated: true
summary: markdown
ci: true
```

Then run:

```sh
araneae check --config araneae.yaml
```

To run Araneae from GitHub Actions, use the composite action:

```yaml
- id: araneae
  uses: cpcf/araneae@v1
  with:
    entry-url: https://docs.example.com/
    out: araneae-report.json
    sitemap: https://docs.example.com/sitemap.xml
    fail-on: all
```

## Documentation

- [Use Araneae](docs/usage.md): command-line options, scan limits, request
  headers, sitemaps, scope rules, and baseline checks.
- [Configuration](docs/configuration.md): YAML config files, command-line
  override behavior, and environment-backed header values.
- [GitHub Actions](docs/github-actions.md): composite action inputs, artifact
  handling, preview authentication, baseline flows, and raw-command examples.
- [Reports and UI](docs/reports-and-ui.md): JSON report shape, comparison
  artifacts, problem values, and local triage UI behavior.
- [Local test site](examples/test-site/README.md): sample static site for
  manual checks.

## Development

Run tests:

```sh
go test ./...
```

Run the crawler race test:

```sh
go test -race ./internal/crawl
```

Sync packaged Vale styles:

```sh
vale sync
```

Run documentation style checks:

```sh
vale .
```

[GitHub releases]: https://github.com/cpcf/araneae/releases
