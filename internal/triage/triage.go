package triage

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/cpcf/araneae/internal/baseline"
	"github.com/cpcf/araneae/internal/report"
)

type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityWarning  Severity = "warning"
	SeverityInfo     Severity = "info"
)

type IssueState string

const (
	StateUnknown  IssueState = "unknown"
	StateNew      IssueState = "new"
	StateExisting IssueState = "existing"
)

type Issue struct {
	Fingerprint string            `json:"fingerprint"`
	Severity    Severity          `json:"severity"`
	State       IssueState        `json:"state"`
	URL         string            `json:"url"`
	FetchURL    string            `json:"fetch_url"`
	TargetHost  string            `json:"target_host"`
	Problem     string            `json:"problem"`
	StatusCode  int               `json:"status_code"`
	Count       int               `json:"count"`
	SourcePages int               `json:"source_pages"`
	FirstSource string            `json:"first_source"`
	Snippets    []string          `json:"snippets"`
	Dead        bool              `json:"dead"`
	Non200      bool              `json:"non_200"`
	OK          bool              `json:"ok"`
	Link        report.LinkResult `json:"link"`
}

type Summary struct {
	Total    int `json:"total"`
	Critical int `json:"critical"`
	Warning  int `json:"warning"`
	Info     int `json:"info"`
	New      int `json:"new"`
	Existing int `json:"existing"`
	Unknown  int `json:"unknown"`
}

type Payload struct {
	Summary       Summary `json:"summary"`
	Issues        []Issue `json:"issues"`
	ProblemGroups []Group `json:"problem_groups"`
	SourceGroups  []Group `json:"source_groups"`
	HostGroups    []Group `json:"host_groups"`
	StateGroups   []Group `json:"state_groups"`
}

type Group struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Count    int    `json:"count"`
	Critical int    `json:"critical"`
	Warning  int    `json:"warning"`
	Info     int    `json:"info"`
}

type Filter struct {
	Problem    string
	Severity   Severity
	TargetHost string
	SourcePage string
	State      IssueState
	Query      string
}

func BuildPayload(reportData report.Report, comparison *baseline.Comparison) Payload {
	issues := Issues(reportData, comparison)
	return Payload{
		Summary:       Summarize(issues),
		Issues:        issues,
		ProblemGroups: GroupByProblem(issues),
		SourceGroups:  GroupBySourcePage(issues),
		HostGroups:    GroupByTargetHost(issues),
		StateGroups:   GroupByState(issues),
	}
}

func Issues(reportData report.Report, comparison *baseline.Comparison) []Issue {
	stateByFingerprint := comparisonStates(comparison)
	issues := make([]Issue, 0)
	for _, link := range reportData.Links {
		if !(link.Dead || link.Non200 || isRedirect(link)) {
			continue
		}
		problem := ProblemForLink(link)
		fingerprint := Fingerprint(link.URL, problem)
		issue := Issue{
			Fingerprint: fingerprint,
			Severity:    SeverityForLink(link),
			State:       stateByFingerprint[fingerprint],
			URL:         link.URL,
			FetchURL:    link.FetchURL,
			TargetHost:  targetHost(link),
			Problem:     problem,
			StatusCode:  link.StatusCode,
			Count:       link.Count,
			SourcePages: len(link.Sources),
			FirstSource: firstSource(link.Sources),
			Snippets:    firstSnippets(link.Sources),
			Dead:        link.Dead,
			Non200:      link.Non200,
			OK:          link.OK,
			Link:        link,
		}
		if issue.State == "" {
			issue.State = StateUnknown
		}
		issues = append(issues, issue)
	}
	SortIssues(issues)
	return issues
}

func ProblemForLink(link report.LinkResult) string {
	if link.Problem != "" {
		return link.Problem
	}
	switch {
	case isRedirect(link):
		return "redirect"
	case link.Dead:
		return "dead"
	case link.Non200:
		return "http_status"
	default:
		return "unknown"
	}
}

func SeverityForLink(link report.LinkResult) Severity {
	if link.Problem == "missing_fragment" {
		return SeverityCritical
	}
	if link.StatusCode == 404 || link.StatusCode == 410 {
		return SeverityCritical
	}
	if link.Dead {
		return SeverityCritical
	}
	if link.Non200 {
		return SeverityWarning
	}
	if isRedirect(link) {
		return SeverityInfo
	}
	return SeverityInfo
}

func Fingerprint(rawURL, problem string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(rawURL) + "\x00" + strings.TrimSpace(problem)))
	return hex.EncodeToString(sum[:16])
}

