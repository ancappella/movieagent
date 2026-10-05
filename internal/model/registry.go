package model

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// CardRegistry stores AgentCard metadata by id (and aliases).
type CardRegistry struct {
	mu    sync.RWMutex
	byID  map[string]AgentCard
	order []string
}

// NewCardRegistry creates an empty agent card registry.
func NewCardRegistry() *CardRegistry {
	return &CardRegistry{byID: map[string]AgentCard{}}
}

// Default is the process-wide agent card registry.
var Default = NewCardRegistry()

// Register inserts or replaces a card. ID is required.
func (r *CardRegistry) Register(card AgentCard) (AgentCard, error) {
	id := strings.TrimSpace(card.ID)
	if err := validCardID(id); err != nil {
		return AgentCard{}, err
	}
	out := normalizeCard(card)
	out.ID = id
	now := time.Now().UTC()
	if out.RegisteredAt.IsZero() {
		out.RegisteredAt = now
	}
	out.UpdatedAt = now

	key := strings.ToLower(id)
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.byID[key]; !exists {
		r.order = append(r.order, key)
	}
	r.byID[key] = out
	return out, nil
}

// MustRegister panics if registration fails (init-time wiring).
func (r *CardRegistry) MustRegister(card AgentCard) AgentCard {
	out, err := r.Register(card)
	if err != nil {
		panic(err)
	}
	return out
}

// Unregister removes a card by id. Unknown ids are ignored.
func (r *CardRegistry) Unregister(id string) {
	key := strings.ToLower(strings.TrimSpace(id))
	if key == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.byID, key)
	filtered := r.order[:0]
	for _, item := range r.order {
		if item != key {
			filtered = append(filtered, item)
		}
	}
	r.order = filtered
}

// Lookup finds a card by id or alias.
func (r *CardRegistry) Lookup(idOrAlias string) (AgentCard, error) {
	key := strings.TrimSpace(idOrAlias)
	if key == "" {
		return AgentCard{}, ErrEmptyCardID
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	if c, ok := r.byID[strings.ToLower(key)]; ok {
		return c, nil
	}
	low := strings.ToLower(key)
	for _, c := range r.byID {
		for _, a := range c.Aliases {
			if strings.ToLower(strings.TrimSpace(a)) == low {
				return c, nil
			}
		}
	}
	return AgentCard{}, fmt.Errorf("%w: %s", ErrCardNotFound, idOrAlias)
}

// List returns registered cards, optionally filtered by mode and/or tag.
func (r *CardRegistry) List(mode, tag string) []AgentCard {
	mode = strings.TrimSpace(mode)
	tag = strings.TrimSpace(tag)

	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]AgentCard, 0, len(r.order))
	for _, key := range r.order {
		c, ok := r.byID[key]
		if !ok {
			continue
		}
		if mode != "" && string(c.Mode) != mode {
			continue
		}
		if tag != "" && !cardHasTag(c, tag) {
			continue
		}
		out = append(out, c)
	}
	return out
}

// Len returns how many cards are registered.
func (r *CardRegistry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.byID)
}

// Search returns cards whose id/name/description/tags contain q (case-insensitive).
func (r *CardRegistry) Search(q string) []AgentCard {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return r.List("", "")
	}
	all := r.List("", "")
	out := make([]AgentCard, 0, len(all))
	for _, c := range all {
		hay := strings.ToLower(c.ID + " " + c.Name + " " + c.Description + " " + strings.Join(c.Tags, " "))
		if strings.Contains(hay, q) {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func normalizeCard(c AgentCard) AgentCard {
	c.ID = strings.TrimSpace(c.ID)
	c.Name = strings.TrimSpace(c.Name)
	c.Description = strings.TrimSpace(c.Description)
	c.Version = strings.TrimSpace(c.Version)
	c.Package = strings.TrimSpace(c.Package)
	c.Entry = strings.TrimSpace(c.Entry)
	c.DocumentationURL = strings.TrimSpace(c.DocumentationURL)
	c.IconURL = strings.TrimSpace(c.IconURL)
	if c.Cover != nil {
		c.Cover.URL = strings.TrimSpace(c.Cover.URL)
		c.Cover.Alt = strings.TrimSpace(c.Cover.Alt)
		c.Cover.MIME = strings.TrimSpace(c.Cover.MIME)
		if c.Cover.URL == "" {
			c.Cover = nil
		}
	}
	if c.Name == "" {
		c.Name = c.ID
	}
	if c.Version == "" {
		c.Version = "1.0.0"
	}
	if c.Mode == "" {
		c.Mode = ModeAgent
	}
	c.Tags = uniqueStrings(c.Tags)
	c.Aliases = uniqueStringsExcept(c.Aliases, c.ID)
	c.DefaultInputModes = uniqueStrings(c.DefaultInputModes)
	c.DefaultOutputModes = uniqueStrings(c.DefaultOutputModes)
	if len(c.DefaultInputModes) == 0 {
		c.DefaultInputModes = []string{"text/plain", "image/*"}
	}
	if len(c.DefaultOutputModes) == 0 {
		c.DefaultOutputModes = []string{"text/plain", "image/png"}
	}
	return c
}

func cardHasTag(c AgentCard, want string) bool {
	want = strings.ToLower(want)
	for _, t := range c.Tags {
		if strings.ToLower(t) == want {
			return true
		}
	}
	for _, s := range c.Skills {
		for _, t := range s.Tags {
			if strings.ToLower(t) == want {
				return true
			}
		}
	}
	return false
}

func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		low := strings.ToLower(s)
		if seen[low] {
			continue
		}
		seen[low] = true
		out = append(out, s)
	}
	return out
}

func uniqueStringsExcept(in []string, except string) []string {
	except = strings.ToLower(strings.TrimSpace(except))
	seen := map[string]bool{except: true}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		low := strings.ToLower(s)
		if seen[low] {
			continue
		}
		seen[low] = true
		out = append(out, s)
	}
	return out
}
