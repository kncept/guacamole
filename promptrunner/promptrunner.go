// Package promptrunner runs prompts through a streaming chat session,
// printing the streamed results, and saving the session to disk after each
// turn.
package promptrunner

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/kncept/guacamole/ai"
	"github.com/kncept/guacamole/config"
	"github.com/kncept/guacamole/restfulai"
	"github.com/kncept/guacamole/sessionstore"
	"github.com/kncept/guacamole/tools"
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

	// interactive reports whether stdin is a terminal. Only interactive
	// runs get the "thinking..." indicator, so piped output stays clean.
	interactive bool
	thinking    bool
}

// NewPromptRunner starts a chat session over conf's provider. If resumeID
// is non-empty, the session with that ID is loaded from the sessions
// directory and continues where it left off.
// The sessions directory is taken from cfg (which applies its defaults).
func NewPromptRunner(conf *config.ApiModelInterfaceDetails, cfg *config.GConfig, resumeID string) (PromptRunner, error) {
	sessionsDir := cfg.SessionsDir

	provider, err := restfulai.NewRestfulAI(conf)
	if err != nil {
		return nil, err
	}

	this := &promptRunner{
		model:       conf.ModelName,
		sessionsDir: sessionsDir,
		interactive: isTerminal(os.Stdin),
	}

	loop := ai.NewLoop(provider)
	loop.Stream = true
	loop.OnChunk = func(chunk ai.Chunk) error {
		this.clearThinking()
		fmt.Print(chunk.Delta)
		return nil
	}
	loop.OnRequest = func(req ai.Request) {
		log.Printf("promptrunner: sending request: model %s, %d message(s), %d tool(s)",
			req.Model, len(req.Messages), len(req.Tools))
	}
	loop.OnError = func(err error, attempt int) {
		// Failed attempts must be visible: silent retries look like the
		// prompt was ignored.
		this.clearThinking()
		total := loop.Retries + 1
		if attempt < total {
			fmt.Printf("(attempt %d of %d failed: %v; retrying)\n", attempt, total, err)
			this.showThinking()
		} else {
			fmt.Printf("(attempt %d of %d failed: %v)\n", attempt, total, err)
		}
	}
	this.loop = loop

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
	ts := tools.AllTools()
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
