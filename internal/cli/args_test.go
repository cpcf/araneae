package cli

import (
	"strings"
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
		"--max-requests-per-second", "4.5",
		"--max-response-bytes", "123456",
		"--retries", "2",
		"--retry-backoff", "750ms",
		"--header", "Authorization: Bearer token",
		"--header", "X-Preview: true",
		"--allow-host", "https://www.example.com",
		"--path-prefix", "/docs/",
		"--local-root", "public",
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
	if opts.maxReqPerSec != 4.5 {
		t.Fatalf("maxReqPerSec = %f", opts.maxReqPerSec)
	}
	if opts.maxResponseBytes != 123456 {
		t.Fatalf("maxResponseBytes = %d", opts.maxResponseBytes)
	}
	if opts.retries != 2 {
		t.Fatalf("retries = %d", opts.retries)
	}
	if opts.retryBackoff != 750*time.Millisecond {
		t.Fatalf("retryBackoff = %s", opts.retryBackoff)
	}
	if len(opts.headers) != 2 {
		t.Fatalf("headers = %#v; want 2", opts.headers)
	}
	if opts.headers[0] != (requestHeader{Name: "Authorization", Value: "Bearer token"}) {
		t.Fatalf("first header = %#v", opts.headers[0])
	}
	if opts.headers[1] != (requestHeader{Name: "X-Preview", Value: "true"}) {
		t.Fatalf("second header = %#v", opts.headers[1])
	}
	if len(opts.allowHosts) != 1 || opts.allowHosts[0] != "https://www.example.com" {
		t.Fatalf("allowHosts = %#v", opts.allowHosts)
	}
	if opts.pathPrefix != "/docs/" {
		t.Fatalf("pathPrefix = %q", opts.pathPrefix)
	}
	if opts.localRoot != "public" {
		t.Fatalf("localRoot = %q", opts.localRoot)
	}
	if opts.userAgent != "custom-agent" {
		t.Fatalf("userAgent = %q", opts.userAgent)
	}
	if !opts.failOnDead || !opts.failOnNon200 {
		t.Fatalf("fail flags = %t %t", opts.failOnDead, opts.failOnNon200)
	}
}

func TestParseScanArgsDefaultsMaxResponseBytes(t *testing.T) {
	opts, err := ParseScanArgs([]string{"https://docs.example.com/"})
	if err != nil {
		t.Fatalf("ParseScanArgs() error = %v", err)
	}

	if opts.maxResponseBytes != 5*1024*1024 {
		t.Fatalf("maxResponseBytes = %d; want 5242880", opts.maxResponseBytes)
	}
	if opts.retries != 0 {
		t.Fatalf("retries = %d; want 0", opts.retries)
	}
	if opts.retryBackoff != 500*time.Millisecond {
		t.Fatalf("retryBackoff = %s; want 500ms", opts.retryBackoff)
	}
}

func TestParseScanArgsAcceptsUnlimitedMaxResponseBytes(t *testing.T) {
	opts, err := ParseScanArgs([]string{
		"https://docs.example.com/",
		"--max-response-bytes", "0",
	})
	if err != nil {
		t.Fatalf("ParseScanArgs() error = %v", err)
	}

	if opts.maxResponseBytes != 0 {
		t.Fatalf("maxResponseBytes = %d; want 0", opts.maxResponseBytes)
	}
}

func TestParseScanArgsRejectsNegativeMaxResponseBytes(t *testing.T) {
	_, err := ParseScanArgs([]string{
		"https://docs.example.com/",
		"--max-response-bytes", "-1",
	})
	if err == nil {
		t.Fatal("ParseScanArgs() error = nil; want error")
	}
}

func TestParseScanArgsRejectsNegativeRetries(t *testing.T) {
	_, err := ParseScanArgs([]string{
		"https://docs.example.com/",
		"--retries", "-1",
	})
	if err == nil {
		t.Fatal("ParseScanArgs() error = nil; want error")
	}
}

func TestParseScanArgsRejectsTooManyRetries(t *testing.T) {
	_, err := ParseScanArgs([]string{
		"https://docs.example.com/",
		"--retries", "101",
	})
	if err == nil {
		t.Fatal("ParseScanArgs() error = nil; want error")
	}
}

func TestParseScanArgsRejectsNegativeRetryBackoff(t *testing.T) {
	_, err := ParseScanArgs([]string{
		"https://docs.example.com/",
		"--retry-backoff", "-1s",
	})
	if err == nil {
		t.Fatal("ParseScanArgs() error = nil; want error")
	}
}

func TestParseScanArgsRejectsMalformedHeader(t *testing.T) {
	secret := "super-secret-token"
	_, err := ParseScanArgs([]string{
		"https://docs.example.com/",
		"--header", "Authorization Bearer " + secret,
	})
	if err == nil {
		t.Fatal("ParseScanArgs() error = nil; want error")
	}
	if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "Authorization") {
		t.Fatalf("error %q leaks header input", err)
	}
}

func TestParseScanArgsRejectsSplitHeaderWithoutLeakingValue(t *testing.T) {
	secret := "fake-secret-token"
	_, err := ParseScanArgs([]string{
		"https://docs.example.com/",
		"--header", "Authorization:", "--" + secret,
	})
	if err == nil {
		t.Fatal("ParseScanArgs() error = nil; want error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error %q leaks split header value", err)
	}
}

func TestParseScanArgsRejectsEmptyHeaderName(t *testing.T) {
	_, err := ParseScanArgs([]string{
		"https://docs.example.com/",
		"--header", ": value",
	})
	if err == nil {
		t.Fatal("ParseScanArgs() error = nil; want error")
	}
}

func TestParseScanArgsRejectsHeaderNewlines(t *testing.T) {
	tests := []struct {
		name   string
		header string
	}{
		{name: "name newline", header: "Bad\nName: value"},
		{name: "value newline", header: "X-Test: first\nsecond"},
		{name: "value carriage return", header: "X-Test: first\rsecond"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseScanArgs([]string{
				"https://docs.example.com/",
				"--header", tt.header,
			})
			if err == nil {
				t.Fatal("ParseScanArgs() error = nil; want error")
			}
		})
	}
}

func TestParseScanArgsRejectsInvalidHeaderSyntax(t *testing.T) {
	tests := []struct {
		name   string
		header string
	}{
		{name: "space in name", header: "Bad Name: value"},
		{name: "invalid symbol in name", header: "X-Test@: value"},
		{name: "unicode name", header: "X-Cafe\u00e9: value"},
		{name: "control value", header: "X-Test: value\x01"},
		{name: "delete value", header: "X-Test: value\x7f"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseScanArgs([]string{
				"https://docs.example.com/",
				"--header", tt.header,
			})
			if err == nil {
				t.Fatal("ParseScanArgs() error = nil; want error")
			}
		})
	}
}

func TestParseScanArgsRejectsNegativeRateLimit(t *testing.T) {
	_, err := ParseScanArgs([]string{
		"https://docs.example.com/",
		"--max-requests-per-second", "-1",
	})
	if err == nil {
		t.Fatal("ParseScanArgs() error = nil; want error")
	}
}

func TestParseScanArgsRejectsNonFiniteRateLimit(t *testing.T) {
	_, err := ParseScanArgs([]string{
		"https://docs.example.com/",
		"--max-requests-per-second", "NaN",
	})
	if err == nil {
		t.Fatal("ParseScanArgs() error = nil; want error")
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
