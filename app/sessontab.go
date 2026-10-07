package app

import (
	"context"
	"fmt"
	"log"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/kncept/guacamole/ai"
	"github.com/kncept/guacamole/config"
	"github.com/kncept/guacamole/restfulai"
	"github.com/kncept/guacamole/tools"
)

// sessionTab bundles a chat session with its tab item and the tab container
// it lives in, so a session always knows where it is displayed.
type sessionTab struct {
	chat     *chat
	tab      *container.TabItem
	tabs     *container.DocTabs
	guac     *Guac
	banner   fyne.CanvasObject
	modelSel *widget.Select
	roleSel  *widget.Select
}

func (g *Guac) newSessionTab() *sessionTab {
	g.count++
	c := &chat{
		id:     fmt.Sprintf("Session %d", g.count),
		output: widget.NewLabel(""),
		input:  widget.NewEntry(),
	}
	st := &sessionTab{chat: c, tabs: g.tabs, guac: g}
	st.modelSel = widget.NewSelect(g.modelLabels(), func(selected string) {
		g.onModelChanged(st, selected)
	})
	if len(g.models) > 0 {
		st.modelSel.SetSelected(g.models[0].Label())
	}
	st.roleSel = widget.NewSelect(g.roleLabels(), func(selected string) {
		g.onRoleChanged(st, selected)
	})
	if len(g.roles) > 0 {
		st.roleSel.SetSelected(g.roles[0].RoleName)
	}
	st.session() // create ai.Session with the selected model
	// Provider / Model banner, with role selector above it
	st.banner = container.NewHBox(
		widget.NewLabel("Current role:"), st.roleSel,
		widget.NewLabel("Provider / Model:"), st.modelSel,
	)
	return st
}

// session returns the session's ai.Session, creating it if needed.
func (st *sessionTab) session() *ai.Session {
	if st.chat.session == nil {
		loop := ai.NewLoop(st.loopProvider())
		loop.Stream = true
		sess := ai.NewSession(loop)
		sess.Model = st.selectedModel().ModelID
		sess.Tools = tools.FileSystem() // add default file/tools: read_file, ls, write_file
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
		if m.Label() == st.modelSel.Selected {
			return m
		}
	}
	return config.ModelOption{}
}
