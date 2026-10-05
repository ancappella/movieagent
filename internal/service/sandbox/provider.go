package sandbox

import (
	"os"
	"sync"

	"movieagent/internal/service/config"
)

const (
	genericSandboxID                = "local"
	DefaultMaxCachedThreadSandboxes = 256
)

// LocalProvider acquires per-thread LocalSandbox instances.
// IDs are "local" without a thread, or "local:{user}:{thread}" with one.
type LocalProvider struct {
	paths           *config.Paths
	skillsContainer string
	maxCached       int
	mu              sync.Mutex
	generic         *LocalSandbox
	threadSandboxes map[string]*LocalSandbox
	threadOrder     []string
}

func NewLocalProvider(paths *config.Paths) *LocalProvider {
	if paths == nil {
		paths = config.GetPaths()
	}
	return &LocalProvider{
		paths:           paths,
		skillsContainer: DefaultSkillsContainerPath,
		maxCached:       DefaultMaxCachedThreadSandboxes,
		threadSandboxes: map[string]*LocalSandbox{},
	}
}

// Acquire returns a sandbox id. An empty threadID reuses the generic singleton.
func (p *LocalProvider) Acquire(threadID, userID string) (string, error) {
	if threadID == "" {
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.generic == nil {
			p.generic = NewLocalSandbox(genericSandboxID, nil)
		}
		return p.generic.ID(), nil
	}
	if userID == "" {
		userID = "default"
	}
	if _, err := config.ValidateUserID(userID); err != nil {
		return "", err
	}
	if _, err := config.ValidateThreadID(threadID); err != nil {
		return "", err
	}

	user := userID
	if err := p.paths.EnsureThreadDirs(threadID, &user); err != nil {
		return "", err
	}
	mappings, err := p.threadMappings(threadID, user)
	if err != nil {
		return "", err
	}
	id := "local:" + user + ":" + threadID

	p.mu.Lock()
	defer p.mu.Unlock()
	if existing, ok := p.threadSandboxes[id]; ok {
		p.touch(id)
		_ = existing
		p.threadSandboxes[id] = NewLocalSandbox(id, mappings)
		return id, nil
	}
	p.threadSandboxes[id] = NewLocalSandbox(id, mappings)
	p.threadOrder = append(p.threadOrder, id)
	p.evict()
	return id, nil
}

func (p *LocalProvider) Get(id string) (Sandbox, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if id == genericSandboxID && p.generic != nil {
		return p.generic, true
	}
	sb, ok := p.threadSandboxes[id]
	return sb, ok
}

func (p *LocalProvider) Release(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if id == genericSandboxID {
		p.generic = nil
		return
	}
	delete(p.threadSandboxes, id)
	filtered := p.threadOrder[:0]
	for _, item := range p.threadOrder {
		if item != id {
			filtered = append(filtered, item)
		}
	}
	p.threadOrder = filtered
}

func (p *LocalProvider) threadMappings(threadID, userID string) ([]PathMapping, error) {
	user := userID
	userData, err := p.paths.SandboxUserDataDir(threadID, &user)
	if err != nil {
		return nil, err
	}
	workspace, err := p.paths.SandboxWorkDir(threadID, &user)
	if err != nil {
		return nil, err
	}
	uploads, err := p.paths.SandboxUploadsDir(threadID, &user)
	if err != nil {
		return nil, err
	}
	outputs, err := p.paths.SandboxOutputsDir(threadID, &user)
	if err != nil {
		return nil, err
	}
	acp, err := p.paths.ACPWorkspaceDir(threadID, &user)
	if err != nil {
		return nil, err
	}
	mappings := []PathMapping{
		{ContainerPath: UserDataVirtualPrefix + "/workspace", LocalPath: workspace},
		{ContainerPath: UserDataVirtualPrefix + "/uploads", LocalPath: uploads},
		{ContainerPath: UserDataVirtualPrefix + "/outputs", LocalPath: outputs},
		{ContainerPath: UserDataVirtualPrefix, LocalPath: userData},
		{ContainerPath: ACPWorkspaceVirtualPrefix, LocalPath: acp},
	}
	root := p.skillsContainer
	candidates := []struct {
		container string
		local     func() (string, error)
	}{
		{root + "/public", p.paths.PublicSkillsViewDir},
		{root + "/custom", func() (string, error) { return p.paths.UserCustomSkillsViewDir(userID) }},
		{root + "/legacy", func() (string, error) { return p.paths.UserLegacySkillsViewDir(userID) }},
		{root + "/integrations", func() (string, error) { return p.paths.UserIntegrationSkillsViewDir(userID) }},
	}
	for _, candidate := range candidates {
		local, err := candidate.local()
		if err != nil {
			continue
		}
		if info, statErr := os.Stat(local); statErr == nil && info.IsDir() {
			mappings = append(mappings, PathMapping{
				ContainerPath: candidate.container,
				LocalPath:     local,
				ReadOnly:      true,
			})
		}
	}
	return mappings, nil
}

func (p *LocalProvider) touch(id string) {
	filtered := p.threadOrder[:0]
	for _, item := range p.threadOrder {
		if item != id {
			filtered = append(filtered, item)
		}
	}
	p.threadOrder = append(filtered, id)
}

func (p *LocalProvider) evict() {
	for len(p.threadSandboxes) > p.maxCached && len(p.threadOrder) > 0 {
		oldest := p.threadOrder[0]
		p.threadOrder = p.threadOrder[1:]
		delete(p.threadSandboxes, oldest)
	}
}
