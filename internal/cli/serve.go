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
	reportPath string
	addr       string
}

func ParseServeArgs(args []string) (serveOptions, error) {
	const cmd = "serve"
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var opts serveOptions
	fs.StringVar(&opts.addr, "addr", "127.0.0.1:0", "local listen address")

	orderedArgs, err := interspersePositionals(fs, args)
	if err != nil {
		return opts, fmt.Errorf("%s: %w", cmd, err)
	}
	if err := fs.Parse(orderedArgs); err != nil {
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

	reportData, err := report.Read(opts.reportPath)
	if err != nil {
		return err
	}

	handler, err := ui.NewHandler(reportData)
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
	fmt.Fprintf(os.Stdout, "Serving araneae report at %s\n", uiURL+"/")

	server := &http.Server{Handler: handler}
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}
