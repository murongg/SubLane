package vault

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

var errKeyInsideData = errors.New("credential key file must be outside the data directory")

// KeyPath keeps legacy instances usable and requires an explicitly configured key to be outside their data directory.
func KeyPath(dataDir, configured string) (string, error) {
	if configured == "" {
		return filepath.Join(dataDir, "credentials.key"), nil
	}
	if strings.ContainsAny(configured, "\x00\r\n") || os.IsPathSeparator(configured[len(configured)-1]) {
		return "", errors.New("invalid credential key path")
	}
	root, err := resolveDirectory(dataDir)
	if err != nil {
		return "", err
	}
	parent, err := resolveDirectory(filepath.Dir(configured))
	if err != nil {
		return "", err
	}
	path := filepath.Join(parent, filepath.Base(configured))
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return "", err
	}
	if relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errKeyInsideData
	}
	rootInfo, err := os.Stat(root)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if err == nil {
		// Filesystem identity catches case-insensitive aliases and direct bind aliases missed by lexical paths.
		for ancestor := parent; ; ancestor = filepath.Dir(ancestor) {
			info, err := os.Stat(ancestor)
			if err == nil && os.SameFile(rootInfo, info) {
				return "", errKeyInsideData
			}
			if err != nil && !os.IsNotExist(err) {
				return "", err
			}
			if filepath.Dir(ancestor) == ancestor {
				break
			}
		}
	}
	return path, nil
}

func resolveDirectory(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	var missing []string
	for {
		resolved, err := filepath.EvalSymlinks(absolute)
		if err == nil {
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return resolved, nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(absolute)
		if parent == absolute {
			return "", err
		}
		missing = append(missing, filepath.Base(absolute))
		absolute = parent
	}
}