func Summarize(issues []Issue) Summary {
	var summary Summary
	summary.Total = len(issues)
	for _, issue := range issues {
		switch issue.Severity {
		case SeverityCritical:
			summary.Critical++
		case SeverityWarning:
			summary.Warning++
		case SeverityInfo:
			summary.Info++
		}
		switch issue.State {
		case StateNew:
			summary.New++
		case StateExisting:
			summary.Existing++
		default:
			summary.Unknown++
		}
	}
	return summary
}

func FilterIssues(issues []Issue, filter Filter, acknowledged map[string]bool) []Issue {
	query := strings.ToLower(strings.TrimSpace(filter.Query))
	filtered := make([]Issue, 0, len(issues))
	for _, issue := range issues {
		if acknowledged != nil && acknowledged[issue.Fingerprint] {
			continue
		}
		if filter.Problem != "" && issue.Problem != filter.Problem {
			continue
		}
		if filter.Severity != "" && issue.Severity != filter.Severity {
			continue
		}
		if filter.TargetHost != "" && issue.TargetHost != filter.TargetHost {
			continue
		}
		if filter.SourcePage != "" && !issueHasSource(issue, filter.SourcePage) {
			continue
		}
		if filter.State != "" && issue.State != filter.State {
			continue
		}
		if query != "" && !issueMatchesQuery(issue, query) {
			continue
		}
		filtered = append(filtered, issue)
	}
	SortIssues(filtered)
	return filtered
}

func GroupByProblem(issues []Issue) []Group {
	return groupIssues(issues, func(issue Issue) (string, string) {
		return issue.Problem, issue.Problem
	})
}

