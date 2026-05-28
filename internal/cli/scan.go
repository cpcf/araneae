package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"
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

	if err := fs.Parse(args); err != nil {
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

	return fmt.Errorf("%w: entry-url=%q out=%q max-pages=%d timeout=%s concurrency=%d path-prefix=%q allow-hosts=%v user-agent=%q fail-on-dead=%t fail-on-non-200=%t",
		errScanNotImplemented,
		opts.entryURL,
		opts.out,
		opts.maxPages,
		opts.timeout,
		opts.concurrency,
		opts.pathPrefix,
		opts.allowHosts,
		opts.userAgent,
		opts.failOnDead,
		opts.failOnNon200,
	)
}

var errScanNotImplemented = errors.New("scan is not implemented yet")
