package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/cpcf/araneae/internal/baseline"
	checkeval "github.com/cpcf/araneae/internal/check"
	"github.com/cpcf/araneae/internal/report"
)

type checkOptions struct {
	scan              scanOptions
	policy            checkeval.Options
	summaryFormat     string
	ci                bool
	githubStepSummary string
	baselinePath      string
	failOn            string
	comparisonOut     string
}

func ParseCheckArgs(args []string) (checkOptions, error) {
	const cmd = "check"

	var opts checkOptions
	opts.summaryFormat = "text"
	opts.failOn = string(checkeval.FailModeAll)

	scanOpts, err := parseScanCoreArgs(cmd, args, func(fs *flag.FlagSet, _ *scanOptions) {
		fs.BoolVar(&opts.policy.FailOnDead, "fail-on-dead", false, "exit non-zero when dead links exist")
		fs.BoolVar(&opts.policy.FailOnNon200, "fail-on-non-200", false, "exit non-zero when non-200 links exist")
		fs.BoolVar(&opts.policy.FailOnTruncated, "fail-on-truncated", false, "exit non-zero when the scan hits --max-pages before visiting every queued URL")
		fs.StringVar(&opts.summaryFormat, "summary", opts.summaryFormat, "summary format: text or markdown")
		fs.BoolVar(&opts.ci, "ci", false, "enable CI conveniences such as default GitHub step summary output")
		fs.StringVar(&opts.githubStepSummary, "github-step-summary", "", "path to append a GitHub step summary markdown report")
		fs.StringVar(&opts.baselinePath, "baseline", "", "previous JSON report to compare against")
		fs.StringVar(&opts.failOn, "fail-on", opts.failOn, "failure mode for link issues: all or new")
		fs.StringVar(&opts.comparisonOut, "comparison-out", "", "write baseline comparison JSON to this path")
	})
	if err != nil {
		return opts, err
	}
	opts.scan = scanOpts

	switch opts.summaryFormat {
	case "text", "markdown":
	default:
		return opts, fmt.Errorf("%s: --summary must be one of: text, markdown", cmd)
	}
	switch checkeval.FailMode(opts.failOn) {
	case checkeval.FailModeAll, checkeval.FailModeNew:
		opts.policy.FailMode = checkeval.FailMode(opts.failOn)
	default:
		return opts, fmt.Errorf("%s: --fail-on must be one of: all, new", cmd)
	}

	return opts, nil
}

func RunCheck(args []string) error {
	return runCheckCommand(args, os.Stdout, os.Getenv)
}

func runCheckCommand(args []string, stdout io.Writer, getenv func(string) string) error {
	opts, err := ParseCheckArgs(args)
	if err != nil {
		if helpRequested(err) {
			return writeHelp(stdout, checkUsage())
		}
		return err
	}
	return runCheck(opts, stdout, getenv)
}

func runCheck(opts checkOptions, stdout io.Writer, getenv func(string) string) error {
	if err := validateCheckOutputPaths(opts); err != nil {
		return err
	}
	baselineReport, err := readBaselineReport(opts.baselinePath)
	if err != nil {
		return err
	}

	reportData, err := runScan(opts.scan)
	if err != nil {
		return err
	}
	if err := writeReportFile(opts.scan.out, reportData); err != nil {
		return err
	}

	comparison := buildComparison(opts, baselineReport, reportData)
	if comparison != nil {
		opts.policy.Comparison = comparison
		if opts.comparisonOut != "" {
			if err := writeComparisonFile(opts.comparisonOut, *comparison); err != nil {
				return err
			}
		}
	}

	result := checkeval.Evaluate(reportData, opts.policy)
	if _, err := io.WriteString(stdout, summaryOutput(result, opts.summaryFormat)); err != nil {
		return fmt.Errorf("write check summary: %w", err)
	}

	if path := githubStepSummaryPath(opts, getenv); path != "" {
		if err := appendGithubStepSummary(path, checkeval.MarkdownSummary(result)); err != nil {
			return err
		}
	}

	return result.Err()
}

