## Araneae test site

This is a small static docs-style site for exercising Araneae locally.

### Start a local server

```sh
cd examples/test-site
python3 -m http.server 8000
```

### Run a scan

```sh
cd /path/to/repo
go run ./cmd/araneae scan http://127.0.0.1:8000/index.html --max-pages 20 --concurrency 4
```

Add `--max-requests-per-second 2` if you want to verify rate limiting while scanning.

If you prefer a local binary, build and run it first:

```sh
go build -o araneae ./cmd/araneae
./araneae scan http://127.0.0.1:8000/index.html --max-pages 20 --concurrency 4
```

The site includes:

- duplicate links to `/guide.html`
- secondary-page discovery from `/guide.html` to `/reference.html`
- a link loop between `/loop-a.html` and `/loop-b.html`
- a valid fragment link to `#quick-start`
- a missing fragment link to `#missing-fragment`
- an external link to `https://example.com/`
- a missing page link (`/missing.html`) that returns 404
- a non-HTML linked asset (`/assets/notes.txt`)
