package crawl

import (
	"strings"
	"testing"
)

func TestParseSitemapURLSet(t *testing.T) {
	document, err := parseSitemap(strings.NewReader(`<?xml version="1.0" encoding="UTF-8"?>
<urlset>
  <url><loc>https://docs.example.test/</loc><lastmod>2026-06-07</lastmod></url>
  <url><loc> https://docs.example.test/guide </loc></url>
</urlset>`))
	if err != nil {
		t.Fatalf("parseSitemap() error = %v", err)
	}

	want := []string{"https://docs.example.test/", "https://docs.example.test/guide"}
	if !equalStrings(document.pageURLs, want) {
		t.Fatalf("page URLs = %#v; want %#v", document.pageURLs, want)
	}
	if len(document.sitemapURLs) != 0 {
		t.Fatalf("child sitemap URLs = %#v; want none", document.sitemapURLs)
	}
}

func TestParseSitemapIndex(t *testing.T) {
	document, err := parseSitemap(strings.NewReader(`<?xml version="1.0" encoding="UTF-8"?>
<sitemapindex>
  <sitemap><loc>https://docs.example.test/sitemap-pages.xml</loc></sitemap>
  <sitemap><loc>https://docs.example.test/sitemap-api.xml</loc></sitemap>
</sitemapindex>`))
	if err != nil {
		t.Fatalf("parseSitemap() error = %v", err)
	}

	want := []string{"https://docs.example.test/sitemap-pages.xml", "https://docs.example.test/sitemap-api.xml"}
	if !equalStrings(document.sitemapURLs, want) {
		t.Fatalf("child sitemap URLs = %#v; want %#v", document.sitemapURLs, want)
	}
	if len(document.pageURLs) != 0 {
		t.Fatalf("page URLs = %#v; want none", document.pageURLs)
	}
}

func TestParseSitemapWithNamespaceAndWhitespace(t *testing.T) {
	document, err := parseSitemap(strings.NewReader(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url>
    <loc>
      https://docs.example.test/namespaced
    </loc>
    <image:image xmlns:image="http://www.google.com/schemas/sitemap-image/1.1">
      <image:loc>https://cdn.example.test/image.png</image:loc>
    </image:image>
  </url>
</urlset>`))
	if err != nil {
		t.Fatalf("parseSitemap() error = %v", err)
	}

	want := []string{"https://docs.example.test/namespaced"}
	if !equalStrings(document.pageURLs, want) {
		t.Fatalf("page URLs = %#v; want %#v", document.pageURLs, want)
	}
}

func TestParseSitemapRejectsMalformedXML(t *testing.T) {
	_, err := parseSitemap(strings.NewReader(`<urlset><url><loc>https://docs.example.test/`))
	if err == nil {
		t.Fatal("parseSitemap() error = nil; want malformed XML error")
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
