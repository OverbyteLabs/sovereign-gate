// Package target reads a target file: where a deployment would go, and which
// provider service and region each Score resource maps to there.
package target

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Resource struct {
	Service string `yaml:"service" json:"service"`
	Region  string `yaml:"region" json:"region"`
}

type Target struct {
	Name      string              `yaml:"name" json:"name"`
	Kind      string              `yaml:"kind" json:"kind"`
	Provider  string              `yaml:"provider" json:"provider"`
	Resources map[string]Resource `yaml:"resources" json:"resources"`
}

func Load(path string) (*Target, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var t Target
	if err := yaml.Unmarshal(b, &t); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if t.Name == "" || t.Provider == "" {
		return nil, fmt.Errorf("%s: needs name and provider", path)
	}
	if t.Resources == nil {
		t.Resources = map[string]Resource{}
	}
	return &t, nil
}
