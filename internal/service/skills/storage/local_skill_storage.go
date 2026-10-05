package storage

// LocalSkillStorage is the filesystem-backed skill storage implementation.
type LocalSkillStorage struct {
	*SkillStorage
}

func NewLocalSkillStorage(containerPath string) *LocalSkillStorage {
	return &LocalSkillStorage{SkillStorage: NewSkillStorage(containerPath)}
}
