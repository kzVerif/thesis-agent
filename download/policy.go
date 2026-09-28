package download

import (
	"fmt"
	"path/filepath"
	"ws-agent/internal/protectedpath"
)

// ValidateRoots validates configuration without creating folders. An empty
// allowlist means only the default tree is allowed in Service mode.
func ValidateRoots(directory string, roots []string, runtimeRoot, installRoot string) ([]string, error) {
	if len(roots) == 0 {
		roots = []string{directory}
	}
	cleaned := make([]string, 0, len(roots))
	for _, root := range roots {
		path, err := cleanDestination(root)
		if err != nil {
			return nil, fmt.Errorf("invalid DOWNLOAD_ALLOWED_ROOTS entry: %w", err)
		}
		if filepath.Dir(path) == path {
			return nil, fmt.Errorf("download root must not be an entire drive")
		}
		if runtimeRoot != "" {
			data := filepath.Join(runtimeRoot, "data")
			if containsPath(path, runtimeRoot) || (containsPath(runtimeRoot, path) && (!containsPath(data, path) || containsPath(path, data))) {
				return nil, fmt.Errorf("download roots must not overlap private Agent runtime state")
			}
		}
		if installRoot != "" && (containsPath(path, installRoot) || containsPath(installRoot, path)) {
			return nil, fmt.Errorf("download roots must not overlap installed Agent binaries")
		}
		cleaned = append(cleaned, path)
	}
	path, err := cleanDestination(directory)
	if err != nil {
		return nil, fmt.Errorf("invalid DOWNLOAD_DIRECTORY: %w", err)
	}
	for _, root := range cleaned {
		if containsPath(root, path) {
			return cleaned, nil
		}
	}
	return nil, fmt.Errorf("DOWNLOAD_DIRECTORY must be inside DOWNLOAD_ALLOWED_ROOTS")
}

func (m *Manager) boundaryFor(path string) *protectedpath.Boundary {
	if m.cfg.Boundary == nil {
		return nil
	}
	if containsPath(m.cfg.Boundary.Root, path) {
		return m.cfg.Boundary
	}
	for _, root := range m.cfg.AllowedRoots {
		if containsPath(root, path) {
			return protectedpath.NewDownloadBoundary(root, m.cfg.Boundary.Report)
		}
	}
	// Out-of-policy operations retain the original rejecting boundary.
	return m.cfg.Boundary
}
