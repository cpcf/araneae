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

func TestTriageEndpoint(t *testing.T) {
	expected := report.Report{
		SchemaVersion: 1,
		Links: []report.LinkResult{
			{
				URL:        "https://example.com/missing",
				FetchURL:   "https://example.com/missing",
				Count:      2,
				Dead:       true,
				Non200:     true,
				StatusCode: 404,
				Problem:    "http_status",
				Sources: []report.ReportSource{
					{PageURL: "https://example.com/", Count: 2, Texts: []string{"Missing"}},
				},
			},
		},
	}

	handler, err := NewHandler(expected)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/triage", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if got, want := w.Result().StatusCode, http.StatusOK; got != want {
		t.Fatalf("status = %d; want %d", got, want)
	}

	var decoded struct {
		Summary struct {
			Total    int `json:"total"`
			Critical int `json:"critical"`
		} `json:"summary"`
		Issues []struct {
			Severity string `json:"severity"`
			Problem  string `json:"problem"`
		} `json:"issues"`
	}
	if err := json.NewDecoder(w.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode response = %v", err)
	}
	if decoded.Summary.Total != 1 || decoded.Summary.Critical != 1 {
		t.Fatalf("triage summary = %#v; want one critical issue", decoded.Summary)
	}
	if len(decoded.Issues) != 1 || decoded.Issues[0].Severity != "critical" || decoded.Issues[0].Problem != "http_status" {
		t.Fatalf("triage issues = %#v", decoded.Issues)
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
