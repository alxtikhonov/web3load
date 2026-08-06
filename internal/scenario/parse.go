package scenario

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ParseYAML strictly decodes and validates scenario YAML from memory.
// Strict decoding (KnownFields) rejects any unrecognized field — including
// an inline `private_key:` that has no place anywhere in the schema — as a
// parse error instead of silently ignoring it. Used directly by distributed
// workers, which receive their scenario shard as text from the controller
// rather than reading it from a local file.
func ParseYAML(data []byte) (*Scenario, error) {
	var s Scenario
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&s); err != nil {
		return nil, fmt.Errorf("scenario: parse: %w", err)
	}
	if err := s.Validate(); err != nil {
		return nil, fmt.Errorf("scenario: invalid: %w", err)
	}
	return &s, nil
}

// ParseFile reads a scenario file and parses it via ParseYAML.
func ParseFile(path string) (*Scenario, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("scenario: read %s: %w", path, err)
	}
	s, err := ParseYAML(data)
	if err != nil {
		return nil, fmt.Errorf("scenario: %s: %w", path, err)
	}
	return s, nil
}
