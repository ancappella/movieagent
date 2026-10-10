package config

import (
	"os"
	"path/filepath"
	"strconv"
)

const (
	DefaultTextModel  = "gemini-3.5-flash"
	DefaultImageModel = "nano-banana-pro"
	DefaultVideoModel = "veo-3.1-generate-preview"
)

type Config struct {
	AppName string
	Mode    string
	Host    string
	Port    int

	// Model catalog
	ModelsConfigPath  string
	DefaultTextModel  string
	DefaultImageModel string
	DefaultVideoModel string

	// Provider credentials / endpoints
	GeminiKey   string
	GeminiBase  string
	MoarkKey    string
	MoarkBase   string
	MinimaxKey  string
	MinimaxBase string
	OpenAIKey   string
	DeepSeekKey string
	DeepSeekBase string
	DeepSeekModel string

	VoicePython  string
	WhisperModel string

	// Dispatch concurrency (defaults to 3).
	DispatchMaxTasks int
	DispatchMaxLLM   int
	DispatchMaxTool  int
	DispatchMaxSkill int
}

func Load() Config {
	// Prefer project .env so GEMINI_API_KEY (and friends) are available to
	// the host process and LocalSandbox passthrough before reading os.Getenv.
	_ = LoadDotEnv(".env")

	return Config{
		AppName: getEnv("APP_NAME", "movieagent"),
		Mode:    getEnv("GIN_MODE", "debug"),
		Host:    getEnv("APP_HOST", "0.0.0.0"),
		Port:    getEnvAsInt("APP_PORT", 8080),

		ModelsConfigPath:  getEnv("MODELS_CONFIG", filepath.Join("internal", "service", "models", "models.yaml")),
		DefaultTextModel:  getEnv("GEMINI_MODEL", DefaultTextModel),
		DefaultImageModel: getEnv("DRAW_MODEL", DefaultImageModel),
		DefaultVideoModel: getEnv("WORKFLOW_VIDEO_MODEL", DefaultVideoModel),

		GeminiKey:     os.Getenv("GEMINI_API_KEY"),
		GeminiBase:    getEnv("GEMINI_BASE_URL", "https://generativelanguage.googleapis.com/v1beta"),
		MoarkKey:      firstEnv("MOARK_API_KEY", "MOARK_TOKEN", "GITEE_AI_TOKEN"),
		MoarkBase:     getEnv("MOARK_BASE_URL", "https://ai.gitee.com"),
		MinimaxKey:    firstEnv("MINIMAX_API_KEY", "MINIMAX_H3_API_KEY"),
		MinimaxBase:   getEnv("MINIMAX_BASE_URL", "https://api.minimax.io"),
		OpenAIKey:     os.Getenv("OPENAI_API_KEY"),
		DeepSeekKey:   firstEnv("DEEPSEEK_API_KEY", "OPENAI_API_KEY"),
		DeepSeekBase:  getEnv("DEEPSEEK_BASE_URL", "https://api.deepseek.com"),
		DeepSeekModel: getEnv("DEEPSEEK_MODEL", "deepseek-chat"),

		VoicePython:  getEnv("VOICE_PYTHON", ""),
		WhisperModel: getEnv("WHISPER_MODEL", "tiny"),

		DispatchMaxTasks: getEnvAsInt("DISPATCH_MAX_TASKS", 3),
		DispatchMaxLLM:   getEnvAsInt("DISPATCH_MAX_LLM", 3),
		DispatchMaxTool:  getEnvAsInt("DISPATCH_MAX_TOOL", 3),
		DispatchMaxSkill: getEnvAsInt("DISPATCH_MAX_SKILL", 3),
	}
}

func (c Config) Addr() string {
	return c.Host + ":" + strconv.Itoa(c.Port)
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getEnvAsInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if v := os.Getenv(key); v != "" {
			return v
		}
	}
	return ""
}
