# Configuration

Use a YAML config file to keep docs-check policy reviewable in the repository
and to keep CI commands short.

Araneae looks for `araneae.yaml` first, then `.araneae.yaml`. To use an
explicit path, pass `--config`:

```sh
araneae check --config path/to/file.yaml
```

Command-line flags override config values. If you pass repeatable flags, they
replace the config list for that field. Repeatable fields include `headers`,
`allow_hosts`, and `sitemaps`.

## Example

Create `araneae.yaml`:

```yaml
schema_version: 1
entry_url: https://docs.example.com/
out: araneae-report.json
max_pages: 1000
timeout: 15s
concurrency: 8
max_requests_per_second: 5
path_prefix: /docs/
allow_hosts:
  - https://www.example.com
sitemaps:
  - https://docs.example.com/sitemap.xml
headers:
  - name: Authorization
    value_env: DOCS_AUTH_HEADER
fail_on_dead: true
fail_on_non_200: true
fail_on_truncated: true
fail_on: new
baseline: araneae-baseline.json
comparison_out: araneae-comparison.json
summary: markdown
ci: true
```

Then run:

```sh
araneae check --config araneae.yaml
```

The config file can provide `entry_url`, so the positional entry URL is
optional when config supplies it. Durations use Go-style strings, such as
`15s`, `500ms`, and `1m`.

Araneae rejects unknown config fields so misspelled policy doesn't silently
pass.

## Secret headers

Headers can use literal values or environment-backed values:

```yaml
headers:
  - name: Authorization
    value_env: DOCS_AUTH_HEADER
  - name: X-Preview
    value: enabled
```

`value_env` reads the named environment variable at runtime and errors if it's
not set. Araneae validates resolved header values the same way it validates
`--header` values. Araneae doesn't print resolved values in parser errors or
write them to the JSON report.

To use an environment-backed header in GitHub Actions, set the environment
variable in the step:

```yaml
      - name: Check docs links
        run: araneae check --config araneae.yaml
        env:
          DOCS_AUTH_HEADER: Bearer ${{ secrets.DOCS_PREVIEW_TOKEN }}
```

## Fields

The config file supports the same options as the `scan` and `check` commands:

- `schema_version`
- `entry_url`
- `out`
- `max_pages`
- `timeout`
- `concurrency`
- `max_requests_per_second`
- `max_response_bytes`
- `retries`
- `retry_backoff`
- `headers`
- `allow_hosts`
- `path_prefix`
- `local_root`
- `sitemaps`
- `max_sitemap_urls`
- `user_agent`
- `fail_on_dead`
- `fail_on_non_200`
- `fail_on_truncated`
- `baseline`
- `fail_on`
- `comparison_out`
- `summary`
- `ci`
- `github_step_summary`

For details about each option, see [Use Araneae](usage.md).