func GroupBySourcePage(issues []Issue) []Group {
	bySource := map[string]*Group{}
	for _, issue := range issues {
		sources := issue.Link.Sources
		if len(sources) == 0 && issue.FirstSource != "" {
			sources = []report.ReportSource{{PageURL: issue.FirstSource}}
		}
		if len(sources) == 0 {
			sources = []report.ReportSource{{PageURL: "(unknown source)"}}
		}
		seen := map[string]struct{}{}
		for _, source := range sources {
			key := source.PageURL
			if key == "" {
				key = "(unknown source)"
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			group, ok := bySource[key]
			if !ok {
				group = &Group{Key: key, Label: key}
				bySource[key] = group
			}
			group.Count++
			switch issue.Severity {
			case SeverityCritical:
				group.Critical++
			case SeverityWarning:
				group.Warning++
			case SeverityInfo:
				group.Info++
			}
		}
	}
	groups := make([]Group, 0, len(bySource))
	for _, group := range bySource {
		groups = append(groups, *group)
	}
	sortGroups(groups)
	return groups
}

func GroupByTargetHost(issues []Issue) []Group {
	return groupIssues(issues, func(issue Issue) (string, string) {
		host := issue.TargetHost
		if host == "" {
			host = "(unknown host)"
		}
		return host, host
	})
}

func GroupByState(issues []Issue) []Group {
	return groupIssues(issues, func(issue Issue) (string, string) {
		state := string(issue.State)
		if state == "" {
			state = string(StateUnknown)
		}
		return state, state
	})
}

func SortIssues(issues []Issue) {
	sort.Slice(issues, func(i, j int) bool {
		if severityRank(issues[i].Severity) != severityRank(issues[j].Severity) {
			return severityRank(issues[i].Severity) < severityRank(issues[j].Severity)
		}
		if issues[i].State != issues[j].State {
			return stateRank(issues[i].State) < stateRank(issues[j].State)
		}
		if issues[i].Problem != issues[j].Problem {
			return issues[i].Problem < issues[j].Problem
		}
		return issues[i].URL < issues[j].URL
	})
}

func Markdown(issues []Issue) string {
	var b strings.Builder
	fmt.Fprintf(&b, "| Severity | State | URL | Problem | Status | First source | Sources | Snippets |\n")
	fmt.Fprintf(&b, "| --- | --- | --- | --- | ---: | --- | ---: | --- |\n")
	for _, issue := range issues {
		fmt.Fprintf(
			&b,
			"| %s | %s | %s | %s | %s | %s | %d | %s |\n",
			escapeMarkdown(string(issue.Severity)),
			escapeMarkdown(string(issue.State)),
			escapeMarkdown(issue.URL),
			escapeMarkdown(issue.Problem),
			statusText(issue.StatusCode),
			escapeMarkdown(issue.FirstSource),
			issue.SourcePages,
			escapeMarkdown(strings.Join(issue.Snippets, " / ")),
		)
	}
	return b.String()
}

func CSV(issues []Issue) string {
	rows := [][]string{{"severity", "state", "url", "problem", "status", "first_source", "source_count", "snippets"}}
	for _, issue := range issues {
		rows = append(rows, []string{
			string(issue.Severity),
			string(issue.State),
			issue.URL,
			issue.Problem,
			statusText(issue.StatusCode),
			issue.FirstSource,
			fmt.Sprintf("%d", issue.SourcePages),
			strings.Join(issue.Snippets, " / "),
		})
	}

	var b strings.Builder
	for _, row := range rows {
		for i, value := range row {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(csvCell(value))
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func JSON(payload Payload) ([]byte, error) {
	return json.MarshalIndent(payload, "", "  ")
}

func comparisonStates(comparison *baseline.Comparison) map[string]IssueState {
	states := map[string]IssueState{}
	if comparison == nil {
		return states
	}
	for _, issue := range comparison.New {
		states[Fingerprint(issue.URL, issue.Problem)] = StateNew
	}
	for _, issue := range comparison.Existing {
		states[Fingerprint(issue.URL, issue.Problem)] = StateExisting
	}
	return states
}

func groupIssues(issues []Issue, key func(Issue) (string, string)) []Group {
	byKey := map[string]*Group{}
	for _, issue := range issues {
		k, label := key(issue)
		group, ok := byKey[k]
		if !ok {
			group = &Group{Key: k, Label: label}
			byKey[k] = group
		}
		group.Count++
		switch issue.Severity {
		case SeverityCritical:
			group.Critical++
		case SeverityWarning:
			group.Warning++
		case SeverityInfo:
			group.Info++
		}
	}
	groups := make([]Group, 0, len(byKey))
	for _, group := range byKey {
		groups = append(groups, *group)
	}
	sort.Slice(groups, func(i, j int) bool {
		return groupLess(groups[i], groups[j])
	})
	return groups
}

func sortGroups(groups []Group) {
	sort.Slice(groups, func(i, j int) bool {
		return groupLess(groups[i], groups[j])
	})
}

func groupLess(a, b Group) bool {
	if a.Critical != b.Critical {
		return a.Critical > b.Critical
	}
	if a.Count != b.Count {
		return a.Count > b.Count
	}
	return a.Key < b.Key
}

func firstSource(sources []report.ReportSource) string {
	if len(sources) == 0 {
		return ""
	}
	return sources[0].PageURL
}

func firstSnippets(sources []report.ReportSource) []string {
	snippets := make([]string, 0, 3)
	for _, source := range sources {
		for _, text := range source.Texts {
			if text == "" {
				continue
			}
			snippets = append(snippets, text)
			if len(snippets) == 3 {
				return snippets
			}
		}
	}
	return snippets
}

func targetHost(link report.LinkResult) string {
	raw := link.FetchURL
	if raw == "" {
		raw = link.URL
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return parsed.Host
}

func issueHasSource(issue Issue, pageURL string) bool {
	if issue.FirstSource == pageURL {
		return true
	}
	for _, source := range issue.Link.Sources {
		if source.PageURL == pageURL {
			return true
		}
	}
	return false
}

func isRedirect(link report.LinkResult) bool {
	return link.FinalURL != "" && link.FetchURL != "" && link.FinalURL != link.FetchURL
}

func issueMatchesQuery(issue Issue, query string) bool {
	values := []string{issue.URL, issue.FetchURL, issue.FirstSource, issue.Problem, issue.TargetHost}
	values = append(values, issue.Snippets...)
	for _, value := range values {
		if strings.Contains(strings.ToLower(value), query) {
			return true
		}
	}
	return false
}

func severityRank(severity Severity) int {
	switch severity {
	case SeverityCritical:
		return 0
	case SeverityWarning:
		return 1
	case SeverityInfo:
		return 2
	default:
		return 3
	}
}

func stateRank(state IssueState) int {
	switch state {
	case StateNew:
		return 0
	case StateExisting:
		return 1
	default:
		return 2
	}
}

func statusText(statusCode int) string {
	if statusCode == 0 {
		return ""
	}
	return fmt.Sprintf("%d", statusCode)
}

func escapeMarkdown(value string) string {
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "|", `\|`)
	return value
}

func csvCell(value string) string {
	value = safeCSVValue(value)
	if !strings.ContainsAny(value, "\",\n\r") {
		return value
	}
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func safeCSVValue(value string) string {
	trimmed := strings.TrimLeft(value, " \t\r\n")
	if trimmed == "" {
		return value
	}
	switch trimmed[0] {
	case '=', '+', '-', '@':
		return "'" + value
	default:
		return value
	}
}
