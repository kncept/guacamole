package app

import (
	"context"
	"fmt"
	"log"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/kncept/guacamole/ai"
	"github.com/kncept/guacamole/config"
	"github.com/kncept/guacamole/restfulai"
	"github.com/kncept/guacamole/roles/definitions"
	"github.com/kncept/guacamole/tools"
)

// sessionTab bundles a chat session with its tab item and the tab container
// it lives in, so a session always knows where it is displayed.
type sessionTab struct {
	chat   *chat
	tab    *container.TabItem
	tabs   *container.DocTabs
	guac   *Guac
	banner *sessionBanner
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
// label).
func newSessionBanner(models []config.ModelOption, roles []definitions.Role, onModelChanged, onRoleChanged func(string)) *sessionBanner {
	b := &sessionBanner{}

	b.modelSel = widget.NewSelect(labelsForModels(models), onModelChanged)
	if len(models) > 0 {
		b.modelSel.SetSelected(models[0].Label())
	}
	b.roleSel = widget.NewSelect(labelsForRoles(roles), onRoleChanged)
	if len(roles) > 0 {
		b.roleSel.SetSelected(roles[0].RoleName)
	}

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
	c := &chat{
		id:     fmt.Sprintf("Session %d", g.count),
		output: widget.NewLabel(""),
		input:  widget.NewEntry(),
	}
	st := &sessionTab{chat: c, tabs: g.tabs, guac: g}
	st.banner = newSessionBanner(g.models, g.roles,
		func(label string) { g.onModelChanged(st, label) },
		func(label string) { g.onRoleChanged(st, label) },
	)
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
		sess.Tools = tools.AllTools()
		st.chat.session = sess
	}
	return st.chat.session
}

// loopProvider builds the REST provider for the session's selected model.
func (st *sessionTab) loopProvider() ai.Provider {
	opt := st.selectedModel()
	provider, err := restfulai.NewRestfulAI(&config.ApiModelInterfaceDetails{
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
// reply into the session log.
func (st *sessionTab) runPrompt(text string) {
	st.chat.messages = append(st.chat.messages, "You: "+text)
	st.chat.output.SetText(strings.Join(st.chat.messages, "\n"))
	st.chat.input.SetText("")

	go func() {
		var reply strings.Builder
		sess := st.session()
		loop := sess.Loop
		loop.OnChunk = func(chunk ai.Chunk) error {
			reply.WriteString(chunk.Delta)
			fyne.Do(func() {
				st.chat.output.SetText(strings.Join(st.chat.messages, "\n") + "\nAI: " + reply.String())
			})
			return nil
		}
		_, err := sess.Say(context.Background(), text)
		fyne.Do(func() {
			if err != nil {
				st.chat.messages = append(st.chat.messages, "Error: "+err.Error())
			} else {
				st.chat.messages = append(st.chat.messages, "AI: "+reply.String())
			}
			st.chat.output.SetText(strings.Join(st.chat.messages, "\n"))
		})
	}()
}