func validateCheckOutputPaths(opts checkOptions) error {
	if opts.baselinePath != "" && samePath(opts.scan.out, opts.baselinePath) {
		return fmt.Errorf("check: --baseline must not be the same path as --out")
	}
	if opts.comparisonOut != "" && samePath(opts.scan.out, opts.comparisonOut) {
		return fmt.Errorf("check: --comparison-out must not be the same path as --out")
	}
	if opts.baselinePath != "" && opts.comparisonOut != "" && samePath(opts.baselinePath, opts.comparisonOut) {
		return fmt.Errorf("check: --comparison-out must not be the same path as --baseline")
	}
	return nil
}

func samePath(a, b string) bool {
	absA, errA := filepath.Abs(a)
	absB, errB := filepath.Abs(b)
	if errA == nil && errB == nil {
		return absA == absB
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

func readBaselineReport(path string) (*report.Report, error) {
	if path == "" {
		return nil, nil
	}
	parsed, err := report.Read(path)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func buildComparison(opts checkOptions, baselineReport *report.Report, reportData report.Report) *baseline.Comparison {
	if opts.baselinePath == "" && opts.comparisonOut == "" && opts.policy.FailMode != checkeval.FailModeNew {
		return nil
	}

	comparison := baseline.Compare(baselineReport, reportData, baseline.Options{
		IncludeDead:   opts.policy.FailOnDead,
		IncludeNon200: opts.policy.FailOnNon200,
	})
	return &comparison
}

func writeComparisonFile(path string, comparison baseline.Comparison) error {
	outputFile, err := os.Create(path)
	if err != nil {
		return err
	}
	defer outputFile.Close()

	return baseline.Write(outputFile, comparison)
}

func summaryOutput(result checkeval.Result, format string) string {
	if format == "markdown" {
		return checkeval.MarkdownSummary(result)
	}
	return checkeval.TextSummary(result)
}

func githubStepSummaryPath(opts checkOptions, getenv func(string) string) string {
	if opts.githubStepSummary != "" {
		return opts.githubStepSummary
	}
	if opts.ci && getenv != nil {
		return getenv("GITHUB_STEP_SUMMARY")
	}
	return ""
}

func appendGithubStepSummary(path, markdown string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open GitHub step summary: %w", err)
	}
	defer file.Close()

	if _, err := io.WriteString(file, markdown); err != nil {
		return fmt.Errorf("write GitHub step summary: %w", err)
	}
	if !strings.HasSuffix(markdown, "\n") {
		if _, err := io.WriteString(file, "\n"); err != nil {
			return fmt.Errorf("write GitHub step summary newline: %w", err)
		}
	}
	return nil
}

func checkUsage() string {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var scan scanOptions
	var allowHosts stringSliceValue
	var rawHeaders stringSliceValue
	var opts checkOptions
	opts.summaryFormat = "text"
	opts.failOn = string(checkeval.FailModeAll)
	registerScanFlags(fs, &scan, &rawHeaders, &allowHosts)
	fs.BoolVar(&opts.policy.FailOnDead, "fail-on-dead", false, "exit non-zero when dead links exist")
	fs.BoolVar(&opts.policy.FailOnNon200, "fail-on-non-200", false, "exit non-zero when non-200 links exist")
	fs.BoolVar(&opts.policy.FailOnTruncated, "fail-on-truncated", false, "exit non-zero when the scan hits --max-pages before visiting every queued URL")
	fs.StringVar(&opts.summaryFormat, "summary", opts.summaryFormat, "summary format: text or markdown")
	fs.BoolVar(&opts.ci, "ci", false, "enable CI conveniences such as default GitHub step summary output")
	fs.StringVar(&opts.githubStepSummary, "github-step-summary", "", "path to append a GitHub step summary markdown report")
	fs.StringVar(&opts.baselinePath, "baseline", "", "previous JSON report to compare against")
	fs.StringVar(&opts.failOn, "fail-on", opts.failOn, "failure mode for link issues: all or new")
	fs.StringVar(&opts.comparisonOut, "comparison-out", "", "write baseline comparison JSON to this path")
	return flagUsage("check", "<entry-url>", fs)
}
