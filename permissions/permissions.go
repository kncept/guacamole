// Package permissions enforces the category-based permission rules held in
// config.GConfig: filesystem (per directory, read/write), shell (one policy
// for all shell operations) and web (per domain). Unanswered checks go to
// the user input, and its answer is recorded in the config.
package permissions

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kncept/guacamole/config"
	"github.com/kncept/guacamole/userinput"
)

// The option labels offered when a permission check falls through to the
// user. The filesystem category has its own labels because a file check can
// scope each lasting answer to the file itself or to the directory that
// contains it. Every "always" answer — including "Always ask" — is recorded
// as a rule; "just this time" answers the current check only.
const (
	// Shell and web checks: one rule for the value itself.
	optionAllowAlways = "Allow Always"
	optionAllowOnce   = "Allow this Time"
	optionDenyForever = "Deny Forever"

	// Directory checks.
	optionDirectoryDeny  = "Always Deny"
	optionDirectoryAsk   = "Always ask"
	optionDirectoryOnce  = "Just This time"
	optionDirectoryAllow = "Always allow"

	// File checks: the lasting answers can cover the file or its directory.
	optionFileDenyFile       = "Always Deny file"
	optionFileDenyDirectory  = "Always Deny directory"
	optionFileAsk            = "Ask: Always ask"
	optionFileOnce           = "This time: just this time"
	optionFileAllowFile      = "Always: Always allow file"
	optionFileAllowDirectory = "Always: Always allow directory"
)

func NewPermissionsManager(
	config *config.GConfig,
	userInput userinput.RequestForegroundUserInput,
) *PermissionsManager {
	return &PermissionsManager{
		config:    config,
		userInput: userInput,
	}
}

// always use New(), never directly create
type PermissionsManager struct {
	config    *config.GConfig
	userInput userinput.RequestForegroundUserInput
}

// AccessKind splits a permission check into read and write.
type AccessKind bool

const (
	AccessRead  AccessKind = false
	AccessWrite AccessKind = true
)

// IsAllowedPath checks the filesystem category against path, split into the
// requested access kind. The allow-all override wins immediately; otherwise
// the most specific rule that answered this access kind applies. Rules that
// only answered the other kind are skipped, so a path inherits the nearest
// parent that answered this kind. Allow and deny rules are applied directly;
// ask rules (and no answer at all) fall through to the user input, whose
// answer is recorded and saved. When path is a file the prompt offers
// scoping each lasting answer to the file or its containing directory; a
// directory is covered as a whole.
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

	// The most specific rule that answered this access kind wins. A rule
	// that only answered the other kind is skipped, so a path inherits the
	// nearest parent that answered this kind.
	if policy, ok := findDirectoryPolicy(filesystem.Directories, path, access); ok {
		switch policy {
		case config.PolicyAllow:
			return true, nil
		case config.PolicyDeny:
			return false, nil
		case config.PolicyAsk:
			// fall through to the granter
		}
	}

	// A file can be allowed or denied on its own or through the directory
	// that contains it, so its prompt offers both scopes. A directory is
	// covered as a whole. Every answer except "just this time" records a
	// rule (including "Always ask").
	options := filesystemOptions(!isDirectory(path))
	answer := this.prompt(permissionQuestion(accessCategory(access), path), options)
	decision := interpretFilesystem(answer)
	if decision.once {
		return true, nil
	}
	if decision.atDirectory {
		path = filepath.Dir(path)
	}
	this.recordDirectory(path, access, decision.policy)
	return decision.policy != config.PolicyDeny, this.config.Save()
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
// applies, falling through to the user input as with the filesystem
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

// ask puts one permission question to the user input for the shell and web
// categories, whose answers are recorded against the value itself. "Allow
// this Time" allows the access without recording anything; an empty or
// missing answer (the prompt could not be asked) defaults to deny.
func (this *PermissionsManager) ask(category string, value string) config.Policy {
	answer := this.prompt(permissionQuestion(category, value),
		[]string{optionAllowOnce, optionAllowAlways, optionDenyForever})
	switch answer {
	case optionAllowOnce:
		return unrecorded
	case optionAllowAlways:
		return config.PolicyAllow
	default:
		return config.PolicyDeny
	}
}

