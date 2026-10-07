package tools

import (
	"github.com/kncept/guacamole/ai"
	"github.com/kncept/guacamole/permissions"
)

// AccessChecker is the subset of PermissionsManager used by the tools.
type AccessChecker interface {
	IsAllowedDirectory(toolName string, toolValue string, access permissions.AccessKind) (bool, error)
}

// AllTools returns every built-in tool. Pass a non-nil checker to enforce
// permissions; a nil checker allows everything. The user_question tool is
// only included when a non-nil callbackHandler is given, because the question
// cannot be asked without a handler to ask it.
func AllTools(checker AccessChecker, callbackHandler UserQuestionCallbackHandler) []ai.Tool {
	tools := make([]ai.Tool, 0)
	tools = append(tools, FileSystem(checker)...)
	tools = append(tools, Console()...)
	if callbackHandler != nil {
		tools = append(tools, User(callbackHandler)...)
	}

	// time : current user time && convert time zone && perhaps ntp time?

	// http_fetch: basic curl-like operations. Custom User-Agent, and allow specifying headers (and operation, and a body)

	return tools
}
