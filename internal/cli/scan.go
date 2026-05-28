package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/cpcf/araneae/internal/crawl"
	"github.com/cpcf/araneae/internal/report"
)

type scanOptions struct {
	entryURL     string
	out          string
	maxPages     int
	timeout      time.Duration
	concurrency  int
	allowHosts   []string
	pathPrefix   string
	userAgent    string
	failOnDead   bool
	failOnNon200 bool
}

type stringSliceValue []string

func (s *stringSliceValue) String() string {
	return strings.Join(*s, ",")
}

func (s *stringSliceValue) Set(v string) error {
	*s = append(*s, v)
	return nil
}

func ParseScanArgs(args []string) (scanOptions, error) {
	const cmd = "scan"
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var opts scanOptions
	var allowHosts stringSliceValue
	fs.StringVar(&opts.out, "out", "araneae-report.json", "output report path")
	fs.IntVar(&opts.maxPages, "max-pages", 500, "maximum checked same-site fetch URLs")
	fs.DurationVar(&opts.timeout, "timeout", 15*time.Second, "per-request timeout")
	fs.IntVar(&opts.concurrency, "concurrency", 8, "fetch concurrency")
	fs.Var(&allowHosts, "allow-host", "additional exact origins allowed for crawl")
	fs.StringVar(&opts.pathPrefix, "path-prefix", "", "optional path prefix restriction")
	fs.StringVar(&opts.userAgent, "user-agent", "araneae/0.1", "user-agent string")
	fs.BoolVar(&opts.failOnDead, "fail-on-dead", false, "exit non-zero when dead links exist")
	fs.BoolVar(&opts.failOnNon200, "fail-on-non-200", false, "exit non-zero when non-200 links exist")

	orderedArgs, err := interspersePositionals(fs, args)
	if err != nil {
		return opts, fmt.Errorf("%s: %w", cmd, err)
	}
	if err := fs.Parse(orderedArgs); err != nil {
		return opts, fmt.Errorf("%s: %w", cmd, err)
	}

	opts.allowHosts = append(opts.allowHosts, allowHosts...)
	if fs.NArg() != 1 {
		return opts, fmt.Errorf("%s: expected <entry-url>", cmd)
	}
	opts.entryURL = fs.Arg(0)

	parsed, err := url.Parse(opts.entryURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return opts, fmt.Errorf("%s: invalid entry URL %q", cmd, opts.entryURL)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return opts, fmt.Errorf("%s: unsupported entry scheme %q", cmd, parsed.Scheme)
	}

	return opts, nil
}

func RunScan(args []string) error {
	opts, err := ParseScanArgs(args)
	if err != nil {
		return err
	}

	crawler := crawl.ScanOptions{
		EntryURL:    opts.entryURL,
		MaxPages:    opts.maxPages,
		Timeout:     opts.timeout,
		Concurrency: opts.concurrency,
		AllowHosts:  opts.allowHosts,
		PathPrefix:  opts.pathPrefix,
		UserAgent:   opts.userAgent,
	}
	reportData, err := crawl.Run(context.Background(), crawler)
	if err != nil {
		return err
	}

	outputFile, err := os.Create(opts.out)
	if err != nil {
		return err
	}
	defer outputFile.Close()

	if err := report.Write(outputFile, reportData); err != nil {
		return err
	}

	if opts.failOnDead && reportData.Summary.DeadLinks > 0 {
		return fmt.Errorf("scan found dead links: %d", reportData.Summary.DeadLinks)
	}
	if opts.failOnNon200 && reportData.Summary.Non200Links > 0 {
		return fmt.Errorf("scan found non-200 links: %d", reportData.Summary.Non200Links)
	}

	return nil
}
