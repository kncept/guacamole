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

// Permissions holds every permission rule, grouped by category: filesystem,
// shell and web.
type Permissions struct {
	Filesystem FilesystemPermissions `json:"filesystem"`
	Shell      ShellPermissions      `json:"shell"`
	Web        WebPermissions        `json:"web"`
}

// FilesystemPermissions tracks read and write access per directory.
// AllowAll is an override: when set, every path is allowed without asking
// and the directory rules are ignored.
type FilesystemPermissions struct {
	AllowAll    bool                  `json:"allowAll"`
	Directories []DirectoryPermission `json:"directories"`
}

// DirectoryPermission is one rule: reads and writes under Directory (and
// everything below it) follow the given policies.
type DirectoryPermission struct {
	Directory string `json:"directory"`
	Read      Policy `json:"read"`
	Write     Policy `json:"write"`
}

// ShellPermissions is a single allow/ask/deny policy shared by every shell
// operation; unlike the other categories it has no per-value rules.
type ShellPermissions struct {
	Policy Policy `json:"policy"`
}

// WebPermissions tracks access per domain. AllowAll is an override: when
// set, every domain is allowed without asking and the domain rules are
// ignored.
type WebPermissions struct {
	AllowAll bool               `json:"allowAll"`
	Domains  []DomainPermission `json:"domains"`
}

// DomainPermission is one rule: requests to Domain (and its subdomains)
// follow Policy.
type DomainPermission struct {
	Domain string `json:"domain"`
	Policy Policy `json:"policy"`
}

// GConfig is the basis of the ~/.guac/config.json file: the directories
// guacamole uses, and the category-based permission rules.
type GConfig struct {
	// SessionsDir is where saved sessions live. Left empty it defaults to
	// ~/.guac/session via InitDefaults.
	SessionsDir string `json:"sessionsDir"`
	// Permissions holds the permission rules of each category.
	Permissions Permissions `json:"permissions"`
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
func (this *GConfig) InitDefaults() {
	if this.SessionsDir == "" {
		if dir, err := GuacDir(); err == nil {
			this.SessionsDir = filepath.Join(dir, "session")
		}
	}
	if this.Permissions.Shell.Policy == "" {
		// An unset shell policy means "ask before every shell command".
		this.Permissions.Shell.Policy = PolicyAsk
	}
}

// Save writes the config back to GuacDir()/config.json, creating the
// directory if needed.
func (this *GConfig) Save() error {
	dir, err := GuacDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(this, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, configFileName), data, 0o644)
}
