package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// configFileName is GConfig's file name, stored in GuacDir().
const configFileName = "config.json"

// Policy is what guacamole does with a tool call that matches a permission
// rule.
type Policy string

const (
	PolicyAllow Policy = "allow"
	PolicyDeny  Policy = "deny"
	PolicyAsk   Policy = "ask"
)

// Permission is one rule for a tool: a value (e.g. a directory path) and the
// policy that applies to it.
type Permission struct {
	Value  string `json:"value"`
	Policy Policy `json:"policy"`
}

// ToolPermissions is the list of permission rules for one tool.
type ToolPermissions []Permission

// GConfig is the basis of the ~/.guac/config.json file: the directories
// guacamole uses, and the permission rules for each tool.
type GConfig struct {
	// SessionsDir is where saved sessions live. Left empty it defaults to
	// ~/.guac/session via InitDefaults.
	SessionsDir string `json:"sessionsDir"`
	// Permissions holds the permission rules of each tool, keyed by tool
	// name (e.g. "read_file", "bash").
	Permissions map[string]ToolPermissions `json:"permissions"`
}

// Load reads GConfig from GuacDir()/config.json — an empty config when the
// file does not exist yet — and applies InitDefaults.
func Load() (*GConfig, error) {
	dir, err := GuacDir()
	if err != nil {
		return nil, err
	}

	c := &GConfig{}
	data, err := os.ReadFile(filepath.Join(dir, configFileName))
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		// First run: no config file yet.
	} else if err := json.Unmarshal(data, c); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", configFileName, err)
	}

	c.InitDefaults()
	return c, nil
}

// InitDefaults populates any required members that are not set, like
// SessionsDir. Values that are already present are never overwritten.
func (c *GConfig) InitDefaults() {
	if c.SessionsDir == "" {
		if dir, err := GuacDir(); err == nil {
			c.SessionsDir = filepath.Join(dir, "session")
		}
	}
	if c.Permissions == nil {
		c.Permissions = map[string]ToolPermissions{}
	}
}

// Save writes the config back to GuacDir()/config.json, creating the
// directory if needed.
func (c *GConfig) Save() error {
	dir, err := GuacDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, configFileName), data, 0o644)
}

// GetToolPermissions returns the permission rules for the named tool. A tool
// with no rules returns an empty list.
func (c *GConfig) GetToolPermissions(toolname string) ToolPermissions {
	return c.Permissions[toolname]
}

// SetToolPermissions stores the permission rules for the named tool.
func (c *GConfig) SetToolPermissions(toolname string, permissions ToolPermissions) {
	if c.Permissions == nil {
		c.Permissions = map[string]ToolPermissions{}
	}
	c.Permissions[toolname] = permissions
}
