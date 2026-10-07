// Package sessionstore persists ai sessions as JSON files, one file per
// session: <dir>/<session ID>.json.
package sessionstore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/kncept/guacamole/ai"
)

// idPattern keeps session IDs safe to use as file names (no path
// separators, no "..").
var idPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// fileFormat is the on-disk JSON shape of a saved session.
type fileFormat struct {
	ID           string       `json:"id"`
	Model        string       `json:"model,omitempty"`
	SystemPrompt string       `json:"systemPrompt,omitempty"`
	Temperature  *float64     `json:"temperature,omitempty"`
	MaxTokens    *int         `json:"maxTokens,omitempty"`
	Messages     []ai.Message `json:"messages"`
}

// Save writes session to <dir>/<session ID>.json, creating dir if needed,
// and returns the path written.
func Save(dir string, session *ai.Session) (string, error) {
	if err := checkID(session.ID); err != nil {
		return "", err
	}
	path := filepath.Join(dir, session.ID+".json")

	data, err := json.MarshalIndent(fileFormat{
		ID:           session.ID,
		Model:        session.Model,
		SystemPrompt: session.SystemPrompt,
		Temperature:  session.Temperature,
		MaxTokens:    session.MaxTokens,
		Messages:     session.Messages(),
	}, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// Load reads the session saved at <dir>/<id>.json and returns a Session
// with its history and settings restored, running each exchange through
// loop.
func Load(dir string, id string, loop *ai.Loop) (*ai.Session, error) {
	if err := checkID(id); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, id+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("sessionstore: no saved session %q in %s", id, dir)
		}
		return nil, err
	}

	var f fileFormat
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("sessionstore: %s: %w", path, err)
	}

	session := ai.NewSession(loop, f.Messages...)
	session.ID = f.ID
	if session.ID == "" {
		session.ID = id
	}
	session.Model = f.Model
	session.SystemPrompt = f.SystemPrompt
	session.Temperature = f.Temperature
	session.MaxTokens = f.MaxTokens
	return session, nil
}

func checkID(id string) error {
	if !idPattern.MatchString(id) {
		return fmt.Errorf("sessionstore: invalid session ID %q", id)
	}
	return nil
}

// List returns the IDs of the sessions saved in dir, in sorted order. A
// directory that does not exist yet (nothing saved) is not an error: it
// simply has no sessions. Files that are not valid session files are
// ignored.
func List(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		id := strings.TrimSuffix(name, ".json")
		if checkID(id) != nil {
			continue
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, nil
}

// Delete removes the session saved at <dir>/<id>.json.
func Delete(dir string, id string) error {
	if err := checkID(id); err != nil {
		return err
	}
	path := filepath.Join(dir, id+".json")
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("sessionstore: no saved session %q in %s", id, dir)
		}
		return err
	}
	return nil
}
