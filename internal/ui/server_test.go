package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cpcf/araneae/internal/report"
)

func TestReportEndpoint(t *testing.T) {
	t.Helper()

	expected := report.Report{
		SchemaVersion: 1,
		Links: []report.LinkResult{
			{
				URL:        "https://example.com/bad",
				FetchURL:   "https://example.com/bad",
				Count:      2,
				Dead:       true,
				Non200:     true,
				StatusCode: 404,
				Error:      "http_status",
				Sources: []report.ReportSource{
					{
						PageURL: "https://example.com/",
						Count:   2,
					},
				},
			},
		},
	}

	handler, err := NewHandler(expected)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/report", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if got, want := w.Result().StatusCode, http.StatusOK; got != want {
		t.Fatalf("status = %d; want %d", got, want)
	}

	var decoded report.Report
	if err := json.NewDecoder(w.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode response = %v", err)
	}
	if got, want := decoded.SchemaVersion, expected.SchemaVersion; got != want {
		t.Fatalf("schema version = %d; want %d", got, want)
	}
	if got, want := len(decoded.Links), len(expected.Links); got != want {
		t.Fatalf("links = %d; want %d", got, want)
	}
}

func TestIndexRoute(t *testing.T) {
	t.Helper()

	handler, err := NewHandler(report.Report{SchemaVersion: 1})
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if got, want := w.Result().StatusCode, http.StatusOK; got != want {
		t.Fatalf("status = %d; want %d", got, want)
	}
	ct := w.Header().Get("Content-Type")
	if ct == "" {
		t.Fatalf("Content-Type = empty; want text/html")
	}
}
