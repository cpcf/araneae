package check

import (
	"strings"
	"testing"

	"github.com/cpcf/araneae/internal/baseline"
	"github.com/cpcf/araneae/internal/report"
)

func TestEvaluateNoFailureReport(t *testing.T) {
	result := Evaluate(report.Report{
		Summary: report.ReportSummary{
			LinksDiscovered:  2,
			LinkOccurrences:  3,
			FetchesAttempted: 2,
			OKLinks:          2,
		},
		Links: []report.LinkResult{
			{URL: "https://docs.example.com/", OK: true, StatusCode: 200},
		},
	}, Options{
		FailOnDead:      true,
		FailOnNon200:    true,
		FailOnTruncated: true,
	})

	if result.Failed() {
		t.Fatalf("Failed() = true; want false")
	}
	if err := result.Err(); err != nil {
		t.Fatalf("Err() = %v; want nil", err)
	}
	if len(result.Failures) != 0 {
		t.Fatalf("Failures = %#v; want empty", result.Failures)
	}
}

func TestEvaluateDeadLinks(t *testing.T) {
	result := Evaluate(report.Report{
		Summary: report.ReportSummary{DeadLinks: 1},
		Links: []report.LinkResult{
			{
				URL:      "https://docs.example.com/missing",
				FetchURL: "https://docs.example.com/missing",
				Dead:     true,
				Problem:  "network_error",
				Sources:  []report.ReportSource{{PageURL: "https://docs.example.com/"}},
			},
		},
	}, Options{FailOnDead: true})

	if !result.Failed() {
		t.Fatal("Failed() = false; want true")
	}
	if len(result.Failures) != 1 {
		t.Fatalf("Failures = %#v; want 1", result.Failures)
	}
	if result.Failures[0].SourcePages != 1 {
		t.Fatalf("SourcePages = %d; want 1", result.Failures[0].SourcePages)
	}
	if err := result.Err(); err == nil || !strings.Contains(err.Error(), "dead_links=1") {
		t.Fatalf("Err() = %v; want dead_links failure", err)
	}
}

func TestEvaluateNon200Links(t *testing.T) {
	result := Evaluate(report.Report{
		Summary: report.ReportSummary{Non200Links: 1},
		Links: []report.LinkResult{
			{
				URL:        "https://docs.example.com/server-error",
				FetchURL:   "https://docs.example.com/server-error",
				Non200:     true,
				Problem:    "http_status",
				StatusCode: 500,
			},
		},
	}, Options{FailOnNon200: true})

	if !result.Failed() {
		t.Fatal("Failed() = false; want true")
	}
	if len(result.Failures) != 1 {
		t.Fatalf("Failures = %#v; want 1", result.Failures)
	}
	if result.Failures[0].StatusCode != 500 {
		t.Fatalf("StatusCode = %d; want 500", result.Failures[0].StatusCode)
	}
	if err := result.Err(); err == nil || !strings.Contains(err.Error(), "non_200_links=1") {
		t.Fatalf("Err() = %v; want non_200_links failure", err)
	}
}

func TestEvaluateMissingFragmentsAsDeadLinks(t *testing.T) {
	result := Evaluate(report.Report{
		Summary: report.ReportSummary{DeadLinks: 1},
		Links: []report.LinkResult{
			{
				URL:        "https://docs.example.com/install#requirements",
				FetchURL:   "https://docs.example.com/install",
				Dead:       true,
				Problem:    "missing_fragment",
				StatusCode: 200,
			},
		},
	}, Options{FailOnDead: true})

	if !result.Failed() {
		t.Fatal("Failed() = false; want true")
	}
	if len(result.Failures) != 1 {
		t.Fatalf("Failures = %#v; want 1", result.Failures)
	}
	if result.Failures[0].Problem != "missing_fragment" {
		t.Fatalf("Problem = %q; want missing_fragment", result.Failures[0].Problem)
	}
}

func TestEvaluateTruncatedReport(t *testing.T) {
	result := Evaluate(report.Report{
		Summary: report.ReportSummary{
			Truncated:     true,
			UnvisitedURLs: 3,
		},
	}, Options{FailOnTruncated: true})

	if !result.Failed() {
		t.Fatal("Failed() = false; want true")
	}
	if !result.TruncatedFailed {
		t.Fatal("TruncatedFailed = false; want true")
	}
	if err := result.Err(); err == nil || !strings.Contains(err.Error(), "truncated=true unvisited_urls=3") {
		t.Fatalf("Err() = %v; want truncated failure", err)
	}
}

