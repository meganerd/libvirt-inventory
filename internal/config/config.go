package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// HypervisorConfig defines a single hypervisor connection.
type HypervisorConfig struct {
	Name string `yaml:"name"`
	URI  string `yaml:"uri"`
}

// Config is the top-level configuration.
type Config struct {
	Hypervisors []HypervisorConfig `yaml:"hypervisors"`
	OutputDir   string             `yaml:"output_dir"`
	SSHUser     string             `yaml:"ssh_user"`
}

// Load reads and parses a YAML config file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", path, err)
	}

	if len(cfg.Hypervisors) == 0 {
		return nil, fmt.Errorf("config %s: no hypervisors defined", path)
	}

	if cfg.OutputDir == "" {
		cfg.OutputDir = "."
	}

	return &cfg, nil
}
