<!-- vale Google.Headings = NO -->
# GitHub Actions
<!-- vale Google.Headings = YES -->

You can run Araneae in GitHub Actions through the composite action in this
repository or through a raw `araneae check` command.

Use the composite action when you want GitHub Actions to install and run
Araneae for you. Tagged action refs try to use the matching release binary
first, then fall back to building from source. Local and branch refs use the
source checkout to build Araneae.

The action enables the GitHub step summary by default, writes the JSON report
to `out`, and exposes `report-path` and `comparison-path` outputs for artifact
upload.

## Composite action inputs

- `entry-url`: entry URL to crawl. Required.
- `out`: JSON report path. Defaults to `araneae-report.json`.
- `max-pages`: maximum number of in-scope crawl targets to check.
- `path-prefix`: optional path prefix restriction.
- `allow-host`: newline-separated additional exact origins allowed for crawl.
- `sitemap`: newline-separated XML sitemap URLs to seed the crawl.
- `baseline`: previous JSON report to compare against.
- `headers`: newline-separated HTTP request headers in `Name: value` form.
- `fail-on`: failure mode for link issues.
  Use `all` or `new`. Defaults to `all`.

The action runs `araneae check` with these flags:

- `--fail-on-dead`
- `--fail-on-non-200`
- `--fail-on-truncated`
- `--summary markdown`
- `--ci`

## Published docs

```yaml
name: docs-check

on:
  pull_request:

jobs:
  araneae:
    runs-on: ubuntu-latest
    steps:
      - id: araneae
        uses: cpcf/araneae@v1
        with:
          entry-url: https://docs.example.com/
          out: araneae-report.json
          max-pages: "1000"
          sitemap: https://docs.example.com/sitemap.xml
          fail-on: all
      - uses: actions/upload-artifact@v4
        if: always()
        with:
          name: araneae-report
          path: ${{ steps.araneae.outputs['report-path'] }}
```

## Preview with authentication

To check a preview URL that requires authentication, pass newline-separated
headers:

```yaml
      - id: araneae
        uses: cpcf/araneae@v1
        with:
          entry-url: https://preview.example.com/docs/
          out: araneae-report.json
          path-prefix: /docs/
          headers: |
            Authorization: Bearer ${{ secrets.DOCS_PREVIEW_TOKEN }}
          fail-on: all
```

## Baseline artifact flow

To compare against a baseline in CI, fetch the previous successful Araneae
report before you run the action. Then upload the current report and comparison
artifact. The fetch command depends on how your project stores artifacts.

A baseline is a previous Araneae JSON report that classifies issues as new,
existing, or resolved.

```yaml
      - name: Fetch previous report
        run: |
          mkdir -p baseline
          ./scripts/fetch-prev-report baseline/araneae-report.json || true
      - id: baseline
        run: |
          if [ -f baseline/araneae-report.json ]; then
            echo "path=baseline/araneae-report.json" >> "$GITHUB_OUTPUT"
            echo "fail_on=new" >> "$GITHUB_OUTPUT"
          else
            echo "path=" >> "$GITHUB_OUTPUT"
            echo "fail_on=all" >> "$GITHUB_OUTPUT"
          fi
      - id: araneae
        uses: cpcf/araneae@v1
        with:
          entry-url: https://preview.example.com/docs/
          out: artifacts/araneae-report.json
          baseline: ${{ steps.baseline.outputs.path }}
          fail-on: ${{ steps.baseline.outputs.fail_on }}
      - uses: actions/upload-artifact@v4
        if: always()
        with:
          name: araneae-report
          path: |
            ${{ steps.araneae.outputs['report-path'] }}
            ${{ steps.araneae.outputs['comparison-path'] }}
```

When you provide `baseline`, the action writes a comparison JSON next to the
report. For example, `artifacts/araneae-report.json` produces
`artifacts/araneae-report-comparison.json`.

## Raw command example

Use a raw command when your workflow already installs Araneae or when you need
flags that the composite action doesn't expose.

```yaml
name: docs-check

on:
  pull_request:

jobs:
  araneae:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Build docs
        run: make docs
      - name: Serve docs
        run: python3 -m http.server 8000 --directory public &
      - name: Check docs links
        run: |
          araneae check http://127.0.0.1:8000/ \
            --out araneae-report.json \
            --local-root public \
            --fail-on-dead \
            --fail-on-non-200 \
            --fail-on-truncated \
            --summary markdown \
            --ci
      - uses: actions/upload-artifact@v4
        if: always()
        with:
          name: araneae-report
          path: araneae-report.json
```

## Raw baseline command

```yaml
      - name: Fetch previous report
        run: |
          mkdir -p baseline
          # Replace this command with your artifact-store lookup.
          # Leave baseline/report.json absent on first run.
          ./scripts/fetch-prev-report baseline/report.json || true
      - name: Check docs links against baseline
        run: |
          BASELINE_FLAGS="--fail-on all"
          if [ -f baseline/report.json ]; then
            BASELINE_FLAGS="--baseline baseline/report.json --fail-on new"
          fi
          araneae check http://127.0.0.1:8000/ \
            --out araneae-report.json \
            --comparison-out araneae-comparison.json \
            --local-root public \
            --fail-on-dead \
            --fail-on-non-200 \
            --fail-on-truncated \
            $BASELINE_FLAGS \
            --summary markdown \
            --ci
      - name: Upload current report
        uses: actions/upload-artifact@v4
        with:
          name: araneae-report
          path: |
            araneae-report.json
            araneae-comparison.json
```
