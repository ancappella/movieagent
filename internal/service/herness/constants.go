package herness

const (
	DefaultSkillsContainerPath   = "/mnt/skills"
	ConversationReaderContextKey = "__conversation_reader"
	ConversationToolUse          = "deerflow.tools.conversation:read_conversation"
	ConversationToolName         = "read_conversation"
	BrowserFramesDirname         = ".browser-frames"
	ToolResultsDirname           = ".tool_results"
	MCPInternalDirname           = ".mcp"
	MCPTmpSubdir                 = ".mcp/tmp"
	DefaultMCPSessionInitTimeout = 60.0
	MCPTaskServerNameMaxLength   = 128
	MCPTaskRemoteIDMaxLength     = 255
	MCPTaskNameMaxLength         = 255
	MCPTaskPollAfterMaxSeconds   = 86400
	RunEventTypeMaxLength        = 32
	RunEventCategoryMaxLength    = 16
	WorkspaceChangesEventType    = "workspace_changes"
	WorkspaceChangesEventCategory = "workspace"

	// Context keys mirrored from DeerFlow runtime context.
	ContextKeyUserID   = "user_id"
	ContextKeyThreadID = "thread_id"
	ContextKeyRunID    = "run_id"
	ContextKeyTraceID  = "trace_id"
)
