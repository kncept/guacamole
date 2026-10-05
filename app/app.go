package app

import (
	"context"
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	fyneapp "fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/kncept/guacamole/ai"
	"github.com/kncept/guacamole/config"
	"github.com/kncept/guacamole/restfulai"
)

// chat represents a single chat session with its own output log and input field.
type chat struct {
	id       string
	output   *widget.Label
	input    *widget.Entry
	messages []string
	session  *ai.Session
}

// sessionTab bundles a chat session with its tab item and the tab container
// it lives in, so a session always knows where it is displayed.
type sessionTab struct {
	chat     *chat
	tab      *container.TabItem
	tabs     *container.DocTabs
	guac     *Guac
	banner   fyne.CanvasObject
	modelSel *widget.Select
}

// Guac holds the state and references for the Guacamole GUI application.
type Guac struct {
	a         fyne.App
	w         fyne.Window
	tabs      *container.DocTabs
	newTab    *container.TabItem
	sessions  []*sessionTab
	activeIdx int
	count     int // monotonic counter for naming sessions
	models    []config.ModelOption
}

// New creates and initializes a new Guac GUI application.
func New() *Guac {
	a := fyneapp.New()
	w := a.NewWindow("Guacamole GUI")

	g := &Guac{a: a, w: w, count: 0}
	g.models, _ = config.AllModelOptions()
	g.tabs = container.NewDocTabs()
	g.tabs.CloseIntercept = g.onTabClosed
	g.tabs.OnSelected = g.onTabSelected

	g.sessions = append(g.sessions, g.newSessionTab())
	g.activeIdx = 0
	g.updateTab(g.sessions[g.activeIdx])

	// "New Session" tab — always rightmost, creates a new session when clicked.
	g.newTab = container.NewTabItem("New Session", widget.NewLabel("Click to create new session"))
	g.tabs.Append(g.newTab)

	g.buildMenu()
	g.refreshContent()
	g.w.Resize(fyne.NewSize(600, 400))
	return g
}

// Run shows the window and starts the event loop.
func (g *Guac) Run() {
	g.w.ShowAndRun()
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
	st.session() // create ai.Session with the selected model
	st.banner = container.NewHBox(widget.NewLabel("Provider / Model:"), st.modelSel)
	return st
}

func (g *Guac) modelLabels() []string {
	labels := make([]string, 0, len(g.models))
	for _, m := range g.models {
		labels = append(labels, m.Label())
	}
	return labels
}

// session returns the session's ai.Session, creating it if needed.
func (st *sessionTab) session() *ai.Session {
	if st.chat.session == nil {
		loop := ai.NewLoop(st.loopProvider())
		loop.Stream = true
		sess := ai.NewSession(loop)
		sess.Model = st.selectedModel().ModelID
		st.chat.session = sess
	}
	return st.chat.session
}

// loopProvider builds the REST provider for the session's selected model.
func (st *sessionTab) loopProvider() ai.Provider {
	opt := st.selectedModel()
	return restfulai.NewRestfulAI(&config.ApiModelInterfaceDetails{
		BaseUrl:   opt.BaseURL,
		ApiKey:    opt.APIKey,
		ModelName: opt.ModelID,
	})
}

func (st *sessionTab) selectedModel() config.ModelOption {
	for _, m := range st.guac.models {
		if m.Label() == st.modelSel.Selected {
			return m
		}
	}
	return config.ModelOption{}
}

func (g *Guac) onModelChanged(st *sessionTab, label string) {
	for _, m := range g.models {
		if m.Label() == label {
			sess := st.session()
			loop := ai.NewLoop(st.loopProvider())
			loop.Stream = true
			sess.Loop = loop
			sess.Model = m.ModelID
			return
		}
	}
}

// refreshContent rebuilds the window content with the active session's input box.
func (g *Guac) refreshContent() {
	inputBox := g.createInputBox(g.sessions[g.activeIdx].chat.input)
	g.w.SetContent(container.NewBorder(nil, inputBox, nil, nil, g.tabs))
	g.w.Resize(fyne.NewSize(600, 400))
}

