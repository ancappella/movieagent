package models

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// Registry holds models registered by ID (and aliases).
type Registry struct {
	mu   sync.RWMutex
	byID map[string]Model
}

func NewRegistry() *Registry {
	return &Registry{byID: make(map[string]Model)}
}

// Register inserts or replaces a model by ID.
func (r *Registry) Register(m Model) (Model, error) {
	id := strings.TrimSpace(m.ID)
	if err := validID(id); err != nil {
		return Model{}, err
	}
	tag, err := ParseTag(string(m.Tag))
	if err != nil {
		return Model{}, err
	}

	out := Model{
		ID:        id,
		Tag:       tag,
		Name:      strings.TrimSpace(m.Name),
		Provider:  strings.TrimSpace(m.Provider),
		Upstream:  strings.TrimSpace(m.Upstream),
		BaseURL:   strings.TrimSpace(m.BaseURL),
		APIKeyEnv: strings.TrimSpace(m.APIKeyEnv),
		Aliases:   uniqueAliases(id, m.Aliases),
	}
	if out.Name == "" {
		out.Name = id
	}
	if out.Upstream == "" {
		out.Upstream = id
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[strings.ToLower(id)] = out
	return out, nil
}

// MustRegister panics on registration failure (for init-time wiring).
func (r *Registry) MustRegister(m Model) Model {
	out, err := r.Register(m)
	if err != nil {
		panic(err)
	}
	return out
}

func (r *Registry) Lookup(idOrAlias string) (Model, error) {
	key := strings.TrimSpace(idOrAlias)
	if key == "" {
		return Model{}, ErrEmptyID
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	if m, ok := r.byID[strings.ToLower(key)]; ok {
		return m, nil
	}
	low := strings.ToLower(key)
	for _, m := range r.byID {
		for _, a := range m.Aliases {
			if strings.ToLower(strings.TrimSpace(a)) == low {
				return m, nil
			}
		}
	}
	return Model{}, fmt.Errorf("%w: %s", ErrNotFound, idOrAlias)
}

func (r *Registry) List(tag string) []Model {
	want := Tag("")
	if strings.TrimSpace(tag) != "" {
		parsed, err := ParseTag(tag)
		if err != nil {
			return nil
		}
		want = parsed
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]Model, 0, len(r.byID))
	for _, m := range r.byID {
		if want != "" && m.Tag != want {
			continue
		}
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Tag == out[j].Tag {
			return out[i].ID < out[j].ID
		}
		return out[i].Tag < out[j].Tag
	})
	return out
}

func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.byID)
}

// LoadFile loads a YAML catalog and registers every model by ID.
func LoadFile(path string) (*Registry, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return LoadBytes(raw)
}

// LoadBytes parses YAML catalog bytes into a registry.
func LoadBytes(raw []byte) (*Registry, error) {
	var cfg FileConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, err
	}

	reg := NewRegistry()
	for _, m := range cfg.Models {
		m.BaseURL = resolveDefaultRef(m.BaseURL, cfg.Defaults)
		if _, err := reg.Register(m); err != nil {
			return nil, fmt.Errorf("register %q: %w", m.ID, err)
		}
	}
	return reg, nil
}

// DefaultConfigPath returns the packaged models.yaml next to this package.
func DefaultConfigPath() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return filepath.Join("internal", "service", "models", "models.yaml")
	}
	return filepath.Join(filepath.Dir(file), "models.yaml")
}

// LoadDefault loads the embedded-path models.yaml catalog.
func LoadDefault() (*Registry, error) {
	return LoadFile(DefaultConfigPath())
}

func resolveDefaultRef(value string, defaults map[string]string) string {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "${defaults.") || !strings.HasSuffix(value, "}") {
		return value
	}
	key := strings.TrimSuffix(strings.TrimPrefix(value, "${defaults."), "}")
	if v, ok := defaults[key]; ok {
		return v
	}
	return value
}

func uniqueAliases(id string, aliases []string) []string {
	seen := map[string]bool{strings.ToLower(id): true}
	out := make([]string, 0, len(aliases))
	for _, a := range aliases {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		low := strings.ToLower(a)
		if seen[low] {
			continue
		}
		seen[low] = true
		out = append(out, a)
	}
	return out
}
