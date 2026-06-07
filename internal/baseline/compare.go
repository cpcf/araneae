package baseline

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cpcf/araneae/internal/report"
)

const ComparisonSchemaVersion = 1

type Options struct {
	IncludeDead   bool
	IncludeNon200 bool
}

type IssueKey struct {
	URL     string `json:"url"`
	Problem string `json:"problem"`
}

type Issue struct {
	Key         IssueKey `json:"key"`
	URL         string   `json:"url"`
	FetchURL    string   `json:"fetch_url"`
	Problem     string   `json:"problem"`
	StatusCode  int      `json:"status_code"`
	Count       int      `json:"count"`
	SourcePages int      `json:"source_pages"`
	Dead        bool     `json:"dead"`
	Non200      bool     `json:"non_200"`
}

type OKLink struct {
	URL         string `json:"url"`
	FetchURL    string `json:"fetch_url"`
	StatusCode  int    `json:"status_code"`
	Count       int    `json:"count"`
	SourcePages int    `json:"source_pages"`
}

type Summary struct {
	New         int `json:"new"`
	Existing    int `json:"existing"`
	Resolved    int `json:"resolved"`
	UnchangedOK int `json:"unchanged_ok"`
}

type Comparison struct {
	SchemaVersion int      `json:"schema_version"`
	BaselineURL   string   `json:"baseline_entry_url,omitempty"`
	CurrentURL    string   `json:"current_entry_url"`
	Summary       Summary  `json:"summary"`
	New           []Issue  `json:"new"`
	Existing      []Issue  `json:"existing"`
	Resolved      []Issue  `json:"resolved"`
	UnchangedOK   []OKLink `json:"unchanged_ok"`
}

func Compare(baselineReport *report.Report, current report.Report, opts Options) Comparison {
	comparison := Comparison{
		SchemaVersion: ComparisonSchemaVersion,
		CurrentURL:    current.EntryURL,
	}
	if baselineReport != nil {
		comparison.BaselineURL = baselineReport.EntryURL
	}

	baselineIssues := map[IssueKey]Issue{}
	baselineOK := map[string]OKLink{}
	if baselineReport != nil {
		baselineIssues = issuesByKey(*baselineReport, opts)
		baselineOK = okLinksByURL(*baselineReport)
	}

	currentIssues := issuesByKey(current, opts)
	for key, issue := range currentIssues {
		if _, ok := baselineIssues[key]; ok {
			comparison.Existing = append(comparison.Existing, issue)
			continue
		}
		comparison.New = append(comparison.New, issue)
	}

	for key, issue := range baselineIssues {
		if _, ok := currentIssues[key]; !ok {
			comparison.Resolved = append(comparison.Resolved, issue)
		}
	}

	for _, link := range current.Links {
		if !link.OK {
			continue
		}
		if baselineReport != nil {
			if _, ok := baselineOK[link.URL]; !ok {
				continue
			}
		}
		comparison.UnchangedOK = append(comparison.UnchangedOK, okLinkFromReport(link))
	}

	sortIssues(comparison.New)
	sortIssues(comparison.Existing)
	sortIssues(comparison.Resolved)
	sortOKLinks(comparison.UnchangedOK)
	comparison.Summary = Summary{
		New:         len(comparison.New),
		Existing:    len(comparison.Existing),
		Resolved:    len(comparison.Resolved),
		UnchangedOK: len(comparison.UnchangedOK),
	}
	return comparison
}

func KeyForLink(link report.LinkResult) IssueKey {
	return IssueKey{
		URL:     strings.TrimSpace(link.URL),
		Problem: issueProblem(link),
	}
}

func issuesByKey(reportData report.Report, opts Options) map[IssueKey]Issue {
	issues := map[IssueKey]Issue{}
	for _, link := range reportData.Links {
		if !includeIssue(link, opts) {
			continue
		}
		issue := issueFromReport(link)
		issues[issue.Key] = issue
	}
	return issues
}

func okLinksByURL(reportData report.Report) map[string]OKLink {
	links := map[string]OKLink{}
	for _, link := range reportData.Links {
		if link.OK {
			links[link.URL] = okLinkFromReport(link)
		}
	}
	return links
}

func includeIssue(link report.LinkResult, opts Options) bool {
	return (opts.IncludeDead && link.Dead) || (opts.IncludeNon200 && link.Non200)
}

func issueFromReport(link report.LinkResult) Issue {
	key := KeyForLink(link)
	return Issue{
		Key:         key,
		URL:         link.URL,
		FetchURL:    link.FetchURL,
		Problem:     key.Problem,
		StatusCode:  link.StatusCode,
		Count:       link.Count,
		SourcePages: len(link.Sources),
		Dead:        link.Dead,
		Non200:      link.Non200,
	}
}

func okLinkFromReport(link report.LinkResult) OKLink {
	return OKLink{
		URL:         link.URL,
		FetchURL:    link.FetchURL,
		StatusCode:  link.StatusCode,
		Count:       link.Count,
		SourcePages: len(link.Sources),
	}
}

func issueProblem(link report.LinkResult) string {
	if link.Problem != "" {
		return link.Problem
	}
	switch {
	case link.Dead:
		return "dead"
	case link.Non200:
		return "http_status"
	default:
		return "failed"
	}
}

func sortIssues(issues []Issue) {
	sort.Slice(issues, func(i, j int) bool {
		return issueSortKey(issues[i]) < issueSortKey(issues[j])
	})
}

func sortOKLinks(links []OKLink) {
	sort.Slice(links, func(i, j int) bool {
		return links[i].URL < links[j].URL
	})
}

func issueSortKey(issue Issue) string {
	return fmt.Sprintf("%s\x00%s", issue.Key.Problem, issue.Key.URL)
}
