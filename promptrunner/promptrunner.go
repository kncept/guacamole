// Package promptrunner runs prompts through a streaming chat session,
// printing the streamed results and saving the session to disk after each
// turn.
package promptrunner

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/kncept/guacamole/ai"
	"github.com/kncept/guacamole/config"
	"github.com/kncept/guacamole/restfulai"
	"github.com/kncept/guacamole/sessionstore"
)

// SessionsDir is where chat sessions are saved: ./sessions/<session ID>.json.
const SessionsDir = "sessions"

// thinkingMessage is shown while waiting for the model's first output.
const thinkingMessage = "thinking..."

type PromptRunner interface {
	// RunPrompt sends one user message through the session, prints the
	// streamed reply, and saves the session.
	RunPrompt(prompt string) error

	// Reset starts a fresh session with a new ID, discarding the history.
	Reset()

	// SessionID returns the current session's ID.
	SessionID() string

	// Save writes the current session to SessionsDir and returns the path
	// written.
	Save() (string, error)
}

type promptRunner struct {
	loop    *ai.Loop
	model   string
	session *ai.Session

	// interactive reports whether stdin is a terminal. Only interactive
	// runs get the "thinking..." indicator, so piped output stays clean.
	interactive bool
	thinking    bool
}

// NewPromptRunner starts a chat session over conf's provider. If resumeID
// is non-empty, the session with that ID is loaded from SessionsDir and
// continues where it left off.
func NewPromptRunner(conf *config.ApiModelInterfaceDetails, resumeID string) (PromptRunner, error) {
	provider := restfulai.NewRestfulAI(conf)

	this := &promptRunner{
		model:       conf.ModelName,
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

	if resumeID != "" {
		session, err := sessionstore.Load(SessionsDir, resumeID, loop)
		if err != nil {
			return nil, err
		}
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
}

func (this *promptRunner) SessionID() string {
	return this.session.ID
}

func (this *promptRunner) Save() (string, error) {
	return sessionstore.Save(SessionsDir, this.session)
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

// isTerminal reports whether f is a terminal (character device).
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}
