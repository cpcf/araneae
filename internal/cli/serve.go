package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
)

type serveOptions struct {
	reportPath string
	addr       string
}

func ParseServeArgs(args []string) (serveOptions, error) {
	const cmd = "serve"
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var opts serveOptions
	fs.StringVar(&opts.addr, "addr", "127.0.0.1:0", "local listen address")

	if err := fs.Parse(args); err != nil {
		return opts, fmt.Errorf("%s: %w", cmd, err)
	}
	if fs.NArg() != 1 {
		return opts, fmt.Errorf("%s: expected <report-path>", cmd)
	}
	opts.reportPath = fs.Arg(0)

	return opts, nil
}

func RunServe(args []string) error {
	opts, err := ParseServeArgs(args)
	if err != nil {
		return err
	}
	return fmt.Errorf("%w: report=%q addr=%q",
		errServeNotImplemented,
		opts.reportPath,
		opts.addr,
	)
}

var errServeNotImplemented = errors.New("serve is not implemented yet")
