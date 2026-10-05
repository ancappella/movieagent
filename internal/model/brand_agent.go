package model

// BrandAgentCardID is the stable registry id for the brand agent.
const BrandAgentCardID = "brand-agent"

// BrandAgentCard returns the brand-agent card from the default registry
// (populated from agent_cards.yaml). Falls back to a Lookup on a fresh load.
func BrandAgentCard() AgentCard {
	if err := EnsureDefaultLoaded(); err == nil {
		if card, err := Default.Lookup(BrandAgentCardID); err == nil {
			return card
		}
	}
	reg, err := LoadDefault()
	if err != nil {
		return AgentCard{ID: BrandAgentCardID, Name: "品牌 Agent", Mode: ModeAgent}
	}
	card, err := reg.Lookup(BrandAgentCardID)
	if err != nil {
		return AgentCard{ID: BrandAgentCardID, Name: "品牌 Agent", Mode: ModeAgent}
	}
	return card
}
