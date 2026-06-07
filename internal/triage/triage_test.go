package triage

import (
	"strings"
	"testing"

	"github.com/cpcf/araneae/internal/baseline"
	"github.com/cpcf/araneae/internal/report"
)

func TestSeverityForLink(t *testing.T) {
	tests := []struct {
		name string
		link report.LinkResult
		want Severity
	}{
		{
			name: "not found",
			link: report.LinkResult{Dead: true, Non200: true, Problem: "http_status", StatusCode: 404},
			want: SeverityCritical,
		},
		{
			name: "gone",
			link: report.LinkResult{Dead: true, Non200: true, Problem: "http_status", StatusCode: 410},
			want: SeverityCritical,
		},
		{
			name: "missing fragment",
			link: report.LinkResult{Dead: true, Problem: "missing_fragment", StatusCode: 200},
			want: SeverityCritical,
		},
		{
			name: "server error",
			link: report.LinkResult{Non200: true, Problem: "http_status", StatusCode: 500},
			want: SeverityWarning,
		},
		{
			name: "redirect",
			link: report.LinkResult{OK: true, FetchURL: "https://docs.example.test/old", FinalURL: "https://docs.example.test/new", StatusCode: 200},
			want: SeverityInfo,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SeverityForLink(tt.link); got != tt.want {
				t.Fatalf("SeverityForLink() = %q; want %q", got, tt.want)
			}
		})
	}
}

func TestFingerprintStableAndProblemSensitive(t *testing.T) {
	first := Fingerprint("https://docs.example.test/missing", "http_status")
	second := Fingerprint("https://docs.example.test/missing", "http_status")
	otherProblem := Fingerprint("https://docs.example.test/missing", "network_error")

	if first == "" {
		t.Fatal("Fingerprint() = empty")
	}
	if first != second {
		t.Fatalf("fingerprints differ for same input: %q vs %q", first, second)
	}
	if first == otherProblem {
		t.Fatalf("fingerprint did not change when problem changed")
	}
}

func TestIssuesAnnotatesStateAndGroups(t *testing.T) {
	reportData := report.Report{
		Links: []report.LinkResult{
			{
				URL:        "https://docs.example.test/missing",
				FetchURL:   "https://docs.example.test/missing",
				Dead:       true,
				Non200:     true,
				Problem:    "http_status",
				StatusCode: 404,
				Count:      2,
				Sources: []report.ReportSource{
					{PageURL: "https://docs.example.test/", Count: 2, Texts: []string{"Broken"}},
					{PageURL: "https://docs.example.test/guide", Count: 1, Texts: []string{"Guide"}},
				},
			},
			{
				URL:        "https://docs.example.test/flaky",
				FetchURL:   "https://docs.example.test/flaky",
				Non200:     true,
				Problem:    "http_status",
				StatusCode: 500,
				Count:      1,
				Sources: []report.ReportSource{
					{PageURL: "https://docs.example.test/install", Count: 1, Texts: []string{"Flaky"}},
				},
			},
			{
				URL:        "https://docs.example.test/ok",
				FetchURL:   "https://docs.example.test/ok",
				OK:         true,
				StatusCode: 200,
			},
			{
				URL:        "https://docs.example.test/old",
				FetchURL:   "https://docs.example.test/old",
				FinalURL:   "https://docs.example.test/new",
				OK:         true,
				StatusCode: 200,
			},
		},
	}

	issues := Issues(reportData, &baseline.Comparison{
		New: []baseline.Issue{{URL: "https://docs.example.test/missing", Problem: "http_status"}},
	})
	if len(issues) != 3 {
		t.Fatalf("Issues() = %#v; want 2 failing issues plus redirect", issues)
	}
	if issues[0].Severity != SeverityCritical || issues[0].State != StateNew {
		t.Fatalf("first issue severity/state = %q/%q; want critical/new", issues[0].Severity, issues[0].State)
	}
	if issues[0].FirstSource != "https://docs.example.test/" {
		t.Fatalf("first source = %q", issues[0].FirstSource)
	}
	if issues[0].TargetHost != "docs.example.test" {
		t.Fatalf("target host = %q", issues[0].TargetHost)
	}

	summary := Summarize(issues)
	if summary.Total != 3 || summary.Critical != 1 || summary.Warning != 1 || summary.Info != 1 || summary.New != 1 || summary.Unknown != 2 {
		t.Fatalf("summary = %#v; want one critical, one warning, one info, one new, two unknown", summary)
	}

	problems := GroupByProblem(issues)
	if len(problems) != 2 || problems[0].Key != "http_status" || problems[0].Count != 2 || problems[1].Key != "redirect" {
		t.Fatalf("problem groups = %#v", problems)
	}
	hosts := GroupByTargetHost(issues)
	if len(hosts) != 1 || hosts[0].Key != "docs.example.test" {
		t.Fatalf("host groups = %#v", hosts)
	}
	sources := GroupBySourcePage(issues)
	if len(sources) != 4 {
		t.Fatalf("source groups = %#v; want first, guide, install, and unknown", sources)
	}
	if !hasGroup(sources, "https://docs.example.test/guide") {
		t.Fatalf("source groups = %#v; want secondary source page", sources)
	}
}

