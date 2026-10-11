package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/kncept/guacamole/ai"
	"github.com/kncept/guacamole/config"
	"github.com/kncept/guacamole/modelclient"
	"github.com/kncept/guacamole/permissions"
	"github.com/kncept/guacamole/roles/definitions"
	"github.com/kncept/guacamole/tools"
	"github.com/kncept/guacamole/userinput"
)

// sessionTab bundles a chat session with its tab item and the tab container
// it lives in, so a session always knows where it is displayed.
type sessionTab struct {
	chat         *chat
	tab          *container.TabItem
	tabs         *container.DocTabs
	guac         *Guac
	banner       *sessionBanner
	inputBoxArea *fyne.Container
}

// sessionBanner is the row of role and model selectors shown at the top of a
// session tab. It owns the selector widgets and the container that lays them
// out, so a session always knows which role and model are active.
type sessionBanner struct {
	modelSel *widget.Select
	roleSel  *widget.Select
	box      *fyne.Container
}

// newSessionBanner builds a banner from the available models and roles, wiring
// each selector to the provided change handlers (called with the selected
// label). Callers must attach the banner to their sessionTab before selecting
// a default, because selecting fires the change handlers.
func newSessionBanner(models []config.ModelOption, roles []definitions.Role, onModelChanged, onRoleChanged func(string)) *sessionBanner {
	b := &sessionBanner{}

	b.modelSel = widget.NewSelect(labelsForModels(models), onModelChanged)
	b.roleSel = widget.NewSelect(labelsForRoles(roles), onRoleChanged)

	// Provider / Model banner, with role selector to the left of it
	b.box = container.NewHBox(
		widget.NewLabel("Current role:"), b.roleSel,
		widget.NewLabel("Provider / Model:"), b.modelSel,
	)
	return b
}

func labelsForModels(models []config.ModelOption) []string {
	labels := make([]string, 0, len(models))
	for _, m := range models {
		labels = append(labels, m.Label())
	}
	return labels
}

func labelsForRoles(roles []definitions.Role) []string {
	labels := make([]string, 0, len(roles))
	for _, r := range roles {
		labels = append(labels, r.RoleName)
	}
	return labels
}

func (g *Guac) newSessionTab() *sessionTab {
	g.count++
	log := container.NewVBox()
	c := &chat{
		id:        fmt.Sprintf("Session %d", g.count),
		log:       log,
		logScroll: container.NewVScroll(log),
		input:     widget.NewEntry(),
	}
	st := &sessionTab{chat: c, tabs: g.tabs, guac: g}
	st.inputBoxArea = st.inputBox()
	// The banner's selectors must not pick defaults until it is attached to
	// st: SetSelected fires the change callbacks, which read st.banner.
	st.banner = newSessionBanner(g.models, g.roles,
		func(label string) { g.onModelChanged(st, label) },
		func(label string) { g.onRoleChanged(st, label) },
	)
	if label := g.defaultModelLabel(); label != "" {
		st.banner.modelSel.SetSelected(label)
	}
	if len(g.roles) > 0 {
		st.banner.roleSel.SetSelected(g.roles[0].RoleName)
	}
	st.session() // create ai.Session with the selected model
	return st
}

// session returns the session's ai.Session, creating it if needed.
func (st *sessionTab) session() *ai.Session {
	if st.chat.session == nil {
		loop := ai.NewLoop(st.loopProvider())
		loop.Stream = true
		sess := ai.NewSession(loop)
		sess.Model = st.selectedModel().ModelID
		userInput := &guiUserInput{tab: st}
		sess.UserInput = userInput
		manager := permissions.NewPermissionsManager(st.guac.config, userInput)
		sess.Tools = st.tracedTools(tools.AllTools(manager, userInput))
		st.chat.session = sess
	}
	return st.chat.session
}

// tracedTools wraps every tool so its use is shown in the session's chat log
// as a line like "→ ls /tmp": tool activity must be visible, otherwise a
// tool round looks like a frozen app. The line is added when the call
// starts; if the call fails, the error is shown on the same line. Handlers
// run in the session's goroutine (never the UI thread), so log updates are
// marshalled onto the UI thread with DoAndWait: the label must be back in
// hand before the call runs, and fyne.Do alone would not wait.
func (st *sessionTab) tracedTools(ts []ai.Tool) []ai.Tool {
	for i := range ts {
		name := ts[i].Name
		handler := ts[i].Handler
		ts[i].Handler = func(ctx context.Context, args json.RawMessage) (string, error) {
			summary := tools.Summarize(name, args)
			var label *widget.Label
			fyne.DoAndWait(func() { label = st.chat.addToolCall(summary) })
			out, err := handler(ctx, args)
			if err != nil {
				fyne.DoAndWait(func() { st.chat.markToolError(label, summary, err) })
			}
			return out, err
		}
	}
	return ts
}

// loopProvider builds the provider for the session's selected model,
// dispatching on the model's API type.
func (st *sessionTab) loopProvider() ai.Provider {
	opt := st.selectedModel()
	provider, err := modelclient.New(&config.ApiModelInterfaceDetails{
		APIType:   opt.ProviderType,
		BaseUrl:   opt.BaseURL,
		ApiKey:    opt.APIKey,
		ModelName: opt.ModelID,
	})
	if err != nil {
		// The GUI keeps running: prompts with this model surface the error
		// in the chat instead of crashing the app.
		log.Printf("app: %v", err)
		return errorProvider{err: err}
	}
	return provider
}

// errorProvider is an ai.Provider that fails every exchange with err, used
// when a selected model has no usable connection details.
type errorProvider struct {
	err error
}

func (e errorProvider) Send(ctx context.Context, req ai.Request) (ai.Response, error) {
	return ai.Response{}, e.err
}

