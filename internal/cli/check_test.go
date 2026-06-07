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

func TestRunCheckFailOnNewIgnoresExistingBaselineIssue(t *testing.T) {
	current := report.Report{
		EntryURL: "https://docs.example.com/",
		Summary:  report.ReportSummary{DeadLinks: 1},
		Links: []report.LinkResult{
			{
				URL:      "https://docs.example.com/existing",
				FetchURL: "https://docs.example.com/existing",
				Dead:     true,
				Problem:  "network_error",
			},
		},
	}
	restore := stubCrawlRun(t, current)
	defer restore()

	dir := t.TempDir()
	baselinePath := filepath.Join(dir, "baseline.json")
	writeReportFixture(t, baselinePath, current)

	var stdout strings.Builder
	err := runCheck(checkOptions{
		scan: scanOptions{
			entryURL: "https://docs.example.com/",
			out:      filepath.Join(dir, "current.json"),
		},
		policy: checkeval.Options{
			FailOnDead: true,
			FailMode:   checkeval.FailModeNew,
		},
		baselinePath:  baselinePath,
		summaryFormat: "text",
	}, &stdout, nil)
	if err != nil {
		t.Fatalf("runCheck() error = %v; want existing baseline issue to pass in new mode", err)
	}
	if !strings.Contains(stdout.String(), "new=0 existing=1 resolved=0") {
		t.Fatalf("stdout = %q; want baseline diff counts", stdout.String())
	}
}

func TestRunCheckFailOnAllFailsExistingBaselineIssue(t *testing.T) {
	current := report.Report{
		EntryURL: "https://docs.example.com/",
		Summary:  report.ReportSummary{DeadLinks: 1},
		Links: []report.LinkResult{
			{
				URL:      "https://docs.example.com/existing",
				FetchURL: "https://docs.example.com/existing",
				Dead:     true,
				Problem:  "network_error",
			},
		},
	}
	restore := stubCrawlRun(t, current)
	defer restore()

	dir := t.TempDir()
	baselinePath := filepath.Join(dir, "baseline.json")
	writeReportFixture(t, baselinePath, current)

	err := runCheck(checkOptions{
		scan: scanOptions{
			entryURL: "https://docs.example.com/",
			out:      filepath.Join(dir, "current.json"),
		},
		policy: checkeval.Options{
			FailOnDead: true,
			FailMode:   checkeval.FailModeAll,
		},
		baselinePath:  baselinePath,
		summaryFormat: "text",
	}, io.Discard, nil)
	if err == nil {
		t.Fatal("runCheck() error = nil; want all-mode failure")
	}
	if !strings.Contains(err.Error(), "dead_links=1") {
		t.Fatalf("error = %q; want dead_links failure", err)
	}
}

func TestRunCheckFailOnNewFailsNewIssueAndWritesComparison(t *testing.T) {
	baselineReport := report.Report{
		EntryURL: "https://docs.example.com/",
		Links: []report.LinkResult{
			{
				URL:      "https://docs.example.com/existing",
				FetchURL: "https://docs.example.com/existing",
				Dead:     true,
				Problem:  "network_error",
			},
		},
	}
	current := report.Report{
		EntryURL: "https://docs.example.com/",
		Summary:  report.ReportSummary{DeadLinks: 2},
		Links: []report.LinkResult{
			baselineReport.Links[0],
			{
				URL:      "https://docs.example.com/new",
				FetchURL: "https://docs.example.com/new",
				Dead:     true,
				Problem:  "missing_fragment",
				Sources: []report.ReportSource{
					{PageURL: "https://docs.example.com/"},
				},
			},
		},
	}
	restore := stubCrawlRun(t, current)
	defer restore()

	dir := t.TempDir()
	baselinePath := filepath.Join(dir, "baseline.json")
	currentPath := filepath.Join(dir, "current.json")
	comparisonPath := filepath.Join(dir, "comparison.json")
	writeReportFixture(t, baselinePath, baselineReport)

	var stdout strings.Builder
	err := runCheck(checkOptions{
		scan: scanOptions{
			entryURL: "https://docs.example.com/",
			out:      currentPath,
		},
		policy: checkeval.Options{
			FailOnDead: true,
			FailMode:   checkeval.FailModeNew,
		},
		baselinePath:  baselinePath,
		comparisonOut: comparisonPath,
		summaryFormat: "markdown",
	}, &stdout, nil)
	if err == nil {
		t.Fatal("runCheck() error = nil; want new issue failure")
	}
	if !strings.Contains(err.Error(), "new_issues=1") {
		t.Fatalf("error = %q; want new issue failure", err)
	}
	if !strings.Contains(stdout.String(), "### New Issues") || !strings.Contains(stdout.String(), "https://docs.example.com/new") {
		t.Fatalf("stdout = %q; want markdown baseline summary", stdout.String())
	}

	currentData, err := os.ReadFile(currentPath)
	if err != nil {
		t.Fatalf("read current report: %v", err)
	}
	if !strings.Contains(string(currentData), `"dead_links": 2`) {
		t.Fatalf("current report = %s; want written before policy failure", currentData)
	}
	comparisonData, err := os.ReadFile(comparisonPath)
	if err != nil {
		t.Fatalf("read comparison: %v", err)
	}
	if !strings.Contains(string(comparisonData), `"new": 1`) || !strings.Contains(string(comparisonData), `"existing": 1`) {
		t.Fatalf("comparison = %s; want stable summary counts", comparisonData)
	}
	if !strings.Contains(string(comparisonData), "https://docs.example.com/new") {
		t.Fatalf("comparison = %s; want new issue URL", comparisonData)
	}
}