func (g *Guac) createInputBox(activeInput *widget.Entry) *fyne.Container {
	submitBtn := widget.NewButton("Submit", func() {
		text := activeInput.Text
		if strings.TrimSpace(text) == "" {
			return
		}
		g.runPrompt(activeInput, text)
	})

	return container.NewBorder(nil, nil, nil, submitBtn, activeInput)
}

// runPrompt sends the prompt through the active session's AI loop, streaming
// the reply into the session log.
func (g *Guac) runPrompt(activeInput *widget.Entry, text string) {
	st := g.sessions[g.activeIdx]
	st.chat.messages = append(st.chat.messages, "You: "+text)
	st.chat.output.SetText(strings.Join(st.chat.messages, "\n"))
	activeInput.SetText("")

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

// updateTab creates/refreshes the tab item for the session and appends it to its container.
func (g *Guac) updateTab(st *sessionTab) {
	st.chat.output.SetText(strings.Join(st.chat.messages, "\n"))
	log := container.NewBorder(st.banner, nil, nil, nil, container.NewVScroll(st.chat.output))
	st.tab = container.NewTabItem(st.chat.id, log)
	st.tabs.Append(st.tab)
}

// onTabClosed handles the X button on a tab.
func (g *Guac) onTabClosed(item *container.TabItem) {
	if item.Text == "New Session" {
		return
	}
	sessIdx := -1
	for i, st := range g.sessions {
		if st.tab == item {
			sessIdx = i
			break
		}
	}
	g.tabs.Remove(item)
	if sessIdx >= 0 {
		g.sessions = append(g.sessions[:sessIdx], g.sessions[sessIdx+1:]...)
	}
	// Keep at least one session
	if len(g.sessions) == 0 {
		st := g.newSessionTab()
		g.sessions = append(g.sessions, st)
		g.updateTab(st)
		// Move the new tab before the "New Session" tab
		newItem := g.tabs.Items[len(g.tabs.Items)-1]
		rest := g.tabs.Items[:len(g.tabs.Items)-1]
		g.tabs.SetItems(append([]*container.TabItem{newItem}, rest...))
	}
	if g.activeIdx >= len(g.sessions) {
		g.activeIdx = len(g.sessions) - 1
	}
	if g.activeIdx < 0 {
		g.activeIdx = 0
	}
	g.tabs.SelectIndex(g.activeIdx)
	g.refreshContent()
}

// onTabSelected handles switching between tabs.
func (g *Guac) onTabSelected(selected *container.TabItem) {
	if selected.Text == "New Session" {
		st := g.newSessionTab()
		g.sessions = append(g.sessions, st)
		g.activeIdx = len(g.sessions) - 1
		g.updateTab(st)
		// Remove and re-append "New Session" to keep it rightmost
		g.tabs.Remove(g.newTab)
		g.tabs.Append(g.newTab)
		g.tabs.SelectIndex(len(g.tabs.Items) - 2)
		g.refreshContent()
		return
	}
	for i, st := range g.sessions {
		if st.tab == selected {
			g.activeIdx = i
			g.refreshContent()
			return
		}
	}
}

func (g *Guac) buildMenu() {
	newTabItem := fyne.NewMenuItem("New Tab", func() {
		st := g.newSessionTab()
		g.sessions = append(g.sessions, st)
		g.updateTab(st)
		g.activeIdx = len(g.sessions) - 1
		// Keep the "New Session" tab rightmost, then select the new tab
		g.tabs.Remove(g.newTab)
		g.tabs.Append(g.newTab)
		g.tabs.SelectIndex(len(g.tabs.Items) - 2)
		g.refreshContent()
	})

	fileMenu := fyne.NewMenu("File", newTabItem)
	helpMenu := fyne.NewMenu("About",
		fyne.NewMenuItem("About", func() {
			g.w.SetContent(container.NewVBox(
				widget.NewLabel("Guacamole GUI"),
				widget.NewLabel("A simple Fyne GUI with tabbed sessions."),
			))
		}),
	)
	g.w.SetMainMenu(fyne.NewMainMenu(fileMenu, helpMenu))
}
