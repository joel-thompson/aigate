package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

const DefaultConfigFile = ".aigate.yml"

type Config struct {
	Claude *ClaudeConfig `yaml:"claude"`
	Git    *GitConfig    `yaml:"git"`
}

type ClaudeConfig struct {
	SessionStart *SessionStartConfig `yaml:"session-start"`
}

type SessionStartConfig struct {
	Context []ProviderConfig `yaml:"context"`
}

type GitConfig struct {
	PreCommit *PreCommitConfig `yaml:"pre-commit"`
}

type PreCommitConfig struct {
	Checks []ProviderConfig `yaml:"checks"`
}

type ProviderConfig struct {
	Type     string `yaml:"type"`
	MaxDepth int    `yaml:"max_depth,omitempty"`
	Command  string `yaml:"command,omitempty"`
	Label    string `yaml:"label,omitempty"`
}

var validTypes = map[string]bool{
	"project-structure": true,
	"shell":             true,
	"secrets-scan":      true,
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}
	return Parse(data)
}

func Parse(data []byte) (*Config, error) {
	var cfg Config
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	if err := validate(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func validate(cfg *Config) error {
	if cfg.Claude != nil && cfg.Claude.SessionStart != nil {
		for i, p := range cfg.Claude.SessionStart.Context {
			if err := validateProvider(p, fmt.Sprintf("claude.session-start.context[%d]", i)); err != nil {
				return err
			}
		}
	}
	if cfg.Git != nil && cfg.Git.PreCommit != nil {
		for i, p := range cfg.Git.PreCommit.Checks {
			if err := validateProvider(p, fmt.Sprintf("git.pre-commit.checks[%d]", i)); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateProvider(p ProviderConfig, path string) error {
	if p.Type == "" {
		return fmt.Errorf("%s: missing required field \"type\"", path)
	}
	if !validTypes[p.Type] {
		return fmt.Errorf("%s: unknown provider type %q", path, p.Type)
	}
	if p.Type == "shell" && strings.TrimSpace(p.Command) == "" {
		return fmt.Errorf("%s: type \"shell\" requires \"command\" field", path)
	}
	return nil
}
