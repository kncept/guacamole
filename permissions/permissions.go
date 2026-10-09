// Package permissions enforces the category-based permission rules held in
// config.GConfig: filesystem (per directory, read/write), shell (one policy
// for all shell operations) and web (per domain). Unanswered checks go to
// the PermissionGranter, and its answer is recorded in the config.
package permissions

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/kncept/guacamole/config"
)

// PermissionGranter asks the user for a decision on one permission check.
// category names the category (and, for the filesystem, the access kind):
// "filesystem read", "filesystem write", "shell" or "web". value is the
// path, command or domain being accessed.
type PermissionGranter interface {
	AskForAccess(category string, value string) config.Policy
}

func NewPermissionsManager(
	config *config.GConfig,
	permissionGranter PermissionGranter,
) *PermissionsManager {
	return &PermissionsManager{
		config:            config,
		permissionGranter: permissionGranter,
	}
}

// always use New(), never directly create
type PermissionsManager struct {
	config            *config.GConfig
	permissionGranter PermissionGranter
}

// AccessKind splits a permission check into read and write.
type AccessKind bool

const (
	AccessRead  AccessKind = false
	AccessWrite AccessKind = true
)

// IsAllowedPath checks the filesystem category against path, split into the
// requested access kind. The allow-all override wins immediately; otherwise
// the most specific directory rule applies. Allow and deny rules are applied
// directly; ask rules (and no matching rule at all) fall through to the
// PermissionGranter, and its answer is recorded and saved.
func (this *PermissionsManager) IsAllowedPath(path string, access AccessKind) (bool, error) {
	// Always resolve to absolute path to handle "." and other relative paths consistently
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false, err
	}
	path = absPath

	filesystem := this.config.Permissions.Filesystem
	if filesystem.AllowAll {
		return true, nil
	}

	// Most specific matching directory wins.
	if matching := findMatchingDirectory(filesystem.Directories, path); matching != nil {
		policy := matching.Read
		if access == AccessWrite {
			policy = matching.Write
		}
		switch policy {
		case config.PolicyAllow:
			return true, nil
		case config.PolicyDeny:
			return false, nil
		case config.PolicyAsk:
			// fall through to the granter
		}
	}

	answer := this.ask(accessCategory(access), path)
	if answer == unrecorded {
		return true, nil
	}
	this.recordDirectory(path, access, answer)
	return answer == config.PolicyAllow, this.config.Save()
}

// IsAllowedShellCommand checks the shell category: one allow/ask/deny
// policy shared by every shell operation, whatever shell it runs in. An ask
// answer is recorded, so one grant (or denial) covers all shell commands.
func (this *PermissionsManager) IsAllowedShellCommand(command string) (bool, error) {
	switch this.config.Permissions.Shell.Policy {
	case config.PolicyAllow:
		return true, nil
	case config.PolicyDeny:
		return false, nil
	case config.PolicyAsk:
		// fall through to the granter
	}

	answer := this.ask("shell", command)
	if answer == unrecorded {
		return true, nil
	}
	this.config.Permissions.Shell.Policy = answer
	return answer == config.PolicyAllow, this.config.Save()
}

// IsAllowedDomain checks the web category against domain. The allow-all
// override wins immediately; otherwise the most specific domain rule
// applies, falling through to the PermissionGranter as with the filesystem
// category. The answer is recorded and saved.
func (this *PermissionsManager) IsAllowedDomain(domain string) (bool, error) {
	web := this.config.Permissions.Web
	if web.AllowAll {
		return true, nil
	}

	if matching := findMatchingDomain(web.Domains, domain); matching != nil {
		switch matching.Policy {
		case config.PolicyAllow:
			return true, nil
		case config.PolicyDeny:
			return false, nil
		case config.PolicyAsk:
			// fall through to the granter
		}
	}

	answer := this.ask("web", domain)
	if answer == unrecorded {
		return true, nil
	}
	this.recordDomain(domain, answer)
	return answer == config.PolicyAllow, this.config.Save()
}

// FilesystemPermissions returns the filesystem permission subset — the
// allow-all override and the per-directory rules — so tools (notably
// list_allowed_directories) can report what is granted.
func (this *PermissionsManager) FilesystemPermissions() config.FilesystemPermissions {
	return this.config.Permissions.Filesystem
}

// unrecorded is the ask answer that allows access this time only, without
// writing anything to the config.
const unrecorded = config.PolicyAsk

