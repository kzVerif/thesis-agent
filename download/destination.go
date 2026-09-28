package download

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"
)

func validFilename(name string) bool {
	if !filepath.IsLocal(name) || name == "." || name == ".." || strings.ContainsAny(name, `/\`) || strings.ContainsFunc(name, unicode.IsControl) {
		return false
	}
	return runtime.GOOS != "windows" || (!strings.ContainsAny(name, `:<>"|?*`) && !strings.HasSuffix(name, ".") && !strings.HasSuffix(name, " "))
}

func invalidDestination() error {
	return &JobError{Code: InvalidDestinationPath, Message: "destination_path must be an allowed absolute local directory"}
}

func (m *Manager) destinationDirectory(path string) (string, error) {
	if path == "" {
		path = m.cfg.Directory
	}
	path, err := cleanDestination(path)
	if err != nil {
		return "", err
	}
	roots := m.cfg.AllowedRoots
	if len(roots) == 0 && m.cfg.Boundary != nil {
		roots = []string{m.cfg.Directory}
	}
	if len(roots) > 0 {
		for _, root := range roots {
			if containsPath(root, path) {
				return path, nil
			}
		}
		return "", invalidDestination()
	}
	return path, nil
}

func cleanDestination(path string) (string, error) {
	if !filepath.IsAbs(path) || strings.HasPrefix(path, `\`) || strings.HasPrefix(path, "//") || strings.ContainsFunc(path, unicode.IsControl) {
		return "", invalidDestination()
	}
	components := path
	if runtime.GOOS == "windows" {
		volume := filepath.VolumeName(path)
		if len(volume) != 2 || volume[1] != ':' || !((volume[0] >= 'A' && volume[0] <= 'Z') || (volume[0] >= 'a' && volume[0] <= 'z')) {
			return "", invalidDestination()
		}
		components = strings.ReplaceAll(path[len(volume):], `\`, "/")
	} else if strings.Contains(path, `\`) {
		return "", invalidDestination()
	}
	for _, component := range strings.Split(components, "/") {
		if component != "" && !validFilename(component) {
			return "", invalidDestination()
		}
	}
	return filepath.Clean(path), nil
}

func containsPath(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && (rel == "." || filepath.IsLocal(rel))
}

// Reject existing links in every ancestor, including Windows junctions. Missing
// directories are allowed and created only after this check succeeds.
func checkDestinationLinks(path string) error {
	for current := path; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil && !os.IsNotExist(err) {
			return &JobError{Code: DiskWriteFailed, Message: "could not inspect destination directory", Err: err}
		}
		if err == nil && isDestinationLink(info) {
			return invalidDestination()
		}
		if filepath.Dir(current) == current {
			return nil
		}
	}
}

func (m *Manager) prepareDestination(c Command) (string, error) {
	directory, err := m.destinationDirectory(c.DestinationPath)
	if err != nil {
		return directory, err
	}
	if err := checkDestinationLinks(directory); err != nil {
		return "", err
	}
	mkdir := func(path string) error { return os.MkdirAll(path, 0700) }
	if boundary := m.boundaryFor(directory); boundary != nil {
		mkdir = boundary.EnsureDirectory
	}
	if err := mkdir(directory); err != nil {
		return "", &JobError{Code: DiskWriteFailed, Message: "could not create destination directory", Err: err}
	}
	return directory, checkDestinationLinks(directory)
}
