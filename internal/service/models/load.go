package models

// LoadFromConfigPath loads the model catalog from an explicit config path.
// Empty path falls back to the packaged models.yaml.
func LoadFromConfigPath(path string) (*Registry, error) {
	if path == "" {
		return LoadDefault()
	}
	return LoadFile(path)
}
