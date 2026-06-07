package cli

import (
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"

	"github.com/cpcf/araneae/internal/report"
	"github.com/cpcf/araneae/internal/ui"
)

type serveOptions struct {
	reportPath   string
	addr         string
	baselinePath string
}

func registerServeFlags(fs *flag.FlagSet, opts *serveOptions) {
	fs.StringVar(&opts.addr, "addr", "127.0.0.1:0", "local listen address")
	fs.StringVar(&opts.baselinePath, "baseline", "", "previous JSON report to annotate new and existing triage issues")
}

func ParseServeArgs(args []string) (serveOptions, error) {
	const cmd = "serve"
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var opts serveOptions
	registerServeFlags(fs, &opts)

	orderedArgs, err := interspersePositionals(fs, args)
	if err != nil {
		return opts, fmt.Errorf("%s: %w", cmd, err)
	}
	if err := fs.Parse(orderedArgs); err != nil {
		return opts, fmt.Errorf("%s: %w", cmd, sanitizeFlagParseError(err))
	}
	if fs.NArg() != 1 {
		return opts, fmt.Errorf("%s: expected <report-path>", cmd)
	}
	opts.reportPath = fs.Arg(0)

	return opts, nil
}

func RunServe(args []string) error {
	return runServeCommand(args, os.Stdout)
}

func runServeCommand(args []string, stdout io.Writer) error {
	opts, err := ParseServeArgs(args)
	if err != nil {
		if helpRequested(err) {
			return writeHelp(stdout, serveUsage())
		}
		return err
	}

	reportData, err := report.Read(opts.reportPath)
	if err != nil {
		return err
	}

	var baselineReport *report.Report
	if opts.baselinePath != "" {
		parsed, err := report.Read(opts.baselinePath)
		if err != nil {
			return err
		}
		baselineReport = &parsed
	}

	handler, err := ui.NewHandlerWithTriage(reportData, baselineReport)
	if err != nil {
		return err
	}

	listener, err := net.Listen("tcp", opts.addr)
	if err != nil {
		return err
	}
	defer listener.Close()

	uiURL, err := ui.ServeURL(listener.Addr())
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Serving araneae report at %s\n", uiURL+"/")

	server := &http.Server{Handler: handler}
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func serveUsage() string {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var opts serveOptions
	registerServeFlags(fs, &opts)
	return flagUsage("serve", "<report-path>", fs)
}
