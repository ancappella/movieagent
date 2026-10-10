package sandbox

import (
	"os"
	"regexp"
	"strings"
)

// Secret-looking names are stripped from the inherited process environment
// before request-scoped secrets are layered on top. Mirrors deer-flow
// sandbox/env_policy.py.
var (
	secretNamePatterns = []string{"KEY", "SECRET", "TOKEN", "PASS", "CREDENTIAL", "DSN"}
	blockedExactNames  = map[string]struct{}{
		"DATABASE_URL":      {},
		"DATABASE_URI":      {},
		"REDIS_URL":         {},
		"MONGODB_URI":       {},
		"MONGO_URL":         {},
		"AMQP_URL":          {},
		"RABBITMQ_URL":      {},
		"POSTGRES_URL":      {},
		"POSTGRESQL_URL":    {},
		"MYSQL_URL":         {},
		"CLICKHOUSE_URL":    {},
		"CONNECTION_STRING": {},
		"CONN_STR":          {},
		"GH_PAT":            {},
		"GITHUB_PAT":        {},
		"MYSQL_PWD":         {},
		"REDISCLI_AUTH":     {},
		"REDIS_AUTH":        {},
		"PGSERVICEFILE":     {},
		"SSH_AUTH_SOCK":     {},
	}
	// sandboxPassthroughEnv are host provider vars re-injected into the
	// sandbox after the secret strip (names contain KEY and would otherwise
	// be dropped). GEMINI_API_KEY is required for in-sandbox Gemini calls.
	sandboxPassthroughEnv = []string{
		"GEMINI_API_KEY",
		"GEMINI_BASE_URL",
		"GEMINI_MODEL",
		"GEMINI_HTTP_PROXY",
	}
	envNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

// IsBlockedEnvName reports whether name looks like a platform credential.
func IsBlockedEnvName(name string) bool {
	upper := strings.ToUpper(name)
	if _, ok := blockedExactNames[upper]; ok {
		return true
	}
	for _, token := range secretNamePatterns {
		if strings.Contains(upper, token) {
			return true
		}
	}
	return false
}

// ValidateExtraEnv rejects env keys that are not POSIX variable names.
func ValidateExtraEnv(extra map[string]string) error {
	for key := range extra {
		if !envNamePattern.MatchString(key) {
			return newError("extra env key is not a valid POSIX environment variable name", map[string]any{"key": key})
		}
	}
	return nil
}

// BuildSandboxEnv copies the process environment without secret-looking names,
// re-injects allowlisted provider keys (e.g. GEMINI_API_KEY), then overlays
// request-scoped secrets. Injected keys win even if blocked.
func BuildSandboxEnv(injected map[string]string) map[string]string {
	env := map[string]string{}
	for _, item := range os.Environ() {
		key, value, ok := strings.Cut(item, "=")
		if !ok || key == "" || IsBlockedEnvName(key) {
			continue
		}
		env[key] = value
	}
	for _, key := range sandboxPassthroughEnv {
		if v := os.Getenv(key); v != "" {
			env[key] = v
		}
	}
	for key, value := range injected {
		env[key] = value
	}
	return env
}

func envList(env map[string]string) []string {
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	return out
}
