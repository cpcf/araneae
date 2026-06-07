# Reports and UI

Araneae writes a stable JSON report from `scan` and `check`. You can serve the
same report in the local UI to decide what to fix first.

## Report shape

The top-level report shape is:

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

Each `links` entry represents one normalized navigable URL. Fragment variants
are separate links, but they share a `fetch_url`:

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

Each `fetches` entry records one fetched URL. It includes redirect metadata,
the completion time, and `duration_ms`, the elapsed fetch time in milliseconds.

When you enable retries, `duration_ms` covers the final reported retry cycle,
including retry attempts and configured retry backoff delays.

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

`skipped_links` contains links that Araneae saw but didn't crawl, such as
external origins or same-origin links outside `--path-prefix`.

## Problem values

Problem values include:

- `http_status`: Araneae received an HTTP status other than 200.
- `network_error`: Araneae hit a Domain Name System failure, connection
  failure, or network failure.
- `timeout`: the request timed out.
- `tls_error`: the request hit a Transport Layer Security failure or
  certificate failure.
- `too_many_redirects`: the request exceeded the redirect limit.
- `missing_fragment`: Araneae didn't find the linked fragment on a 200 HTML
  page.
- `parsing_error`: Araneae couldn't parse the HTML page.
- `response_too_large`: an HTML response exceeded `--max-response-bytes`.

`dead` is true for network failures, timeouts, certificate failures, HTTP 404
and 410, missing fragments, and HTML responses that exceed
`--max-response-bytes`.

`non_200` is true for any received HTTP status other than 200.

## Comparison artifact

When you run `check` with `--comparison-out`, Araneae writes a separate JSON
comparison artifact:

```json
{
  "schema_version": 1,
  "baseline_entry_url": "https://docs.example.com/",
  "current_entry_url": "https://docs.example.com/",
  "summary": {
    "new": 1,
    "existing": 2,
    "resolved": 1,
    "unchanged_ok": 42
  },
  "new": [],
  "existing": [],
  "resolved": [],
  "unchanged_ok": []
}
```

The comparison groups include only issue types enabled by the check policy
flags, such as `--fail-on-dead` and `--fail-on-non-200`.

## Web UI

Serve the UI locally from the Go binary:

```sh
araneae serve report.json
```

To mark issues as new or existing, serve the report with the baseline report:

```sh
araneae serve report.json --baseline previous-report.json
```

The UI includes:

- Summary metrics.
- Problem links with severity, problem type, target host, source page, and
  baseline state filters.
- Skipped links table.
- Search by link URL or source page.
- Sorting by severity, count, status, URL, and source count.
- Link detail with sources, snippets, redirect chain, final URL, media type,
  and error details.
- Markdown and CSV exports for the filtered triage issue list.
- Browser-local acknowledged state keyed by stable issue fingerprints.
- Reset action for acknowledged state.
- Copy URL and copy source page actions when browser clipboard support exists.

Araneae stores acknowledged state only in the current browser through
`localStorage`. It doesn't write acknowledged state back to the report or
share it with teammates.

A typical docs PR triage flow is to serve the report with `--baseline`, filter
to critical introduced issues first, export the filtered Markdown list into the
PR discussion, and then acknowledge items locally as you assign or defer them.