// permissionQuestion formats the prompt shown for one permission check.
func permissionQuestion(category string, value string) string {
	return fmt.Sprintf("Allow %s access to %s?", category, value)
}

// filesystemOptions returns the prompt options for a filesystem check. A
// value that is (or will be) a file offers every lasting answer scoped to
// the file or to the directory that contains it; a directory offers one
// option per policy.
func filesystemOptions(isFile bool) []string {
	if isFile {
		return []string{
			optionFileDenyFile,
			optionFileDenyDirectory,
			optionFileAsk,
			optionFileOnce,
			optionFileAllowFile,
			optionFileAllowDirectory,
		}
	}
	return []string{
		optionDirectoryDeny,
		optionDirectoryAsk,
		optionDirectoryOnce,
		optionDirectoryAllow,
	}
}

// filesystemDecision is the outcome of a filesystem prompt answer: the
// policy to record and whether the rule covers the value itself or the
// directory that contains it. once answers the current check only, without
// recording anything.
type filesystemDecision struct {
	policy      config.Policy
	once        bool
	atDirectory bool
}

// interpretFilesystem interprets a filesystem prompt answer. Any
// unrecognised answer — including a closed or failing prompt — denies the
// value.
func interpretFilesystem(answer string) filesystemDecision {
	switch answer {
	case optionDirectoryDeny, optionFileDenyFile:
		return filesystemDecision{policy: config.PolicyDeny}
	case optionFileDenyDirectory:
		return filesystemDecision{policy: config.PolicyDeny, atDirectory: true}
	case optionDirectoryAsk, optionFileAsk:
		return filesystemDecision{policy: config.PolicyAsk}
	case optionDirectoryOnce, optionFileOnce:
		return filesystemDecision{once: true}
	case optionDirectoryAllow, optionFileAllowFile:
		return filesystemDecision{policy: config.PolicyAllow}
	case optionFileAllowDirectory:
		return filesystemDecision{policy: config.PolicyAllow, atDirectory: true}
	default:
		return filesystemDecision{policy: config.PolicyDeny}
	}
}

// prompt puts one question to the user input and returns the chosen option.
// A closed or failing prompt returns an empty answer.
func (this *PermissionsManager) prompt(text string, options []string) string {
	answer, err := this.userInput.Prompt(text, false, options)
	if err != nil {
		return ""
	}
	return answer
}

// isDirectory reports whether path exists and is a directory. A path that
// does not exist yet is treated as a file: the prompt then offers scoping
// each answer to the file or the directory that contains it.
func isDirectory(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// accessCategory labels an access kind for the granter prompt.
func accessCategory(access AccessKind) string {
	if access == AccessWrite {
		return "filesystem write"
	}
	return "filesystem read"
}

// findDirectoryPolicy returns the response for the requested access kind
// from the most specific rule that answered that kind. A rule that only
// answered the other kind is skipped, so path inherits the nearest ancestor
// that answered this kind. ok is false when no rule answered this kind,
// which means "ask". The longest (most specific) Directory wins.
func findDirectoryPolicy(rules []config.DirectoryPermission, path string, access AccessKind) (config.Policy, bool) {
	sorted := make([]config.DirectoryPermission, len(rules))
	copy(sorted, rules)
	sort.SliceStable(sorted, func(i, j int) bool {
		return len(sorted[i].Directory) > len(sorted[j].Directory)
	})

	for i := range sorted {
		if !pathCovers(sorted[i].Directory, path) {
			continue
		}
		policy := sorted[i].Read
		if access == AccessWrite {
			policy = sorted[i].Write
		}
		if policy != "" {
			return policy, true
		}
	}
	return "", false
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

// recordDirectory inserts or updates the rule for the exact path, setting
// only the requested access kind. The other kind is left unset (or kept as
// it was), so the path inherits a parent's response for it instead of
// silently overriding it with an ask.
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
	rule := config.DirectoryPermission{Directory: path}
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