// ask puts one question to the granter. A PolicyAsk answer means "allow
// this time" and stays unrecorded; a "" answer (the prompt was closed)
// defaults to deny.
func (this *PermissionsManager) ask(category string, value string) config.Policy {
	answer := this.permissionGranter.AskForAccess(category, value)
	if answer == config.PolicyAsk {
		return unrecorded
	}
	if answer == "" {
		return config.PolicyDeny
	}
	return answer
}

// accessCategory labels an access kind for the granter prompt.
func accessCategory(access AccessKind) string {
	if access == AccessWrite {
		return "filesystem write"
	}
	return "filesystem read"
}

// findMatchingDirectory returns the rule whose Directory covers path, or nil
// when no rule matches. The longest (most specific) Directory wins.
func findMatchingDirectory(rules []config.DirectoryPermission, path string) *config.DirectoryPermission {
	if len(rules) == 0 {
		return nil
	}
	sorted := make([]config.DirectoryPermission, len(rules))
	copy(sorted, rules)
	sort.SliceStable(sorted, func(i, j int) bool {
		return len(sorted[i].Directory) > len(sorted[j].Directory)
	})

	for i := range sorted {
		if pathCovers(sorted[i].Directory, path) {
			return &sorted[i]
		}
	}
	return nil
}

// pathCovers reports whether a directory rule covers path. "*" and "" cover
// everything; otherwise a directory covers itself and anything under it.
func pathCovers(ruleDirectory string, path string) bool {
	if ruleDirectory == "" || ruleDirectory == "*" {
		return true
	}
	ruleClean := filepath.Clean(ruleDirectory)
	valueClean := filepath.Clean(path)
	if ruleClean == valueClean {
		return true
	}
	return strings.HasPrefix(valueClean, ruleClean+string(filepath.Separator))
}

// findMatchingDomain returns the rule whose Domain covers domain, or nil
// when no rule matches. The longest (most specific) Domain wins.
func findMatchingDomain(rules []config.DomainPermission, domain string) *config.DomainPermission {
	if len(rules) == 0 {
		return nil
	}
	sorted := make([]config.DomainPermission, len(rules))
	copy(sorted, rules)
	sort.SliceStable(sorted, func(i, j int) bool {
		return len(sorted[i].Domain) > len(sorted[j].Domain)
	})

	for i := range sorted {
		if domainCovers(sorted[i].Domain, domain) {
			return &sorted[i]
		}
	}
	return nil
}

// domainCovers reports whether a domain rule covers domain. "*" and "" cover
// everything; otherwise a domain covers itself and its subdomains
// ("example.com" covers "api.example.com").
func domainCovers(ruleDomain string, domain string) bool {
	if ruleDomain == "" || ruleDomain == "*" {
		return true
	}
	rule := normalizeDomain(ruleDomain)
	value := normalizeDomain(domain)
	return value == rule || strings.HasSuffix(value, "."+rule)
}

// normalizeDomain lowercases a domain and strips a leading "*." wildcard,
// which is how subdomain rules are commonly written.
func normalizeDomain(domain string) string {
	return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(domain), "*."))
}

// recordDirectory inserts or updates the rule for the exact path/policy
// pair, then stores the updated slice back on the config.
func (this *PermissionsManager) recordDirectory(path string, access AccessKind, policy config.Policy) {
	directories := this.config.Permissions.Filesystem.Directories
	for i := range directories {
		if directories[i].Directory == path {
			if access == AccessWrite {
				directories[i].Write = policy
			} else {
				directories[i].Read = policy
			}
			this.config.Permissions.Filesystem.Directories = directories
			return
		}
	}
	rule := config.DirectoryPermission{Directory: path, Read: config.PolicyAsk, Write: config.PolicyAsk}
	if access == AccessWrite {
		rule.Write = policy
	} else {
		rule.Read = policy
	}
	this.config.Permissions.Filesystem.Directories = append(directories, rule)
}

// recordDomain inserts or updates the rule for the exact domain, then
// stores the updated slice back on the config.
func (this *PermissionsManager) recordDomain(domain string, policy config.Policy) {
	domains := this.config.Permissions.Web.Domains
	for i := range domains {
		if normalizeDomain(domains[i].Domain) == normalizeDomain(domain) {
			domains[i].Policy = policy
			this.config.Permissions.Web.Domains = domains
			return
		}
	}
	this.config.Permissions.Web.Domains = append(domains,
		config.DomainPermission{Domain: domain, Policy: policy})
}
