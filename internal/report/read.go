package report

import (
	"encoding/json"
	"fmt"
	"os"
)

func Read(path string) (Report, error) {
	file, err := os.Open(path)
	if err != nil {
		return Report{}, fmt.Errorf("open report file: %w", err)
	}
	defer file.Close()

	var parsed Report
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&parsed); err != nil {
		return Report{}, fmt.Errorf("decode report file %q: %w", path, err)
	}
	return parsed, nil
}
