package scenario

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ParseFile reads, strictly decodes, and validates a scenario file.
// Strict decoding (KnownFields) rejects any unrecognized field — including
// an inline `private_key:` that has no place anywhere in the schema — as a
// parse error instead of silently ignoring it.
func ParseFile(path string) (*Scenario, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("scenario: read %s: %w", path, err)
	}

	var s Scenario
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&s); err != nil {
		return nil, fmt.Errorf("scenario: parse %s: %w", path, err)
	}

	if err := s.Validate(); err != nil {
		return nil, fmt.Errorf("scenario: invalid %s: %w", path, err)
	}
	return &s, nil
}