func TestEvaluateFailModeNewUsesComparison(t *testing.T) {
	result := Evaluate(report.Report{
		Summary: report.ReportSummary{DeadLinks: 1},
		Links: []report.LinkResult{
			{
				URL:      "https://docs.example.com/existing",
				FetchURL: "https://docs.example.com/existing",
				Dead:     true,
				Problem:  "network_error",
			},
		},
	}, Options{
		FailOnDead: true,
		FailMode:   FailModeNew,
		Comparison: &baseline.Comparison{
			Summary:  baseline.Summary{New: 0, Existing: 1},
			Existing: []baseline.Issue{{URL: "https://docs.example.com/existing", Problem: "network_error"}},
		},
	})

	if result.Failed() {
		t.Fatal("Failed() = true; want false for existing-only issue in new mode")
	}

	result.Comparison.New = []baseline.Issue{{URL: "https://docs.example.com/new", Problem: "network_error"}}
	result.Comparison.Summary.New = 1
	if !result.Failed() {
		t.Fatal("Failed() = false; want true for new issue in new mode")
	}
	if err := result.Err(); err == nil || !strings.Contains(err.Error(), "new_issues=1") {
		t.Fatalf("Err() = %v; want new_issues failure", err)
	}
}

func TestMarkdownSummary(t *testing.T) {
	result := Evaluate(report.Report{
		Summary: report.ReportSummary{
			LinksDiscovered:  2,
			LinkOccurrences:  4,
			FetchesAttempted: 2,
			OKLinks:          1,
			DeadLinks:        1,
			Non200Links:      1,
			SkippedLinks:     1,
		},
		Links: []report.LinkResult{
			{
				URL:        "https://docs.example.com/missing|pipe",
				FetchURL:   "https://docs.example.com/missing%7Cpipe",
				Dead:       true,
				Non200:     true,
				Problem:    "http_status",
				StatusCode: 404,
				Sources: []report.ReportSource{
					{PageURL: "https://docs.example.com/"},
					{PageURL: "https://docs.example.com/install"},
				},
			},
		},
	}, Options{FailOnDead: true, FailOnNon200: true})

	got := MarkdownSummary(result)
	want := `## Araneae Check Summary

**Status:** FAIL

| Metric | Count |
| --- | ---: |
| Links discovered | 2 |
| Link occurrences | 4 |
| Fetches attempted | 2 |
| OK links | 1 |
| Dead links | 1 |
| Non-200 links | 1 |
| Skipped links | 1 |
| Unvisited URLs | 0 |

### Top Problems

| URL | Problem | Status | Sources |
| --- | --- | ---: | ---: |
| https://docs.example.com/missing\|pipe | http_status | 404 | 2 |
`
	if got != want {
		t.Fatalf("MarkdownSummary() =\n%s\nwant:\n%s", got, want)
	}
}

func TestMarkdownSummaryWithBaselineDiff(t *testing.T) {
	result := Evaluate(report.Report{
		Summary: report.ReportSummary{LinksDiscovered: 3, DeadLinks: 2},
	}, Options{
		FailOnDead: true,
		FailMode:   FailModeNew,
		Comparison: &baseline.Comparison{
			Summary: baseline.Summary{New: 1, Existing: 1, Resolved: 1, UnchangedOK: 2},
			New: []baseline.Issue{
				{URL: "https://docs.example.com/new", Problem: "missing_fragment", StatusCode: 200, SourcePages: 2},
			},
			Existing: []baseline.Issue{
				{URL: "https://docs.example.com/existing", Problem: "network_error", SourcePages: 1},
			},
			Resolved: []baseline.Issue{
				{URL: "https://docs.example.com/resolved", Problem: "http_status", StatusCode: 404, SourcePages: 1},
			},
		},
	})

	got := MarkdownSummary(result)
	want := `## Araneae Check Summary

**Status:** FAIL

| Metric | Count |
| --- | ---: |
| Links discovered | 3 |
| Link occurrences | 0 |
| Fetches attempted | 0 |
| OK links | 0 |
| Dead links | 2 |
| Non-200 links | 0 |
| Skipped links | 0 |
| Unvisited URLs | 0 |

### Baseline Diff

| Group | Count |
| --- | ---: |
| New issues | 1 |
| Existing issues | 1 |
| Resolved issues | 1 |
| Unchanged OK links | 2 |

### New Issues

| URL | Problem | Status | Sources |
| --- | --- | ---: | ---: |
| https://docs.example.com/new | missing_fragment | 200 | 2 |

### Existing Issues

| URL | Problem | Status | Sources |
| --- | --- | ---: | ---: |
| https://docs.example.com/existing | network_error | - | 1 |

### Resolved Issues

| URL | Problem | Status | Sources |
| --- | --- | ---: | ---: |
| https://docs.example.com/resolved | http_status | 404 | 1 |
`
	if got != want {
		t.Fatalf("MarkdownSummary() =\n%s\nwant:\n%s", got, want)
	}
}
