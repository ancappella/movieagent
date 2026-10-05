package config

import (
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// VirtualPathPrefix is the path prefix agents see inside the sandbox.
const VirtualPathPrefix = "/mnt/user-data"

const (
	safeUserIDDigestHexLen = 16
	acpVirtualPrefix       = "/mnt/acp-workspace"
)

var (
	safeUserIDRe        = regexp.MustCompile(`^[A-Za-z0-9_\-]+$`)
	safeIntegrationIDRe = regexp.MustCompile(`^[A-Za-z0-9_.\-]+$`)
	unsafeUserIDCharRe  = regexp.MustCompile(`[^A-Za-z0-9_\-]`)
	winDriveRe          = regexp.MustCompile(`^[A-Za-z]:[\\/]`)
	threadIDRe          = regexp.MustCompile(`^[A-Za-z0-9_\-]{1,64}$`)
	once  sync.Once
	paths *Paths
)

func ValidateThreadID(threadID string) (string, error) {
	if !threadIDRe.MatchString(threadID) {
		return "", fmt.Errorf("invalid thread ID: %s", threadID)
	}
	return threadID, nil
}

func ValidateUserID(userID string) (string, error) {
	if !safeUserIDRe.MatchString(userID) {
		return "", fmt.Errorf("invalid user ID: %s", userID)
	}
	return userID, nil
}

func ValidateIntegrationID(id string) (string, error) {
	if !safeIntegrationIDRe.MatchString(id) {
		return "", fmt.Errorf("invalid integration ID: %s", id)
	}
	if id == "." || id == ".." {
		return "", fmt.Errorf("invalid integration ID: %s", id)
	}
	return id, nil
}

func ValidateProjectID(projectID string) (string, error) {
	if !safeUserIDRe.MatchString(projectID) {
		return "", fmt.Errorf("invalid project ID: %s", projectID)
	}
	return projectID, nil
}

func MakeSafeUserID(raw string) (string, error) {
	if raw == "" {
		return "", fmt.Errorf("raw user ID is empty")
	}
	sanitized := unsafeUserIDCharRe.ReplaceAllString(raw, "-")
	if sanitized == raw {
		return raw, nil
	}
	sum := sha256.Sum256([]byte(raw))
	return sanitized + "-" + hex.EncodeToString(sum[:])[:safeUserIDDigestHexLen], nil
}

func legacySafeUserID(raw, sanitized string) string {
	sum := sha1.Sum([]byte(raw))
	return sanitized + "-" + hex.EncodeToString(sum[:])[:safeUserIDDigestHexLen]
}

func JoinHostPath(base string, parts ...string) string {
	if len(parts) == 0 {
		return base
	}
	isWin := winDriveRe.MatchString(base) || strings.HasPrefix(base, `\\`) || strings.Contains(base, `\`)
	if isWin {
		result := strings.ReplaceAll(filepath.ToSlash(base), `/`, `\`)
		for _, part := range parts {
			part = strings.Trim(strings.ReplaceAll(filepath.ToSlash(part), `/`, `\`), `\`)
			if part == "" {
				continue
			}
			result = strings.TrimRight(result, `\`) + `\` + part
		}
		return result
	}
	result := base
	for _, part := range parts {
		result = path.Join(result, part)
	}
	return result
}

func confinedJoin(base, rel string) (string, error) {
	baseAbs, err := filepath.Abs(base)
	if err != nil {
		return "", err
	}
	actual, err := filepath.Abs(filepath.Join(baseAbs, rel))
	if err != nil {
		return "", err
	}
	r, err := filepath.Rel(baseAbs, actual)
	if err != nil || r == ".." || strings.HasPrefix(r, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("access denied: path traversal detected")
	}
	return actual, nil
}

// Paths resolves DeerFlow host paths.
//
//	{base_dir}/                          # default {project_root}/.deer-flow
//	├── memory.json
//	├── USER.md
//	├── agents/{name}/
//	├── managed-subagents/{name}.json
//	├── integrations/skills/{provider}/{skill}/
//	├── skills_view/public/
//	├── threads/{thread_id}/
//	└── users/{user_id}/
//	    ├── memory.json
//	    ├── agents/{name}/memory.json
//	    ├── skills/custom/
//	    ├── skills_view/{custom,legacy,integrations}/
//	    ├── threads/{thread_id}/user-data/{workspace,uploads,outputs}/
//	    ├── threads/{thread_id}/acp-workspace/
//	    ├── threads/{thread_id}/skills_view/
//	    ├── projects/{project_id}/documents/...
//	    └── integrations/{integration_id}/{config,data}/
type Paths struct {
	baseDirOverride *string
}

func NewPaths(baseDir *string) (*Paths, error) {
	p := &Paths{}
	if baseDir == nil {
		return p, nil
	}
	abs, err := filepath.Abs(*baseDir)
	if err != nil {
		return nil, err
	}
	cleaned := filepath.Clean(abs)
	p.baseDirOverride = &cleaned
	return p, nil
}

func (p *Paths) BaseDir() (string, error) {
	if p != nil && p.baseDirOverride != nil {
		return *p.baseDirOverride, nil
	}
	return RuntimeHome()
}

func (p *Paths) HostBaseDir() (string, error) {
	if env := os.Getenv("DEER_FLOW_HOST_BASE_DIR"); env != "" {
		return env, nil
	}
	return p.BaseDir()
}

func (p *Paths) join(parts ...string) (string, error) {
	base, err := p.BaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(append([]string{base}, parts...)...), nil
}

// --- global layout ---

func (p *Paths) MemoryFile() (string, error) {
	return p.join("memory.json")
}

func (p *Paths) UserMDFile() (string, error) {
	return p.join("USER.md")
}

func (p *Paths) AgentsDir() (string, error) {
	return p.join("agents")
}

func (p *Paths) AgentDir(name string) (string, error) {
	return p.join("agents", strings.ToLower(name))
}

func (p *Paths) AgentMemoryFile(name string) (string, error) {
	return p.join("agents", strings.ToLower(name), "memory.json")
}

func (p *Paths) ManagedSubagentsDir() (string, error) {
	return p.join("managed-subagents")
}

func (p *Paths) ManagedSubagentFile(name string) (string, error) {
	return p.join("managed-subagents", strings.ToLower(name)+".json")
}

func (p *Paths) IntegrationSkillsDir() (string, error) {
	return p.join("integrations", "skills")
}

func (p *Paths) IntegrationSkillDir(provider, skill string) (string, error) {
	provider, err := ValidateIntegrationID(provider)
	if err != nil {
		return "", err
	}
	skill, err = ValidateIntegrationID(skill)
	if err != nil {
		return "", err
	}
	return p.join("integrations", "skills", provider, skill)
}

func (p *Paths) SkillsViewDir() (string, error) {
	return p.join("skills_view")
}

func (p *Paths) PublicSkillsViewDir() (string, error) {
	return p.join("skills_view", "public")
}

func (p *Paths) ThreadsDir() (string, error) {
	return p.join("threads")
}

// --- users ---

func (p *Paths) UserDir(userID string) (string, error) {
	id, err := ValidateUserID(userID)
	if err != nil {
		return "", err
	}
	return p.join("users", id)
}

func (p *Paths) UserMemoryFile(userID string) (string, error) {
	id, err := ValidateUserID(userID)
	if err != nil {
		return "", err
	}
	return p.join("users", id, "memory.json")
}

func (p *Paths) UserAgentsDir(userID string) (string, error) {
	id, err := ValidateUserID(userID)
	if err != nil {
		return "", err
	}
	return p.join("users", id, "agents")
}

func (p *Paths) UserAgentDir(userID, agentName string) (string, error) {
	id, err := ValidateUserID(userID)
	if err != nil {
		return "", err
	}
	return p.join("users", id, "agents", strings.ToLower(agentName))
}

func (p *Paths) UserAgentMemoryFile(userID, agentName string) (string, error) {
	id, err := ValidateUserID(userID)
	if err != nil {
		return "", err
	}
	return p.join("users", id, "agents", strings.ToLower(agentName), "memory.json")
}

func (p *Paths) UserCustomSkillsDir(userID string) (string, error) {
	id, err := ValidateUserID(userID)
	if err != nil {
		return "", err
	}
	return p.join("users", id, "skills", "custom")
}

func (p *Paths) UserSkillsViewDir(userID string) (string, error) {
	id, err := ValidateUserID(userID)
	if err != nil {
		return "", err
	}
	return p.join("users", id, "skills_view")
}

func (p *Paths) UserCustomSkillsViewDir(userID string) (string, error) {
	return p.userSkillsViewChild(userID, "custom")
}

func (p *Paths) UserLegacySkillsViewDir(userID string) (string, error) {
	return p.userSkillsViewChild(userID, "legacy")
}

func (p *Paths) UserIntegrationSkillsViewDir(userID string) (string, error) {
	return p.userSkillsViewChild(userID, "integrations")
}

func (p *Paths) userSkillsViewChild(userID, child string) (string, error) {
	id, err := ValidateUserID(userID)
	if err != nil {
		return "", err
	}
	return p.join("users", id, "skills_view", child)
}

func (p *Paths) ThreadDir(threadID string, userID *string) (string, error) {
	tid, err := ValidateThreadID(threadID)
	if err != nil {
		return "", err
	}
	if userID == nil {
		return p.join("threads", tid)
	}
	id, err := ValidateUserID(*userID)
	if err != nil {
		return "", err
	}
	return p.join("users", id, "threads", tid)
}

func (p *Paths) SandboxUserDataDir(threadID string, userID *string) (string, error) {
	dir, err := p.ThreadDir(threadID, userID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "user-data"), nil
}

func (p *Paths) SandboxWorkDir(threadID string, userID *string) (string, error) {
	return p.sandboxChild(threadID, userID, "workspace")
}

func (p *Paths) SandboxUploadsDir(threadID string, userID *string) (string, error) {
	return p.sandboxChild(threadID, userID, "uploads")
}

func (p *Paths) SandboxOutputsDir(threadID string, userID *string) (string, error) {
	return p.sandboxChild(threadID, userID, "outputs")
}

func (p *Paths) sandboxChild(threadID string, userID *string, child string) (string, error) {
	dir, err := p.SandboxUserDataDir(threadID, userID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, child), nil
}

func (p *Paths) ACPWorkspaceDir(threadID string, userID *string) (string, error) {
	dir, err := p.ThreadDir(threadID, userID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "acp-workspace"), nil
}

func (p *Paths) ThreadSkillsViewDir(threadID string, userID string) (string, error) {
	dir, err := p.ThreadDir(threadID, &userID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "skills_view"), nil
}

func (p *Paths) ProjectDocumentsDir(userID, projectID string) (string, error) {
	id, err := ValidateUserID(userID)
	if err != nil {
		return "", err
	}
	pid, err := ValidateProjectID(projectID)
	if err != nil {
		return "", err
	}
	return p.join("users", id, "projects", pid, "documents")
}

func (p *Paths) ProjectDocumentStagingDir(userID, projectID, uuid string) (string, error) {
	docs, err := p.ProjectDocumentsDir(userID, projectID)
	if err != nil {
		return "", err
	}
	return filepath.Join(docs, ".staging", uuid), nil
}

func (p *Paths) ProjectDocumentDir(userID, projectID, sha256hex, documentID string) (string, error) {
	if len(sha256hex) < 2 {
		return "", fmt.Errorf("sha256 must be at least 2 hex chars")
	}
	docs, err := p.ProjectDocumentsDir(userID, projectID)
	if err != nil {
		return "", err
	}
	return filepath.Join(docs, sha256hex[:2], sha256hex, documentID), nil
}

func (p *Paths) ProjectDocumentOriginal(userID, projectID, sha256hex, documentID, name string) (string, error) {
	dir, err := p.ProjectDocumentDir(userID, projectID, sha256hex, documentID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "original", name), nil
}

func (p *Paths) ProjectDocumentDerived(userID, projectID, sha256hex, documentID string) (string, error) {
	dir, err := p.ProjectDocumentDir(userID, projectID, sha256hex, documentID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "derived", "converted.md"), nil
}

func (p *Paths) UserIntegrationDir(userID, integrationID string) (string, error) {
	id, err := ValidateUserID(userID)
	if err != nil {
		return "", err
	}
	iid, err := ValidateIntegrationID(integrationID)
	if err != nil {
		return "", err
	}
	return p.join("users", id, "integrations", iid)
}

func (p *Paths) UserIntegrationConfigDir(userID, integrationID string) (string, error) {
	dir, err := p.UserIntegrationDir(userID, integrationID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config"), nil
}

func (p *Paths) UserIntegrationDataDir(userID, integrationID string) (string, error) {
	dir, err := p.UserIntegrationDir(userID, integrationID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "data"), nil
}

// ResolveVirtualPath maps /mnt/user-data/... to the host user-data directory.
func (p *Paths) ResolveVirtualPath(threadID, virtualPath string, userID *string) (string, error) {
	stripped := strings.TrimLeft(virtualPath, "/")
	prefix := strings.TrimLeft(VirtualPathPrefix, "/")
	if stripped != prefix && !strings.HasPrefix(stripped, prefix+"/") {
		return "", fmt.Errorf("path must start with %s", VirtualPathPrefix)
	}
	relative := strings.TrimLeft(strings.TrimPrefix(stripped, prefix), "/")
	base, err := p.SandboxUserDataDir(threadID, userID)
	if err != nil {
		return "", err
	}
	return confinedJoin(base, relative)
}

func (p *Paths) EnsureThreadDirs(threadID string, userID *string) error {
	dirs := []func() (string, error){
		func() (string, error) { return p.SandboxWorkDir(threadID, userID) },
		func() (string, error) { return p.SandboxUploadsDir(threadID, userID) },
		func() (string, error) { return p.SandboxOutputsDir(threadID, userID) },
		func() (string, error) { return p.ACPWorkspaceDir(threadID, userID) },
	}
	for _, fn := range dirs {
		dir, err := fn()
		if err != nil {
			return err
		}
		if err := os.MkdirAll(dir, 0o777); err != nil {
			return err
		}
		_ = os.Chmod(dir, 0o777)
	}
	return nil
}

func (p *Paths) DeleteThreadDir(threadID string, userID *string) error {
	dir, err := p.ThreadDir(threadID, userID)
	if err != nil {
		return err
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil
	}
	return os.RemoveAll(dir)
}


func GetPaths() *Paths {
	once.Do(func() {
		p, err := NewPaths(nil)
		if err != nil {
			p = &Paths{}
		}
		paths = p
	})
	return paths
}

func (p *Paths) UserSkillsDir(userID string) (string, error) {
	id, err := ValidateUserID(userID)
	if err != nil {
		return "", err
	}
	return p.join("users", id, "skills")
}

