package report

import "time"

const SchemaVersion = 1

type Report struct {
	SchemaVersion int           `json:"schema_version"`
	GeneratedAt   time.Time     `json:"generated_at"`
	EntryURL      string        `json:"entry_url"`
	Scope         ReportScope   `json:"scope"`
	Limits        ReportLimits  `json:"limits"`
	Summary       ReportSummary `json:"summary"`
	Links         []LinkResult  `json:"links"`
	Fetches       []FetchResult `json:"fetches"`
	SkippedLinks  []SkippedLink `json:"skipped_links"`
}

type ReportScope struct {
	Origin         string   `json:"origin"`
	AllowedOrigins []string `json:"allowed_origins"`
	Policy         string   `json:"same_site_policy"`
	PathPrefix     string   `json:"path_prefix"`
}

type ReportLimits struct {
	MaxPages          int `json:"max_pages"`
	RequestTimeoutSec int `json:"request_timeout_seconds"`
	MaxConcurrency    int `json:"max_concurrency"`
}

type ReportSummary struct {
	LinksDiscovered  int  `json:"links_discovered"`
	LinkOccurrences  int  `json:"link_occurrences"`
	FetchesAttempted int  `json:"fetches_attempted"`
	OKLinks          int  `json:"ok_links"`
	DeadLinks        int  `json:"dead_links"`
	Non200Links      int  `json:"non_200_links"`
	SkippedLinks     int  `json:"skipped_links"`
	SkippedExternal  int  `json:"skipped_external_links"`
	Truncated        bool `json:"truncated"`
	UnvisitedURLs    int  `json:"unvisited_urls"`
}

type ReportSource struct {
	PageURL string   `json:"page_url"`
	Count   int      `json:"count"`
	Texts   []string `json:"texts"`
}

type LinkResult struct {
	URL         string         `json:"url"`
	FetchURL    string         `json:"fetch_url"`
	Count       int            `json:"count"`
	OK          bool           `json:"ok"`
	Dead        bool           `json:"dead"`
	Non200      bool           `json:"non_200"`
	Problem     string         `json:"problem"`
	StatusCode  int            `json:"status_code"`
	FinalURL    string         `json:"final_url"`
	ContentType string         `json:"content_type"`
	Error       string         `json:"error"`
	Sources     []ReportSource `json:"sources"`
}

type FetchResult struct {
	URL           string    `json:"url"`
	StatusCode    int       `json:"status_code"`
	FinalURL      string    `json:"final_url"`
	ContentType   string    `json:"content_type"`
	Error         string    `json:"error"`
	RedirectChain []string  `json:"redirect_chain"`
	CheckedAt     time.Time `json:"checked_at"`
}

type SkippedLink struct {
	URL     string         `json:"url"`
	Reason  string         `json:"reason"`
	Count   int            `json:"count"`
	Sources []ReportSource `json:"sources"`
}
