package cli

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	checkeval "github.com/cpcf/araneae/internal/check"
	"github.com/cpcf/araneae/internal/crawl"
	"github.com/cpcf/araneae/internal/report"
)

func TestRunCheckWritesReportSummaryAndReturnsPolicyError(t *testing.T) {
	restore := stubCrawlRun(t, report.Report{
		SchemaVersion: report.SchemaVersion,
		GeneratedAt:   time.Date(2026, 6, 6, 10, 0, 0, 0, time.UTC),
		EntryURL:      "https://docs.example.com/",
		Summary: report.ReportSummary{
			LinksDiscovered:  1,
			LinkOccurrences:  1,
			FetchesAttempted: 1,
			DeadLinks:        1,
		},
		Links: []report.LinkResult{
			{
				URL:      "https://docs.example.com/missing",
				FetchURL: "https://docs.example.com/missing",
				Dead:     true,
				Problem:  "network_error",
			},
		},
	})
	defer restore()

	outPath := filepath.Join(t.TempDir(), "report.json")
	var stdout strings.Builder
	err := runCheck(checkOptions{
		scan: scanOptions{
			entryURL:         "https://docs.example.com/",
			out:              outPath,
			maxPages:         1,
			timeout:          time.Second,
			concurrency:      1,
			maxResponseBytes: 1024,
			userAgent:        "test-agent",
		},
		policy:        checkeval.Options{FailOnDead: true},
		summaryFormat: "text",
	}, &stdout, nil)

	if err == nil {
		t.Fatal("runCheck() error = nil; want policy failure")
	}
	if !strings.Contains(err.Error(), "dead_links=1") {
		t.Fatalf("error = %q; want dead link failure", err)
	}
	if !strings.Contains(stdout.String(), "status=fail") || !strings.Contains(stdout.String(), "dead=1") {
		t.Fatalf("stdout = %q; want failing text summary", stdout.String())
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	if !strings.Contains(string(data), `"dead_links": 1`) {
		t.Fatalf("report JSON = %s; want dead_links count", data)
	}
}

func TestRunCheckWritesMarkdownSummary(t *testing.T) {
	restore := stubCrawlRun(t, report.Report{
		Summary: report.ReportSummary{LinksDiscovered: 1, OKLinks: 1},
		Links:   []report.LinkResult{{URL: "https://docs.example.com/", OK: true}},
	})
	defer restore()

	var stdout strings.Builder
	err := runCheck(checkOptions{
		scan: scanOptions{
			entryURL: "https://docs.example.com/",
			out:      filepath.Join(t.TempDir(), "report.json"),
		},
		summaryFormat: "markdown",
	}, &stdout, nil)
	if err != nil {
		t.Fatalf("runCheck() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "## Araneae Check Summary") {
		t.Fatalf("stdout = %q; want markdown summary", stdout.String())
	}
	if !strings.Contains(stdout.String(), "**Status:** PASS") {
		t.Fatalf("stdout = %q; want pass status", stdout.String())
	}
}

func TestRunCheckWritesGithubStepSummaryFromCIEnv(t *testing.T) {
	restore := stubCrawlRun(t, report.Report{
		Summary: report.ReportSummary{LinksDiscovered: 1, OKLinks: 1},
	})
	defer restore()

	dir := t.TempDir()
	stepSummary := filepath.Join(dir, "step.md")
	err := runCheck(checkOptions{
		scan: scanOptions{
			entryURL: "https://docs.example.com/",
			out:      filepath.Join(dir, "report.json"),
		},
		ci:            true,
		summaryFormat: "text",
	}, io.Discard, func(name string) string {
		if name == "GITHUB_STEP_SUMMARY" {
			return stepSummary
		}
		return ""
	})
	if err != nil {
		t.Fatalf("runCheck() error = %v", err)
	}

	data, err := os.ReadFile(stepSummary)
	if err != nil {
		t.Fatalf("read step summary: %v", err)
	}
	if !strings.Contains(string(data), "## Araneae Check Summary") {
		t.Fatalf("step summary = %q; want markdown", string(data))
	}
}

func TestRunCheckWritesExplicitGithubStepSummaryWithoutCI(t *testing.T) {
	restore := stubCrawlRun(t, report.Report{
		Summary: report.ReportSummary{LinksDiscovered: 1, OKLinks: 1},
	})
	defer restore()

	dir := t.TempDir()
	stepSummary := filepath.Join(dir, "explicit.md")
	err := runCheck(checkOptions{
		scan: scanOptions{
			entryURL: "https://docs.example.com/",
			out:      filepath.Join(dir, "report.json"),
		},
		githubStepSummary: stepSummary,
		summaryFormat:     "text",
	}, io.Discard, func(string) string {
		return ""
	})
	if err != nil {
		t.Fatalf("runCheck() error = %v", err)
	}

	data, err := os.ReadFile(stepSummary)
	if err != nil {
		t.Fatalf("read step summary: %v", err)
	}
	if !strings.Contains(string(data), "## Araneae Check Summary") {
		t.Fatalf("step summary = %q; want markdown", string(data))
	}
}

func stubCrawlRun(t *testing.T, reportData report.Report) func() {
	t.Helper()

	previous := crawlRun
	crawlRun = func(context.Context, crawl.ScanOptions) (report.Report, error) {
		return reportData, nil
	}
	return func() {
		crawlRun = previous
	}
}