func TestRunCheckRejectsBaselineOutCollisionBeforeWriting(t *testing.T) {
	restore := stubCrawlRun(t, report.Report{
		EntryURL: "https://docs.example.com/",
		Summary:  report.ReportSummary{DeadLinks: 1},
		Links: []report.LinkResult{
			{
				URL:      "https://docs.example.com/new",
				FetchURL: "https://docs.example.com/new",
				Dead:     true,
				Problem:  "network_error",
			},
		},
	})
	defer restore()

	dir := t.TempDir()
	reportPath := filepath.Join(dir, "report.json")
	err := runCheck(checkOptions{
		scan: scanOptions{
			entryURL: "https://docs.example.com/",
			out:      reportPath,
		},
		policy: checkeval.Options{
			FailOnDead: true,
			FailMode:   checkeval.FailModeNew,
		},
		baselinePath:  reportPath,
		summaryFormat: "text",
	}, io.Discard, nil)
	if err == nil {
		t.Fatal("runCheck() error = nil; want path collision error")
	}
	if !strings.Contains(err.Error(), "--baseline") || !strings.Contains(err.Error(), "--out") {
		t.Fatalf("error = %q; want baseline/out collision", err)
	}
	if _, statErr := os.Stat(reportPath); !os.IsNotExist(statErr) {
		t.Fatalf("report path was created before collision error: %v", statErr)
	}
}

func TestRunCheckRejectsComparisonOutCollisions(t *testing.T) {
	current := report.Report{
		EntryURL: "https://docs.example.com/",
		Summary:  report.ReportSummary{DeadLinks: 1},
		Links: []report.LinkResult{
			{
				URL:      "https://docs.example.com/new",
				FetchURL: "https://docs.example.com/new",
				Dead:     true,
				Problem:  "network_error",
			},
		},
	}
	restore := stubCrawlRun(t, current)
	defer restore()

	tests := []struct {
		name          string
		collision     string
		wantSubstring string
	}{
		{name: "with out", collision: "out", wantSubstring: "--out"},
		{name: "with baseline", collision: "baseline", wantSubstring: "--baseline"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			outPath := filepath.Join(dir, "current.json")
			baselinePath := filepath.Join(dir, "baseline.json")
			comparisonPath := filepath.Join(dir, "comparison.json")
			writeReportFixture(t, baselinePath, current)

			switch tt.collision {
			case "out":
				comparisonPath = outPath
			case "baseline":
				comparisonPath = baselinePath
			}

			err := runCheck(checkOptions{
				scan: scanOptions{
					entryURL: "https://docs.example.com/",
					out:      outPath,
				},
				policy: checkeval.Options{
					FailOnDead: true,
					FailMode:   checkeval.FailModeNew,
				},
				baselinePath:  baselinePath,
				comparisonOut: comparisonPath,
				summaryFormat: "text",
			}, io.Discard, nil)
			if err == nil {
				t.Fatal("runCheck() error = nil; want path collision error")
			}
			if !strings.Contains(err.Error(), "--comparison-out") || !strings.Contains(err.Error(), tt.wantSubstring) {
				t.Fatalf("error = %q; want comparison collision with %s", err, tt.wantSubstring)
			}

			data, readErr := os.ReadFile(baselinePath)
			if readErr != nil {
				t.Fatalf("read baseline: %v", readErr)
			}
			if !strings.Contains(string(data), `"dead_links": 1`) {
				t.Fatalf("baseline was overwritten: %s", data)
			}
			if tt.collision == "out" {
				if _, statErr := os.Stat(outPath); !os.IsNotExist(statErr) {
					t.Fatalf("out path was created before collision error: %v", statErr)
				}
			}
		})
	}
}

func writeReportFixture(t *testing.T, path string, reportData report.Report) {
	t.Helper()

	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create report fixture: %v", err)
	}
	defer file.Close()
	if err := report.Write(file, reportData); err != nil {
		t.Fatalf("write report fixture: %v", err)
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
