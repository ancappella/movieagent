package model

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"gopkg.in/yaml.v3"
)

// FileConfig is the on-disk YAML catalog for agent cards.
type FileConfig struct {
	Version int         `yaml:"version"`
	Agents  []AgentCard `yaml:"agents"`
}

// DefaultConfigPath returns the packaged agent_cards.yaml next to this package.
func DefaultConfigPath() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return filepath.Join("internal", "model", "agent_cards.yaml")
	}
	return filepath.Join(filepath.Dir(file), "agent_cards.yaml")
}

// LoadFile loads agent cards from a YAML file into a new registry.
func LoadFile(path string) (*CardRegistry, error) {
	if path == "" {
		path = DefaultConfigPath()
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return LoadBytes(raw)
}

// LoadBytes parses YAML catalog bytes into a registry.
func LoadBytes(raw []byte) (*CardRegistry, error) {
	var cfg FileConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, err
	}
	reg := NewCardRegistry()
	if err := reg.LoadConfig(cfg); err != nil {
		return nil, err
	}
	return reg, nil
}

// LoadConfig registers every agent card from a parsed FileConfig.
func (r *CardRegistry) LoadConfig(cfg FileConfig) error {
	for i, card := range cfg.Agents {
		if _, err := r.Register(card); err != nil {
			id := card.ID
			if id == "" {
				id = fmt.Sprintf("index-%d", i)
			}
			return fmt.Errorf("register %q: %w", id, err)
		}
	}
	return nil
}

// LoadFromConfigPath loads cards from path (or default) into a new registry.
func LoadFromConfigPath(path string) (*CardRegistry, error) {
	if path == "" {
		return LoadDefault()
	}
	return LoadFile(path)
}

// LoadDefault loads the packaged agent_cards.yaml.
func LoadDefault() (*CardRegistry, error) {
	return LoadFile(DefaultConfigPath())
}

// LoadDefaultInto clears nothing; it registers (upserts) all cards from the
// default YAML into the given registry.
func LoadDefaultInto(r *CardRegistry) error {
	if r == nil {
		return fmt.Errorf("nil registry")
	}
	raw, err := os.ReadFile(DefaultConfigPath())
	if err != nil {
		return err
	}
	var cfg FileConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return err
	}
	return r.LoadConfig(cfg)
}

var defaultLoadOnce sync.Once
var defaultLoadErr error

// EnsureDefaultLoaded loads agent_cards.yaml into Default once.
func EnsureDefaultLoaded() error {
	defaultLoadOnce.Do(func() {
		defaultLoadErr = LoadDefaultInto(Default)
	})
	return defaultLoadErr
}

func init() {
	_ = EnsureDefaultLoaded()
}
