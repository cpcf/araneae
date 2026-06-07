package baseline

import (
	"bytes"
	"testing"

	"github.com/cpcf/araneae/internal/report"
)

func TestIssueKeyMatchesSameURLAndProblem(t *testing.T) {
	a := KeyForLink(report.LinkResult{
		URL:        "https://docs.example.com/missing",
		Problem:    "http_status",
		StatusCode: 404,
		Non200:     true,
	})
	b := KeyForLink(report.LinkResult{
		URL:        "https://docs.example.com/missing",
		Problem:    "http_status",
		StatusCode: 500,
		Non200:     true,
	})

	if a != b {
		t.Fatalf("keys differ for same URL/problem: %#v != %#v", a, b)
	}
}

func TestIssueKeyChangesWithURLOrProblem(t *testing.T) {
	base := KeyForLink(report.LinkResult{
		URL:     "https://docs.example.com/missing",
		Problem: "missing_fragment",
		Dead:    true,
	})
	changedURL := KeyForLink(report.LinkResult{
		URL:     "https://docs.example.com/other",
		Problem: "missing_fragment",
		Dead:    true,
	})
	changedProblem := KeyForLink(report.LinkResult{
		URL:     "https://docs.example.com/missing",
		Problem: "network_error",
		Dead:    true,
	})

	if base == changedURL {
		t.Fatal("key did not change when URL changed")
	}
	if base == changedProblem {
		t.Fatal("key did not change when problem changed")
	}
}

func TestCompareNewResolvedExistingAndUnchangedOK(t *testing.T) {
	baselineReport := report.Report{
		EntryURL: "https://docs.example.com/",
		Links: []report.LinkResult{
			issueLink("https://docs.example.com/existing", "network_error", 0, true, false, 1, 1),
			issueLink("https://docs.example.com/resolved", "http_status", 404, true, true, 1, 1),
			okLink("https://docs.example.com/ok", 1, 1),
		},
	}
	current := report.Report{
		EntryURL: "https://docs.example.com/",
		Links: []report.LinkResult{
			issueLink("https://docs.example.com/existing", "network_error", 0, true, false, 3, 2),
			issueLink("https://docs.example.com/new", "missing_fragment", 200, true, false, 1, 1),
			okLink("https://docs.example.com/ok", 1, 1),
		},
	}

	comparison := Compare(&baselineReport, current, Options{IncludeDead: true, IncludeNon200: true})

	if comparison.Summary.New != 1 || comparison.Summary.Existing != 1 || comparison.Summary.Resolved != 1 || comparison.Summary.UnchangedOK != 1 {
		t.Fatalf("summary = %#v", comparison.Summary)
	}
	if comparison.New[0].URL != "https://docs.example.com/new" {
		t.Fatalf("new = %#v", comparison.New)
	}
	if comparison.Existing[0].URL != "https://docs.example.com/existing" {
		t.Fatalf("existing = %#v", comparison.Existing)
	}
	if comparison.Existing[0].Count != 3 || comparison.Existing[0].SourcePages != 2 {
		t.Fatalf("existing details = %#v; want current count/source pages", comparison.Existing[0])
	}
	if comparison.Resolved[0].URL != "https://docs.example.com/resolved" {
		t.Fatalf("resolved = %#v", comparison.Resolved)
	}
	if comparison.UnchangedOK[0].URL != "https://docs.example.com/ok" {
		t.Fatalf("unchanged_ok = %#v", comparison.UnchangedOK)
	}
}

func TestCompareWithoutBaselineTreatsCurrentIssuesAsNew(t *testing.T) {
	current := report.Report{
		EntryURL: "https://docs.example.com/",
		Links: []report.LinkResult{
			issueLink("https://docs.example.com/new", "network_error", 0, true, false, 1, 1),
			okLink("https://docs.example.com/ok", 1, 1),
		},
	}

	comparison := Compare(nil, current, Options{IncludeDead: true})

	if comparison.Summary.New != 1 {
		t.Fatalf("new = %d; want 1", comparison.Summary.New)
	}
	if comparison.Summary.UnchangedOK != 1 {
		t.Fatalf("unchanged_ok = %d; want current ok link without baseline", comparison.Summary.UnchangedOK)
	}
}

func TestWriteComparisonJSON(t *testing.T) {
	comparison := Comparison{
		SchemaVersion: ComparisonSchemaVersion,
		BaselineURL:   "https://docs.example.com/",
		CurrentURL:    "https://docs.example.com/",
		Summary:       Summary{New: 1},
		New: []Issue{
			{
				Key:         IssueKey{URL: "https://docs.example.com/new", Problem: "network_error"},
				URL:         "https://docs.example.com/new",
				Problem:     "network_error",
				SourcePages: 2,
				Dead:        true,
			},
		},
	}

	var out bytes.Buffer
	if err := Write(&out, comparison); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	want := `{
  "schema_version": 1,
  "baseline_entry_url": "https://docs.example.com/",
  "current_entry_url": "https://docs.example.com/",
  "summary": {
    "new": 1,
    "existing": 0,
    "resolved": 0,
    "unchanged_ok": 0
  },
  "new": [
    {
      "key": {
        "url": "https://docs.example.com/new",
        "problem": "network_error"
      },
      "url": "https://docs.example.com/new",
      "fetch_url": "",
      "problem": "network_error",
      "status_code": 0,
      "count": 0,
      "source_pages": 2,
      "dead": true,
      "non_200": false
    }
  ],
  "existing": [],
  "resolved": [],
  "unchanged_ok": []
}
`
	if out.String() != want {
		t.Fatalf("Write() =\n%s\nwant:\n%s", out.String(), want)
	}
}

func issueLink(url, problem string, statusCode int, dead, non200 bool, count, sourcePages int) report.LinkResult {
	sources := make([]report.ReportSource, 0, sourcePages)
	for i := 0; i < sourcePages; i++ {
		sources = append(sources, report.ReportSource{PageURL: url})
	}
	return report.LinkResult{
		URL:        url,
		FetchURL:   url,
		Problem:    problem,
		StatusCode: statusCode,
		Dead:       dead,
		Non200:     non200,
		Count:      count,
		Sources:    sources,
	}
}

func okLink(url string, count, sourcePages int) report.LinkResult {
	sources := make([]report.ReportSource, 0, sourcePages)
	for i := 0; i < sourcePages; i++ {
		sources = append(sources, report.ReportSource{PageURL: url})
	}
	return report.LinkResult{
		URL:        url,
		FetchURL:   url,
		StatusCode: 200,
		OK:         true,
		Count:      count,
		Sources:    sources,
	}
}
