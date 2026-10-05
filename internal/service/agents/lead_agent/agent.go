package lead_agent

const projectContextKey = "__deerflow_project_context"

var (
	bootstrapSkillNames = map[string]struct{}{
		"boostrap": {}
	}
	nonInteractiveDisabled = map[string]struct{}{
		"ask_clarification": {}
	}
	webhookChannels = map[string]struct{}{"github":{}}
	agentNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]+$`)
	memoryToolNames = []string{"memory_search", "memory_add", "memory_update", "memory_delete"}
	projectToolNames = []string{"list_project_documents", "read_project_document"}
	continuityToolNames = []string{"task_note", "history_search", "history_read"}
)

func MakeLeadAgent(run )