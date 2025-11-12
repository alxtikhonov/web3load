package report

import (
	"encoding/json"
	"fmt"
	"os"
)

func WriteJSON(path string, r Result) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("report: marshal: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("report: write %s: %w", path, err)
	}
	return nil
}

func ReadJSON(path string) (Result, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Result{}, fmt.Errorf("report: read %s: %w", path, err)
	}
	var r Result
	if err := json.Unmarshal(data, &r); err != nil {
		return Result{}, fmt.Errorf("report: parse %s: %w", path, err)
	}
	return r, nil
}
