package tools

import (
	"github.com/kncept/guacamole/ai"
	"github.com/kncept/guacamole/config"
	"github.com/kncept/guacamole/permissions"
)

// AccessChecker is the subset of PermissionsManager used by the tools. Each
// method enforces one permission category; FilesystemPermissions exposes the
// filesystem rules so list_allowed_directories can report them. A nil
// AccessChecker allows everything.
type AccessChecker interface {
	// IsAllowedPath enforces the filesystem category on path.
	IsAllowedPath(path string, access permissions.AccessKind) (bool, error)
	// IsAllowedShellCommand enforces the shell category on a command line.
	IsAllowedShellCommand(command string) (bool, error)
	// IsAllowedDomain enforces the web category on a request's domain.
	IsAllowedDomain(domain string) (bool, error)
	// FilesystemPermissions returns the filesystem permission subset:
	// the allow-all override and the per-directory rules.
	FilesystemPermissions() config.FilesystemPermissions
}

// AllTools returns every built-in tool. Pass a non-nil checker to enforce
// permissions; a nil checker allows everything. The user_question tool is
// only included when a non-nil callbackHandler is given, because the question
// cannot be asked without a handler to ask it.
func AllTools(checker AccessChecker, callbackHandler UserQuestionCallbackHandler) []ai.Tool {
	tools := make([]ai.Tool, 0)
	tools = append(tools, FileSystem(checker)...)
	tools = append(tools, Console(checker)...)
	tools = append(tools, Web(checker)...)
	if callbackHandler != nil {
		tools = append(tools, User(callbackHandler)...)
	}

	// time : current user time && convert time zone && perhaps ntp time?

	return tools
}
