package check

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cpcf/araneae/internal/report"
)

const defaultFailureLimit = 10

type Options struct {
	FailOnDead      bool
	FailOnNon200    bool
	FailOnTruncated bool
}

type Summary struct {
	LinksDiscovered  int
	LinkOccurrences  int
	FetchesAttempted int
	OKLinks          int
	DeadLinks        int
	Non200Links      int
	SkippedLinks     int
	SkippedExternal  int
	Truncated        bool
	UnvisitedURLs    int
}

type Failure struct {
	URL         string
	FetchURL    string
	Problem     string
	StatusCode  int
	Count       int
	SourcePages int
	Dead        bool
	Non200      bool
}

type Result struct {
	Policy          Options
	Summary         Summary
	Failures        []Failure
	TruncatedFailed bool
}

func Evaluate(reportData report.Report, opts Options) Result {
	result := Result{
		Policy: opts,
		Summary: Summary{
			LinksDiscovered:  reportData.Summary.LinksDiscovered,
			LinkOccurrences:  reportData.Summary.LinkOccurrences,
			FetchesAttempted: reportData.Summary.FetchesAttempted,
			OKLinks:          reportData.Summary.OKLinks,
			DeadLinks:        reportData.Summary.DeadLinks,
			Non200Links:      reportData.Summary.Non200Links,
			SkippedLinks:     reportData.Summary.SkippedLinks,
			SkippedExternal:  reportData.Summary.SkippedExternal,
			Truncated:        reportData.Summary.Truncated,
			UnvisitedURLs:    reportData.Summary.UnvisitedURLs,
		},
		TruncatedFailed: opts.FailOnTruncated && reportData.Summary.Truncated,
	}

	for _, link := range reportData.Links {
		if !((opts.FailOnDead && link.Dead) || (opts.FailOnNon200 && link.Non200)) {
			continue
		}
		result.Failures = append(result.Failures, Failure{
			URL:         link.URL,
			FetchURL:    link.FetchURL,
			Problem:     link.Problem,
			StatusCode:  link.StatusCode,
			Count:       link.Count,
			SourcePages: len(link.Sources),
			Dead:        link.Dead,
			Non200:      link.Non200,
		})
	}
	sortFailures(result.Failures)
	return result
}

func (r Result) Failed() bool {
	return r.TruncatedFailed || len(r.Failures) > 0
}

func (r Result) Err() error {
	if !r.Failed() {
		return nil
	}

	parts := make([]string, 0, 3)
	if r.Policy.FailOnDead && r.Summary.DeadLinks > 0 {
		parts = append(parts, fmt.Sprintf("dead_links=%d", r.Summary.DeadLinks))
	}
	if r.Policy.FailOnNon200 && r.Summary.Non200Links > 0 {
		parts = append(parts, fmt.Sprintf("non_200_links=%d", r.Summary.Non200Links))
	}
	if r.TruncatedFailed {
		parts = append(parts, fmt.Sprintf("truncated=true unvisited_urls=%d", r.Summary.UnvisitedURLs))
	}
	if len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("failures=%d", len(r.Failures)))
	}
	return fmt.Errorf("check failed: %s", strings.Join(parts, " "))
}

func TextSummary(result Result) string {
	status := "pass"
	if result.Failed() {
		status = "fail"
	}
	return fmt.Sprintf(
		"status=%s links=%d occurrences=%d fetches=%d ok=%d dead=%d non_200=%d skipped=%d skipped_external=%d truncated=%t unvisited=%d failures=%d\n",
		status,
		result.Summary.LinksDiscovered,
		result.Summary.LinkOccurrences,
		result.Summary.FetchesAttempted,
		result.Summary.OKLinks,
		result.Summary.DeadLinks,
		result.Summary.Non200Links,
		result.Summary.SkippedLinks,
		result.Summary.SkippedExternal,
		result.Summary.Truncated,
		result.Summary.UnvisitedURLs,
		len(result.Failures),
	)
}

func MarkdownSummary(result Result) string {
	var b strings.Builder
	status := "PASS"
	if result.Failed() {
		status = "FAIL"
	}

	fmt.Fprintf(&b, "## Araneae Check Summary\n\n")
	fmt.Fprintf(&b, "**Status:** %s\n\n", status)
	fmt.Fprintf(&b, "| Metric | Count |\n")
	fmt.Fprintf(&b, "| --- | ---: |\n")
	fmt.Fprintf(&b, "| Links discovered | %d |\n", result.Summary.LinksDiscovered)
	fmt.Fprintf(&b, "| Link occurrences | %d |\n", result.Summary.LinkOccurrences)
	fmt.Fprintf(&b, "| Fetches attempted | %d |\n", result.Summary.FetchesAttempted)
	fmt.Fprintf(&b, "| OK links | %d |\n", result.Summary.OKLinks)
	fmt.Fprintf(&b, "| Dead links | %d |\n", result.Summary.DeadLinks)
	fmt.Fprintf(&b, "| Non-200 links | %d |\n", result.Summary.Non200Links)
	fmt.Fprintf(&b, "| Skipped links | %d |\n", result.Summary.SkippedLinks)
	fmt.Fprintf(&b, "| Unvisited URLs | %d |\n", result.Summary.UnvisitedURLs)
	if result.Summary.Truncated {
		fmt.Fprintf(&b, "\nReport was truncated before all queued URLs were visited.\n")
	}

	fmt.Fprintf(&b, "\n### Top Problems\n\n")
	if len(result.Failures) == 0 {
		fmt.Fprintf(&b, "No failing links.\n")
		return b.String()
	}

	fmt.Fprintf(&b, "| URL | Problem | Status | Sources |\n")
	fmt.Fprintf(&b, "| --- | --- | ---: | ---: |\n")
	for _, failure := range topFailures(result.Failures, defaultFailureLimit) {
		fmt.Fprintf(
			&b,
			"| %s | %s | %s | %d |\n",
			escapeMarkdownCell(failure.URL),
			escapeMarkdownCell(failureProblem(failure)),
			failureStatus(failure),
			failure.SourcePages,
		)
	}
	return b.String()
}

func topFailures(failures []Failure, limit int) []Failure {
	if limit <= 0 || len(failures) <= limit {
		return failures
	}
	return failures[:limit]
}

func sortFailures(failures []Failure) {
	sort.Slice(failures, func(i, j int) bool {
		if failures[i].Problem != failures[j].Problem {
			return failures[i].Problem < failures[j].Problem
		}
		return failures[i].URL < failures[j].URL
	})
}

func failureProblem(failure Failure) string {
	if failure.Problem != "" {
		return failure.Problem
	}
	switch {
	case failure.Dead:
		return "dead"
	case failure.Non200:
		return "http_status"
	default:
		return "failed"
	}
}

func failureStatus(failure Failure) string {
	if failure.StatusCode == 0 {
		return "-"
	}
	return fmt.Sprintf("%d", failure.StatusCode)
}

func escapeMarkdownCell(value string) string {
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "|", `\|`)
	return value
}
