package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// 项目根目录
func ProjectRoot() (string, error) {
	if envRoot := os.Getenv("DEER_FLOW_PROJECT_ROOT"); envRoot != "" {
		root, err := filepath.Abs(envRoot)
		if err != nil {
			return "", err
		}
		root = filepath.Clean(root)
		fi, err := os.Stat(root)
		if err != nil {
			if os.IsNotExist(err) {
			    return "" , fmt.Errorf(
					"DEER_FLOW_PROJECT_ROOT is set to '%s', but the resolved path '%s' does not exist",
					envRoot, root,
				)
			}
			return "", err
		}
		if !fi.IsDir() {
			return "", fmt.Errorf(
				"DEER_FLOW_PROJECT_ROOT is set to '%s', but the resolved path '%s' is not a directory",
				envRoot, root,
			)
		}
		return root, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Abs(cwd)
}

func RuntimeHome() (string , error) {
	if envHome := os.Getenv("DEER_FLOW_HOME"); envHome != "" {
		abs , err := filepath.Abs(envHome)
		if err != nil {
			return "", err
		}
		return filepath.Clean(abs), nil
	}
	root, err := ProjectRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, ".deer-flow"), nil
}

func ResolvePath(value string , base string) (string , error) {
	path := value
	if !filepath.IsAbs(path) {
		if base == "" {
			var err error
			base, err = ProjectRoot()
			if err != nil {
				return "", err
			}
		}
		path = filepath.Join(base, path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err 
	}
	return filepath.Clean(abs), nil
}

func ExistingProjectFile(names ...string) (string , error) {
	root, err := ProjectRoot()
	if err != nil {
		return "" , err 
	}
	for _, name := range names {
		candidate := filepath.Join(root, name)
        fi, err := os.Stat(candidate)
		if err == nil && fi.Mode().IsRegular() {
           return candidate, nil
		}
	}
	return "", nil
}

