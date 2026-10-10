package dispatch

// DefaultConcurrency is the default slot count for tasks, LLM, tools, and skills.
const DefaultConcurrency = 3

// Config controls global task scheduling and per-resource rate limits.
type Config struct {
	// MaxTasks is how many AgentTasks may run concurrently (dispatch pool).
	MaxTasks int
	// MaxLLM is concurrent LLM calls (global).
	MaxLLM int
	// MaxTool is concurrent calls per tool name.
	MaxTool int
	// MaxSkill is concurrent calls per skill name.
	MaxSkill int
}

// DefaultConfig returns all limits set to DefaultConcurrency (3).
func DefaultConfig() Config {
	return Config{
		MaxTasks: DefaultConcurrency,
		MaxLLM:   DefaultConcurrency,
		MaxTool:  DefaultConcurrency,
		MaxSkill: DefaultConcurrency,
	}
}

func (c Config) normalized() Config {
	out := c
	if out.MaxTasks < 1 {
		out.MaxTasks = DefaultConcurrency
	}
	if out.MaxLLM < 1 {
		out.MaxLLM = DefaultConcurrency
	}
	if out.MaxTool < 1 {
		out.MaxTool = DefaultConcurrency
	}
	if out.MaxSkill < 1 {
		out.MaxSkill = DefaultConcurrency
	}
	return out
}
