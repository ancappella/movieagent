package agents

import "movieagent/internal/model"

// Info is the studio-facing projection of an AgentCard.
type Info struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Mode        string             `json:"mode"`
	Tags        []string           `json:"tags"`
	Description string             `json:"description"`
	Skill       string             `json:"skill,omitempty"`
	Skills      []model.AgentSkill `json:"skills,omitempty"`
	Version     string             `json:"version,omitempty"`
	Package     string             `json:"package,omitempty"`
	Cover       *model.AgentCover  `json:"cover,omitempty"`
	IconURL     string             `json:"icon_url,omitempty"`
}

// Catalog returns registered agent cards as studio Info rows.
func Catalog() []Info {
	cards := model.Default.List("", "")
	out := make([]Info, 0, len(cards))
	for _, c := range cards {
		out = append(out, toInfo(c))
	}
	return out
}

// Filter returns agents matching mode and/or tag.
func Filter(mode, tag string) []Info {
	cards := model.Default.List(mode, tag)
	out := make([]Info, 0, len(cards))
	for _, c := range cards {
		out = append(out, toInfo(c))
	}
	return out
}

// Search filters cards by free-text query.
func Search(q string) []Info {
	cards := model.Default.Search(q)
	out := make([]Info, 0, len(cards))
	for _, c := range cards {
		out = append(out, toInfo(c))
	}
	return out
}

func toInfo(c model.AgentCard) Info {
	skill := ""
	if c.Extra != nil {
		if s, ok := c.Extra["skill"].(string); ok {
			skill = s
		}
	}
	if skill == "" && len(c.Skills) > 0 {
		skill = c.Skills[0].ID
	}
	return Info{
		ID:          c.ID,
		Name:        c.Name,
		Mode:        string(c.Mode),
		Tags:        append([]string(nil), c.Tags...),
		Description: c.Description,
		Skill:       skill,
		Skills:      append([]model.AgentSkill(nil), c.Skills...),
		Version:     c.Version,
		Package:     c.Package,
		Cover:       c.Cover,
		IconURL:     c.IconURL,
	}
}
