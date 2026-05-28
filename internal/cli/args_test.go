package cli

import (
	"testing"
	"time"
)

func TestParseScanArgsAcceptsFlagsAfterEntryURL(t *testing.T) {
	opts, err := ParseScanArgs([]string{
		"https://docs.example.com/",
		"--out", "report.json",
		"--max-pages", "17",
		"--timeout", "3s",
		"--concurrency", "2",
		"--allow-host", "https://www.example.com",
		"--path-prefix", "/docs/",
		"--user-agent", "custom-agent",
		"--fail-on-dead",
		"--fail-on-non-200",
	})
	if err != nil {
		t.Fatalf("ParseScanArgs() error = %v", err)
	}

	if opts.entryURL != "https://docs.example.com/" {
		t.Fatalf("entryURL = %q", opts.entryURL)
	}
	if opts.out != "report.json" {
		t.Fatalf("out = %q", opts.out)
	}
	if opts.maxPages != 17 {
		t.Fatalf("maxPages = %d", opts.maxPages)
	}
	if opts.timeout != 3*time.Second {
		t.Fatalf("timeout = %s", opts.timeout)
	}
	if opts.concurrency != 2 {
		t.Fatalf("concurrency = %d", opts.concurrency)
	}
	if len(opts.allowHosts) != 1 || opts.allowHosts[0] != "https://www.example.com" {
		t.Fatalf("allowHosts = %#v", opts.allowHosts)
	}
	if opts.pathPrefix != "/docs/" {
		t.Fatalf("pathPrefix = %q", opts.pathPrefix)
	}
	if opts.userAgent != "custom-agent" {
		t.Fatalf("userAgent = %q", opts.userAgent)
	}
	if !opts.failOnDead || !opts.failOnNon200 {
		t.Fatalf("fail flags = %t %t", opts.failOnDead, opts.failOnNon200)
	}
}

func TestParseScanArgsAcceptsFlagsBeforeEntryURL(t *testing.T) {
	opts, err := ParseScanArgs([]string{
		"--out=report.json",
		"--max-pages=17",
		"https://docs.example.com/",
	})
	if err != nil {
		t.Fatalf("ParseScanArgs() error = %v", err)
	}

	if opts.entryURL != "https://docs.example.com/" {
		t.Fatalf("entryURL = %q", opts.entryURL)
	}
	if opts.out != "report.json" {
		t.Fatalf("out = %q", opts.out)
	}
	if opts.maxPages != 17 {
		t.Fatalf("maxPages = %d", opts.maxPages)
	}
}

func TestParseServeArgsAcceptsFlagsAfterReportPath(t *testing.T) {
	opts, err := ParseServeArgs([]string{"report.json", "--addr", "127.0.0.1:9000"})
	if err != nil {
		t.Fatalf("ParseServeArgs() error = %v", err)
	}

	if opts.reportPath != "report.json" {
		t.Fatalf("reportPath = %q", opts.reportPath)
	}
	if opts.addr != "127.0.0.1:9000" {
		t.Fatalf("addr = %q", opts.addr)
	}
}
