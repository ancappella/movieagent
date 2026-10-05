package skillscan

// 获取scoped scanner
type UserScopedScanner struct {
    HostPath         string
	ContainerPath    string
	AppConfig        map[string]any
	IntegrationsRoot string
	UserSKillsRoot   string
	GlobalCustomRoot string
	SkillStatesFile  string
}

func NewUserScopedScanner(userID string, hostpath string, containerpath string, appconfig map[string]interface{}) *UserScopedScanner {
	userID := config.ValidateUserID(userID)
    paths := get_paths(userID)
}

