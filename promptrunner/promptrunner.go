// Package promptrunner runs prompts through a streaming chat session,
// printing the streamed results, saving the session to disk after each
// turn, and guarding file writes behind user-granted permissions.
package promptrunner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kncept/guacamole/ai"
	"github.com/kncept/guacamole/config"
	"github.com/kncept/guacamole/permissions"
	"github.com/kncept/guacamole/restfulai"
	"github.com/kncept/guacamole/sessionstore"
	"github.com/kncept/guacamole/tools"
)

// Sessions live at ~/.guac/session/<session ID>.json; permissions at
// ~/.guac/permissions.json.
const (
	sessionsDirName     = "session"
	permissionsFileName = "permissions.json"
)

type PromptRunner interface {
	// RunPrompt sends one user message through the session, prints the
	// streamed reply, and saves the session.
	RunPrompt(prompt string) error

	// Reset starts a fresh session with a new ID, discarding the history.
	Reset()

	// SessionID returns the current session's ID.
	SessionID() string

	// Save writes the current session to the sessions directory and
	// returns the path written.
	Save() (string, error)
}

type promptRunner struct {
	loop        *ai.Loop
	model       string
	session     *ai.Session
	sessionsDir string
	perms       *permissions.Manager

	// interactive reports whether stdin is a terminal. Only interactive
	// runs get the "thinking..." indicator, so piped output stays clean.
	interactive bool
	thinking    bool
}

// NewPromptRunner starts a chat session over conf's provider. If resumeID
// is non-empty, the session with that ID is loaded from the sessions
// directory and continues where it left off.
//
// ask prompts the user with a yes/no question (e.g. a permission request);
// a nil ask denies everything that would need to ask.
func NewPromptRunner(conf *config.ApiModelInterfaceDetails, resumeID string, ask func(question string) bool) (PromptRunner, error) {
	if ask == nil {
		ask = func(string) bool { return false }
	}

	guacDir, err := config.GuacDir()
	if err != nil {
		return nil, err
	}

	provider := restfulai.NewRestfulAI(conf)

	this := &promptRunner{
		model:       conf.ModelName,
		sessionsDir: filepath.Join(guacDir, sessionsDirName),
		interactive: isTerminal(os.Stdin),
	}

	loop := ai.NewLoop(provider)
	loop.Stream = true
	loop.OnChunk = func(chunk ai.Chunk) error {
		this.clearThinking()
		fmt.Print(chunk.Delta)
		return nil
	}
	loop.OnError = func(err error, attempt int) {
		// Failed attempts must be visible: silent retries look like the
		// prompt was ignored.
		this.clearThinking()
		if attempt <= loop.Retries {
			fmt.Printf("(attempt %d failed: %v; retrying)\n", attempt, err)
			this.showThinking()
		}
	}
	this.loop = loop

	perms, err := permissions.Load(filepath.Join(guacDir, permissionsFileName))
	if err != nil {
		return nil, err
	}
	perms.Ask = func(dir string) bool {
		this.clearThinking()
		granted := ask(fmt.Sprintf("Grant write access to %s (and its subdirectories)? [Y/n]", dir))
		this.showThinking()
		return granted
	}
	perms.OnError = func(err error) {
		fmt.Printf("(warning: could not save permissions: %v)\n", err)
	}
	this.perms = perms

	if resumeID != "" {
		session, err := sessionstore.Load(this.sessionsDir, resumeID, loop)
		if err != nil {
			return nil, err
		}
		session.Tools = this.tracedTools()
		this.session = session
		fmt.Printf("(resumed session %s: %d messages)\n", resumeID, len(session.Messages()))
	} else {
		this.Reset()
	}
	return this, nil
}

func (this *promptRunner) RunPrompt(prompt string) error {
	// Show activity right away: the model can take a while to produce its
	// first token, and a silent wait looks like the prompt was ignored.
	this.showThinking()
	_, err := this.session.Say(context.Background(), prompt)
	this.clearThinking()
	if err != nil {
		return err
	}

	// The streamed text was already printed chunk by chunk by OnChunk, so
	// only the trailing newline is needed here.
	fmt.Println()
	if _, err := this.Save(); err != nil {
		fmt.Printf("(warning: could not save session: %v)\n", err)
	}
	return nil
}

func (this *promptRunner) Reset() {
	this.session = ai.NewSession(this.loop)
	this.session.Model = this.model
	this.session.Tools = this.tracedTools()
}

func (this *promptRunner) SessionID() string {
	return this.session.ID
}

func (this *promptRunner) Save() (string, error) {
	return sessionstore.Save(this.sessionsDir, this.session)
}

// tracedTools wraps the file tools so every call is printed: tool activity
// must be visible, otherwise a tool round looks like a hung prompt.
func (this *promptRunner) tracedTools() []ai.Tool {
	ts := tools.FileSystem(this.perms.AllowWrite)
	for i := range ts {
		name := ts[i].Name
		handler := ts[i].Handler
		ts[i].Handler = func(ctx context.Context, args json.RawMessage) (string, error) {
			this.clearThinking()
			fmt.Printf("→ %s %s\n", name, truncate(string(args), 80))
			out, err := handler(ctx, args)
			this.showThinking()
			return out, err
		}
	}
	return ts
}

// showThinking prints the "thinking..." indicator, once.
func (this *promptRunner) showThinking() {
	if this.interactive && !this.thinking {
		fmt.Print(thinkingMessage)
		this.thinking = true
	}
}

// clearThinking erases the "thinking..." indicator, if it is shown.
func (this *promptRunner) clearThinking() {
	if this.interactive && this.thinking {
		fmt.Printf("\r%s\r", strings.Repeat(" ", len(thinkingMessage)))
		this.thinking = false
	}
}

// thinkingMessage is shown while waiting for the model's first output.
const thinkingMessage = "thinking..."

// isTerminal reports whether f is a terminal (character device).
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// truncate shortens s to at most n bytes, appending "..." if cut.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
