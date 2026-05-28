package crawl

import (
	"testing"
)

func TestNormalizeLink(t *testing.T) {
	t.Run("relative_links_are_resolved_and_normalized", func(t *testing.T) {
		got, err := NormalizeLink("../Guide?b=2&a=1#intro", "https://docs.EXAMPLE.com/docs/index.html")
		if err != nil {
			t.Fatalf("NormalizeLink() error = %v", err)
		}
		wantLink := "https://docs.example.com/Guide?b=2&a=1#intro"
		wantFetch := "https://docs.example.com/Guide?b=2&a=1"
		if got.LinkURL != wantLink || got.FetchURL != wantFetch {
			t.Fatalf("NormalizeLink() = %#v; want link %q, fetch %q", got, wantLink, wantFetch)
		}
	})

	t.Run("empty_path_is_root", func(t *testing.T) {
		got, err := NormalizeLink("https://Example.Com", "https://base.example.com/")
		if err != nil {
			t.Fatalf("NormalizeLink() error = %v", err)
		}
		if got.FetchURL != "https://example.com/" {
			t.Fatalf("NormalizeLink() fetch = %q; want %q", got.FetchURL, "https://example.com/")
		}
	})

	t.Run("empty_href_ignored", func(t *testing.T) {
		if _, err := NormalizeLink("", "https://example.com/"); err != ErrEmptyHref {
			t.Fatalf("NormalizeLink() error = %v; want ErrEmptyHref", err)
		}
	})

	t.Run("unsupported_scheme_ignored", func(t *testing.T) {
		if _, err := NormalizeLink("mailto:foo@example.com", "https://example.com/"); err != ErrUnsupportedScheme {
			t.Fatalf("NormalizeLink() error = %v; want ErrUnsupportedScheme", err)
		}
	})

	t.Run("default_http_https_port_removed", func(t *testing.T) {
		got, err := NormalizeLink("https://Example.COM:443/install", "https://example.com/")
		if err != nil {
			t.Fatalf("NormalizeLink() error = %v", err)
		}
		if got.FetchURL != "https://example.com/install" {
			t.Fatalf("NormalizeLink() fetch = %q; want %q", got.FetchURL, "https://example.com/install")
		}
		got, err = NormalizeLink("http://Example.COM:80/install", "https://example.com/")
		if err != nil {
			t.Fatalf("NormalizeLink() error = %v", err)
		}
		if got.FetchURL != "http://example.com/install" {
			t.Fatalf("NormalizeLink() fetch = %q; want %q", got.FetchURL, "http://example.com/install")
		}
	})

	t.Run("preserves_query_order", func(t *testing.T) {
		got, err := NormalizeLink("https://example.com/page?b=2&a=1", "https://example.com/")
		if err != nil {
			t.Fatalf("NormalizeLink() error = %v", err)
		}
		if got.FetchURL != "https://example.com/page?b=2&a=1" {
			t.Fatalf("NormalizeLink() fetch = %q; want %q", got.FetchURL, "https://example.com/page?b=2&a=1")
		}
		if got.LinkURL != "https://example.com/page?b=2&a=1" {
			t.Fatalf("NormalizeLink() link = %q; want %q", got.LinkURL, "https://example.com/page?b=2&a=1")
		}
	})

	t.Run("link_url_keeps_fragment_fetch_url_drops_it", func(t *testing.T) {
		got, err := NormalizeLink("https://example.com/page#section", "https://example.com/")
		if err != nil {
			t.Fatalf("NormalizeLink() error = %v", err)
		}
		if got.LinkURL != "https://example.com/page#section" {
			t.Fatalf("NormalizeLink() link = %q; want %q", got.LinkURL, "https://example.com/page#section")
		}
		if got.FetchURL != "https://example.com/page" {
			t.Fatalf("NormalizeLink() fetch = %q; want %q", got.FetchURL, "https://example.com/page")
		}
	})
}

func TestScope(t *testing.T) {
	scope, err := NewScope("https://docs.EXAMPLE.com", []string{"https://www.example.com", "http://not-in-scope.example.com"}, "/docs/")
	if err != nil {
		t.Fatalf("NewScope() error = %v", err)
	}

	t.Run("same_origin_allowed_by_default", func(t *testing.T) {
		got, err := scope.Check("https://docs.example.com/docs/intro")
		if err != nil {
			t.Fatalf("Check() error = %v", err)
		}
		if !got.Allowed || got.Reason != "" {
			t.Fatalf("scope.Check() = %#v; want allowed with no reason", got)
		}
	})

	t.Run("allowed_hosts_supported", func(t *testing.T) {
		got, err := scope.Check("https://www.example.com/docs/")
		if err != nil {
			t.Fatalf("Check() error = %v", err)
		}
		if !got.Allowed || got.Reason != "" {
			t.Fatalf("scope.Check() = %#v; want allowed with no reason", got)
		}
	})

	t.Run("path_prefix_enforced", func(t *testing.T) {
		got, err := scope.Check("https://docs.example.com/blog/")
		if err != nil {
			t.Fatalf("Check() error = %v", err)
		}
		if got.Allowed || got.Reason != ScopeReasonOutsidePathPrefix {
			t.Fatalf("scope.Check() = %#v; want reason %q", got, ScopeReasonOutsidePathPrefix)
		}
	})

	t.Run("external_origin_rejected", func(t *testing.T) {
		got, err := scope.Check("https://api.example.com/")
		if err != nil {
			t.Fatalf("Check() error = %v", err)
		}
		if got.Allowed || got.Reason != ScopeReasonExternalOrigin {
			t.Fatalf("scope.Check() = %#v; want reason %q", got, ScopeReasonExternalOrigin)
		}
	})

	t.Run("scheme_mismatch_rejected", func(t *testing.T) {
		got, err := scope.Check("http://www.example.com/docs/")
		if err != nil {
			t.Fatalf("Check() error = %v", err)
		}
		if got.Allowed || got.Reason != ScopeReasonExternalOrigin {
			t.Fatalf("scope.Check() = %#v; want reason %q", got, ScopeReasonExternalOrigin)
		}
	})
}
