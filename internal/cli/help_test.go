package cli

import (
	"strings"
	"testing"
)

func TestRunScanHelpWritesUsage(t *testing.T) {
	var stdout strings.Builder
	if err := runScanCommand([]string{"--help"}, &stdout); err != nil {
		t.Fatalf("runScanCommand(--help) error = %v", err)
	}

	got := stdout.String()
	assertContainsAll(t, got,
		"usage: araneae scan [entry-url] [flags]",
		"-config",
		"-max-pages",
		"-sitemap",
		"-max-sitemap-urls",
		"-fail-on-dead",
	)
}

func TestRunCheckHelpWritesUsage(t *testing.T) {
	var stdout strings.Builder
	if err := runCheckCommand([]string{"--help"}, &stdout, nil); err != nil {
		t.Fatalf("runCheckCommand(--help) error = %v", err)
	}

	got := stdout.String()
	assertContainsAll(t, got,
		"usage: araneae check [entry-url] [flags]",
		"-config",
		"-max-pages",
		"-sitemap",
		"-max-sitemap-urls",
		"-fail-on-truncated",
		"-baseline",
		"-fail-on",
		"-comparison-out",
		"-summary",
		"-github-step-summary",
	)
}

func TestRunServeHelpWritesUsage(t *testing.T) {
	var stdout strings.Builder
	if err := runServeCommand([]string{"--help"}, &stdout); err != nil {
		t.Fatalf("runServeCommand(--help) error = %v", err)
	}

	got := stdout.String()
	assertContainsAll(t, got,
		"usage: araneae serve <report-path> [flags]",
		"-addr",
		"-baseline",
	)
}

func assertContainsAll(t *testing.T, got string, wants ...string) {
	t.Helper()

	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Fatalf("output = %q; missing %q", got, want)
		}
	}
}