func TestFilterIssues(t *testing.T) {
	issues := []Issue{
		{
			Fingerprint: "a",
			Severity:    SeverityCritical,
			State:       StateNew,
			URL:         "https://docs.example.test/missing",
			Problem:     "http_status",
			TargetHost:  "docs.example.test",
			FirstSource: "https://docs.example.test/",
			Snippets:    []string{"Broken"},
			Link: report.LinkResult{
				Sources: []report.ReportSource{
					{PageURL: "https://docs.example.test/"},
					{PageURL: "https://docs.example.test/guide"},
				},
			},
		},
		{
			Fingerprint: "b",
			Severity:    SeverityWarning,
			State:       StateExisting,
			URL:         "https://api.example.test/flaky",
			Problem:     "http_status",
			TargetHost:  "api.example.test",
			FirstSource: "https://docs.example.test/api",
			Snippets:    []string{"API"},
		},
		{
			Fingerprint: "c",
			Severity:    SeverityCritical,
			State:       StateUnknown,
			URL:         "not a url",
			Problem:     "network_error",
			TargetHost:  "",
			FirstSource: "",
		},
	}

	got := FilterIssues(issues, Filter{Severity: SeverityCritical, Query: "broken"}, nil)
	if len(got) != 1 || got[0].Fingerprint != "a" {
		t.Fatalf("critical broken filter = %#v", got)
	}

	got = FilterIssues(issues, Filter{TargetHost: "api.example.test"}, map[string]bool{"b": true})
	if len(got) != 0 {
		t.Fatalf("acknowledged issue was not hidden: %#v", got)
	}

	got = FilterIssues(issues, Filter{SourcePage: "https://docs.example.test/guide"}, nil)
	if len(got) != 1 || got[0].Fingerprint != "a" {
		t.Fatalf("secondary source page filter = %#v", got)
	}

	got = FilterIssues(issues, Filter{SourcePage: UnknownSourceKey}, nil)
	if len(got) != 1 || got[0].Fingerprint != "c" {
		t.Fatalf("unknown source filter = %#v", got)
	}

	got = FilterIssues(issues, Filter{TargetHost: UnknownHostKey}, nil)
	if len(got) != 1 || got[0].Fingerprint != "c" {
		t.Fatalf("unknown host filter = %#v", got)
	}
}

func TestMarkdownAndCSVExports(t *testing.T) {
	issues := []Issue{
		{
			Severity:    SeverityCritical,
			State:       StateNew,
			URL:         "=HYPERLINK(\"https://docs.example.test/missing|pipe\")",
			Problem:     "http_status",
			StatusCode:  404,
			FirstSource: "https://docs.example.test/",
			SourcePages: 2,
			Snippets:    []string{"=cmd, link"},
		},
	}

	markdown := Markdown(issues)
	if !strings.Contains(markdown, "https://docs.example.test/missing\\|pipe") {
		t.Fatalf("Markdown() = %s; want escaped pipe", markdown)
	}
	if !strings.Contains(markdown, "| critical | new |") {
		t.Fatalf("Markdown() = %s; want severity/state row", markdown)
	}

	csv := CSV(issues)
	if !strings.Contains(csv, `"'=cmd, link"`) {
		t.Fatalf("CSV() = %s; want quoted comma cell", csv)
	}
	if !strings.Contains(csv, `"'=HYPERLINK(""https://docs.example.test/missing|pipe"")"`) {
		t.Fatalf("CSV() = %s; want formula URL escaped", csv)
	}
	if !strings.Contains(csv, "severity,state,url,problem,status,first_source,source_count,snippets") {
		t.Fatalf("CSV() = %s; want header", csv)
	}
}

func hasGroup(groups []Group, key string) bool {
	for _, group := range groups {
		if group.Key == key {
			return true
		}
	}
	return false
}