func (e errorProvider) SendStream(ctx context.Context, req ai.Request, onChunk func(ai.Chunk) error) (ai.Response, error) {
	return ai.Response{}, e.err
}

func (st *sessionTab) selectedModel() config.ModelOption {
	for _, m := range st.guac.models {
		if m.Label() == st.banner.modelSel.Selected {
			return m
		}
	}
	return config.ModelOption{}
}

// guiUserInput asks the user a question by showing the options as buttons in
// place of the session's input box. The caller is blocked until the user
// answers, then the input box is restored. Prompt runs in the session's
// goroutine (never the UI thread), so the UI updates are marshalled with
// DoAndWait: the prompt must be shown before the wait for the answer, and the
// restore must be done before the next tool call reads inputBoxArea. It
// serves every foreground user prompt — permission checks, the model's
// user_question tool, and tool-round-limit decisions.
type guiUserInput struct {
	tab *sessionTab
}

var _ userinput.RequestForegroundUserInput = (*guiUserInput)(nil)

func (g *guiUserInput) Prompt(text string, allowFree bool, options []string) (string, error) {
	if g.tab == nil {
		return "", fmt.Errorf("no session tab to ask the question in")
	}
	answer := make(chan string, 1)
	fmt.Printf("[UserInput] %s\n", text)

	original := g.tab.inputBoxArea
	fyne.DoAndWait(func() {
		row := container.NewVBox(widget.NewLabel(text))
		buttons := container.NewHBox()
		for _, option := range options {
			// Late clicks after an answer was already given must not
			// block the UI, so extra sends are dropped.
			choice := option
			buttons.Add(widget.NewButton(choice, func() {
				select {
				case answer <- choice:
				default:
				}
			}))
		}
		row.Add(buttons)
		if allowFree {
			entry := widget.NewEntry()
			entry.OnSubmitted = func(text string) {
				text = strings.TrimSpace(text)
				if text == "" {
					return
				}
				select {
				case answer <- text:
				default:
				}
			}
			row.Add(entry)
		}
		g.tab.inputBoxArea = row
		g.tab.updateInput()
	})
	a := <-answer
	fyne.DoAndWait(func() {
		g.tab.inputBoxArea = original
		g.tab.updateInput()
	})
	return a, nil
}

// updateInput rebuilds the tab so the new inputBoxArea is displayed.
func (st *sessionTab) updateInput() {
	st.guac.updateTab(st)
}

// inputBox builds the session's input area: a multi-line entry with a submit
// button. It is placed in the bottom border of the session's tab, so each
// session has its own input independent of the others.
func (st *sessionTab) inputBox() *fyne.Container {
	input := st.chat.input
	// Enable multi-line mode so that Enter inserts a newline instead of submitting.
	input.MultiLine = true

	// Submit handler: called from the submit button and keyboard shortcuts.
	submit := func(text string) {
		text = strings.TrimSpace(text)
		if text == "" {
			return
		}
		st.runPrompt(text)
	}

	input.OnSubmitted = submit

	// Submit button for mouse users; pressing it triggers submission.
	submmitBtn := widget.NewButton("Submit", func() {
		submit(input.Text)
	})

	return container.NewBorder(nil, nil, nil, submmitBtn, input)
}

// runPrompt sends the prompt through the session's AI loop, streaming the
// reply into the session log. The spinner at the bottom of the log runs for
// as long as the query is active.
func (st *sessionTab) runPrompt(text string) {
	swc := st.chat.addUserMessage(text)
	st.chat.input.SetText("")

	// Create a cancellable context for this request. Cancelling it aborts
	// the whole turn set: the in-flight model call and any tool rounds.
	ctx, cancel := context.WithCancel(context.Background())

	// The red X next to the spinner aborts the request.
	swc.cancelButton.OnTapped = func() {
		cancel()
	}

	go func() {
		var reply strings.Builder
		sess := st.session()
		loop := sess.Loop
		var aiLabel *widget.Label
		var inToolRound bool
		loop.OnChunk = func(chunk ai.Chunk) error {
			reply.WriteString(chunk.Delta)
			fyne.Do(func() {
				// Trim leading/trailing newlines from the accumulated response
				displayText := strings.Trim(reply.String(), "\n")
				// A response that is entirely empty (only whitespace) gets no
				// bubble at all: skip until there is something to show.
				if strings.TrimSpace(displayText) == "" {
					return
				}
				// If we're starting a new response after a tool round, create a new bubble
				if inToolRound {
					aiLabel = st.chat.addAIMessage()
					inToolRound = false
				}
				if aiLabel == nil {
					aiLabel = st.chat.addAIMessage()
				}
				aiLabel.SetText(displayText)
				st.chat.scrollLog()
			})
			return nil
		}
		// Track when we enter a tool round so we can create a fresh bubble after
		originalOnResponse := loop.OnResponse
		loop.OnResponse = func(resp ai.Response) {
			// Check if this response contains tool calls
			hasTools := false
			for _, msg := range resp.Messages {
				if msg.Role == ai.RoleAssistant && msg.Name != "" {
					hasTools = true
					break
				}
			}
			if hasTools {
				inToolRound = true
			}
			if originalOnResponse != nil {
				originalOnResponse(resp)
			}
		}
		_, err := sess.Say(ctx, text)
		fyne.Do(func() {
			if err != nil {
				// A cancelled turn is a deliberate stop, not a failure: don't
				// show an error line for it.
				if !errors.Is(err, context.Canceled) {
					st.chat.addErrorMessage(err)
				}
			}
			st.chat.removeSpinner(swc)
			st.chat.scrollLog()
		})
	}()
}
