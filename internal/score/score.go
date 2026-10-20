// Package score reads the parts of a Score workload file the gate needs.
package score

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Container struct {
	Image     string            `yaml:"image" json:"image"`
	Variables map[string]string `yaml:"variables" json:"variables,omitempty"`
}

type Port struct {
	Port       int `yaml:"port" json:"port"`
	TargetPort int `yaml:"targetPort" json:"targetPort,omitempty"`
}

type Resource struct {
	Type string `yaml:"type" json:"type"`
}

type Workload struct {
	APIVersion string `yaml:"apiVersion" json:"apiVersion"`
	Metadata   struct {
		Name string `yaml:"name" json:"name"`
	} `yaml:"metadata" json:"metadata"`
	Containers map[string]Container `yaml:"containers" json:"containers"`
	Service    struct {
		Ports map[string]Port `yaml:"ports" json:"ports,omitempty"`
	} `yaml:"service" json:"service"`
	Resources map[string]Resource `yaml:"resources" json:"resources"`
}

func Load(path string) (*Workload, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var w Workload
	if err := yaml.Unmarshal(b, &w); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if w.Metadata.Name == "" || len(w.Containers) == 0 {
		return nil, fmt.Errorf("%s: needs metadata.name and at least one container", path)
	}
	return &w, nil
}

// Target is the container port a published port forwards to.
func (p Port) Target() int {
	if p.TargetPort != 0 {
		return p.TargetPort
	}
	return p.Port
}
