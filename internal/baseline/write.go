package baseline

import (
	"encoding/json"
	"io"
)

func Write(w io.Writer, comparison Comparison) error {
	comparison = normalizeComparison(comparison)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(comparison)
}

func normalizeComparison(comparison Comparison) Comparison {
	if comparison.New == nil {
		comparison.New = []Issue{}
	}
	if comparison.Existing == nil {
		comparison.Existing = []Issue{}
	}
	if comparison.Resolved == nil {
		comparison.Resolved = []Issue{}
	}
	if comparison.UnchangedOK == nil {
		comparison.UnchangedOK = []OKLink{}
	}
	return comparison
}
