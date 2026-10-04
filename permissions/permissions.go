// Package permissions manages permissions granted by the user, persisted
// as a JSON file (by default ~/.guac/permissions.json).
package permissions

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Manager tracks granted permissions. It is not safe for concurrent use.
type Manager struct {
	path      string
	writeDirs []string

	// Ask asks the user to grant a permission interactively: it is called
	// with a directory that write access has been requested for, and
	// returns whether access is granted. A granted directory is persisted.
	// If Ask is nil, nothing is ever granted by asking.
	Ask func(dir string) bool

	// OnError observes persistence failures. Optional.
	OnError func(err error)
}

// fileFormat is the on-disk JSON shape of the permissions file.
type fileFormat struct {
	WriteDirectories []string `json:"writeDirectories"`
}

// Load reads the permissions file at path. A missing file is not an error:
// it just means nothing has been granted yet.
func Load(path string) (*Manager, error) {
	m := &Manager{path: path}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return m, nil
		}
		return nil, err
	}
	var f fileFormat
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("permissions: %s: %w", path, err)
	}
	for _, dir := range f.WriteDirectories {
		m.writeDirs = append(m.writeDirs, normalize(dir))
	}
	return m, nil
}

// WriteDirectories returns the directories write access has been granted
// for, as absolute paths.
func (m *Manager) WriteDirectories() []string {
	dirs := make([]string, len(m.writeDirs))
	copy(dirs, m.writeDirs)
	return dirs
}

// AllowWrite reports whether dir may be written to. Access covers a
// granted directory and everything under it.
//
// If access has not been granted yet, the user is asked (via Ask); a grant
// is persisted so the question is never asked again for that directory or
// its subdirectories.
func (m *Manager) AllowWrite(dir string) bool {
	dir = normalize(dir)
	for _, granted := range m.writeDirs {
		if withinDir(granted, dir) {
			return true
		}
	}

	if m.Ask == nil || !m.Ask(dir) {
		return false
	}
	m.writeDirs = append(m.writeDirs, dir)
	if err := m.save(); err != nil && m.OnError != nil {
		m.OnError(err)
	}
	return true
}

// save persists the permissions file, creating its directory as needed.
func (m *Manager) save() error {
	data, err := json.MarshalIndent(fileFormat{WriteDirectories: m.writeDirs}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(m.path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(m.path, data, 0o600)
}

// normalize turns dir into a clean absolute path, so comparisons and
// persisted grants are consistent.
func normalize(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return filepath.Clean(dir)
	}
	return abs
}

// withinDir reports whether target is granted or one of its
// subdirectories.
func withinDir(granted, target string) bool {
	if granted == target {
		return true
	}
	rel, err := filepath.Rel(granted, target)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
