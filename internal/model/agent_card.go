// Package model holds domain metadata types and registries.
// Agent cards describe discoverable agent identity and capabilities.
package model

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrCardNotFound  = errors.New("agent card not found")
	ErrEmptyCardID   = errors.New("agent card id is required")
	ErrInvalidCardID = errors.New("invalid agent card id")
	ErrDuplicateCard = errors.New("duplicate agent card id")
)

// AgentMode is how the studio / runtime invokes the agent.
type AgentMode string

const (
	ModeAgent AgentMode = "agent"
	ModeAsk   AgentMode = "ask"
)

// AgentCapabilities declares runtime features (A2A-inspired subset).
type AgentCapabilities struct {
	Streaming            bool `json:"streaming" yaml:"streaming"`
	PushNotifications    bool `json:"push_notifications,omitempty" yaml:"push_notifications,omitempty"`
	Sandbox              bool `json:"sandbox,omitempty" yaml:"sandbox,omitempty"`
	BrandAssets          bool `json:"brand_assets,omitempty" yaml:"brand_assets,omitempty"`
	ExtendedAgentCard    bool `json:"extended_agent_card,omitempty" yaml:"extended_agent_card,omitempty"`
}

// AgentSkill is one discoverable capability advertised on the card.
type AgentSkill struct {
	ID          string   `json:"id" yaml:"id"`
	Name        string   `json:"name" yaml:"name"`
	Description string   `json:"description,omitempty" yaml:"description,omitempty"`
	Tags        []string `json:"tags,omitempty" yaml:"tags,omitempty"`
	Examples    []string `json:"examples,omitempty" yaml:"examples,omitempty"`
	InputModes  []string `json:"input_modes,omitempty" yaml:"input_modes,omitempty"`
	OutputModes []string `json:"output_modes,omitempty" yaml:"output_modes,omitempty"`
}

// AgentProvider identifies who publishes the agent.
type AgentProvider struct {
	Organization string `json:"organization,omitempty" yaml:"organization,omitempty"`
	URL          string `json:"url,omitempty" yaml:"url,omitempty"`
}

// AgentInterface is a contact endpoint for the agent (A2A-inspired).
type AgentInterface struct {
	URL             string `json:"url" yaml:"url"`
	ProtocolBinding string `json:"protocol_binding,omitempty" yaml:"protocol_binding,omitempty"`
	ProtocolVersion string `json:"protocol_version,omitempty" yaml:"protocol_version,omitempty"`
}

// AgentCard is the registered metadata document for one agent.
type AgentCard struct {
	// ID is the stable registry key (e.g. brand-agent).
	ID          string `json:"id" yaml:"id"`
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description" yaml:"description"`
	Version     string `json:"version" yaml:"version"`
	Mode        AgentMode `json:"mode" yaml:"mode"`

	// Package is the Go import path or module path of the implementation.
	Package string `json:"package,omitempty" yaml:"package,omitempty"`
	// Entry is the primary Go type or factory name.
	Entry string `json:"entry,omitempty" yaml:"entry,omitempty"`

	Provider     *AgentProvider     `json:"provider,omitempty" yaml:"provider,omitempty"`
	Capabilities AgentCapabilities  `json:"capabilities" yaml:"capabilities"`
	Skills       []AgentSkill       `json:"skills,omitempty" yaml:"skills,omitempty"`
	Interfaces   []AgentInterface   `json:"interfaces,omitempty" yaml:"interfaces,omitempty"`

	Tags               []string `json:"tags,omitempty" yaml:"tags,omitempty"`
	Aliases            []string `json:"aliases,omitempty" yaml:"aliases,omitempty"`
	DefaultInputModes  []string `json:"default_input_modes,omitempty" yaml:"default_input_modes,omitempty"`
	DefaultOutputModes []string `json:"default_output_modes,omitempty" yaml:"default_output_modes,omitempty"`
	DocumentationURL   string   `json:"documentation_url,omitempty" yaml:"documentation_url,omitempty"`
	IconURL            string   `json:"icon_url,omitempty" yaml:"icon_url,omitempty"`
	// Cover is the agent card cover image shown in studio / discovery UIs.
	Cover *AgentCover `json:"cover,omitempty" yaml:"cover,omitempty"`

	// Extra holds app-specific metadata not covered by the card schema.
	Extra map[string]any `json:"extra,omitempty" yaml:"extra,omitempty"`

	RegisteredAt time.Time `json:"registered_at,omitempty" yaml:"registered_at,omitempty"`
	UpdatedAt    time.Time `json:"updated_at,omitempty" yaml:"updated_at,omitempty"`
}

// AgentCover is the card cover / hero image metadata.
type AgentCover struct {
	URL    string `json:"url" yaml:"url"`
	Alt    string `json:"alt,omitempty" yaml:"alt,omitempty"`
	Width  int    `json:"width,omitempty" yaml:"width,omitempty"`
	Height int    `json:"height,omitempty" yaml:"height,omitempty"`
	// MIME is optional, e.g. image/png.
	MIME string `json:"mime,omitempty" yaml:"mime,omitempty"`
}

func validCardID(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return ErrEmptyCardID
	}
	if strings.ContainsAny(id, `/\:`) {
		return ErrInvalidCardID
	}
	return nil
}
