package permissions

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/kncept/guacamole/config"
)

type PerissionGranter interface {
	AskForAccess(toolName string, toolValue string) config.Policy
}

func NewPermissionsManager(
	config *config.GConfig,
	perissionGranter PerissionGranter,
) *PermissionsManager {
	return &PermissionsManager{
		config:           config,
		perissionGranter: perissionGranter,
	}
}

// always use New(), never directly create
type PermissionsManager struct {
	config           *config.GConfig
	perissionGranter PerissionGranter
}

// AccessKind splits a permission check into read and write.
type AccessKind bool

const (
	AccessRead  AccessKind = false
	AccessWrite AccessKind = true
)

// IsAllowedDirectory checks the tool's rules against toolValue (typically a
// directory path), split into the requested access kind. Allow and deny
// rules are applied directly; ask rules (and no matching rule at all) fall
// through to the PermissionGranter, and its answer is recorded and saved.
func (this *PermissionsManager) IsAllowedDirectory(toolName string, toolValue string, access AccessKind) (bool, error) {
	var err error = nil
	permissionsChanged := false
	toolPermissions := this.config.GetToolPermissions(toolName)

	// Most specific matching rule wins.
	matching := findMatching(toolPermissions, toolValue)
	if matching != nil {
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

	// Ask the user.
	answer := this.perissionGranter.AskForAccess(toolName, toolValue)
	if answer == config.PolicyAsk {
		// stays as ask, don't even record it
		return true, nil
	}

	// Record the answer so future calls don't ask again. A granter that
	// returns "" (e.g. closed the prompt) defaults to deny.
	if answer == "" {
		answer = config.PolicyDeny
	}
	this.recordPolicy(toolName, toolValue, access, answer)
	permissionsChanged = true

	if permissionsChanged {
		err = this.config.Save()
	}

	return answer == config.PolicyAllow, err
}

// findMatching returns the most specific rule whose Value covers toolValue,
// or nil when no rule matches.
func findMatching(rules config.ToolPermissions, toolValue string) *config.Permission {
	if len(rules) == 0 {
		return nil
	}

	// Sort by value length, longest first, so the most specific rule wins.
	sorted := make(config.ToolPermissions, len(rules))
	copy(sorted, rules)
	sort.SliceStable(sorted, func(i, j int) bool {
		return len(sorted[i].Value) > len(sorted[j].Value)
	})

	for i := range sorted {
		if ruleCovers(sorted[i].Value, toolValue) {
			return &sorted[i]
		}
	}
	return nil
}

// ruleCovers reports whether a rule Value covers toolValue. "*" and ""
// cover everything; otherwise a path Value covers itself and anything under
// it.
func ruleCovers(ruleValue string, toolValue string) bool {
	if ruleValue == "" || ruleValue == "*" {
		return true
	}
	ruleClean := filepath.Clean(ruleValue)
	valueClean := filepath.Clean(toolValue)
	if ruleClean == valueClean {
		return true
	}
	return strings.HasPrefix(valueClean, ruleClean+string(filepath.Separator))
}

// recordPolicy inserts or updates the rule for the exact toolValue/policy
// pair, then stores the updated slice back on the config.
func (this *PermissionsManager) recordPolicy(toolName string, toolValue string, access AccessKind, policy config.Policy) {
	rules := this.config.GetToolPermissions(toolName)
	if rules == nil {
		rules = make([]config.Permission, 0)
	}
	for i := range rules {
		if rules[i].Value == toolValue {
			if access == AccessWrite {
				rules[i].Write = policy
			} else {
				rules[i].Read = policy
			}
			this.config.SetToolPermissions(toolName, rules)
			return
		}
	}
	rules = append(rules, config.Permission{Value: toolValue, Read: config.PolicyAsk, Write: config.PolicyAsk})
	if access == AccessWrite {
		rules[len(rules)-1].Write = policy
	} else {
		rules[len(rules)-1].Read = policy
	}
	this.config.SetToolPermissions(toolName, rules)
}
