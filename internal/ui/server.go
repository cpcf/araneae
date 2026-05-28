package ui

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"

	"github.com/cpcf/araneae/internal/report"
)

//go:embed static/*
var staticFS embed.FS

func NewHandler(reportData report.Report) (http.Handler, error) {
	subFS, err := fs.Sub(staticFS, "static")
	if err != nil {
		return nil, fmt.Errorf("resolve ui assets: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/report", reportEndpoint(reportData))
	mux.Handle("/", http.FileServer(http.FS(subFS)))
	return mux, nil
}

func reportEndpoint(reportData report.Report) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		enc := json.NewEncoder(w)
		if err := enc.Encode(reportData); err != nil {
			http.Error(w, "failed to encode report", http.StatusInternalServerError)
		}
	}
}

func buildServeURL(host, port string) string {
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port)
}

func ServeURL(listenAddress net.Addr) (string, error) {
	host, port, err := net.SplitHostPort(listenAddress.String())
	if err != nil {
		return "", fmt.Errorf("split listen address %q: %w", listenAddress.String(), err)
	}
	return buildServeURL(host, port), nil
}
