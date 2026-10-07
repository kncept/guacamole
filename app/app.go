package app

import (
	"strings"

	"fyne.io/fyne/v2"
	fyneapp "fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/kncept/guacamole/ai"
	"github.com/kncept/guacamole/config"
	"github.com/kncept/guacamole/roles"
	"github.com/kncept/guacamole/roles/definitions"
)

// chat represents a single chat session with its own output log and input field.
type chat struct {
	id       string
	output   *widget.Label
	input    *widget.Entry
	messages []string
	session  *ai.Session
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
	roles     []definitions.Role
	config    *config.GConfig
	prefs     *preferencesWindow // lazily created Preferences window
}

// New creates and initializes a new Guac GUI application.
func New() *Guac {
	a := fyneapp.New()
	w := a.NewWindow("Guacamole GUI")

	g := &Guac{a: a, w: w, count: 0}
	g.config, _ = config.Load()
	g.models, _ = config.AllModelOptions()
	g.roles = roles.AllRoles()
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

func (g *Guac) onRoleChanged(st *sessionTab, label string) {
	for _, r := range g.roles {
		if r.RoleName == label {
			sess := st.session()
			sess.SystemPrompt = r.RoleSystemPrompt
			return
		}
	}
}

// refreshContent rebuilds the window content around the session tabs. Each tab
// carries its own input box, so only the tabs need to be (re)installed.
func (g *Guac) refreshContent() {
	g.w.SetContent(g.tabs)
	g.w.Resize(fyne.NewSize(600, 400))
}

// updateTab creates/refreshes the tab item for the session and appends it to
// its container. Each tab gets its own border: the banner on top, the
// scrollable output in the middle, and the session's input box at the bottom.
func (g *Guac) updateTab(st *sessionTab) {
	st.chat.output.SetText(strings.Join(st.chat.messages, "\n"))
	content := container.NewBorder(st.banner.box, st.inputBox(), nil, nil, container.NewVScroll(st.chat.output))
	st.tab = container.NewTabItem(st.chat.id, content)
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

	preferencesItem := fyne.NewMenuItem("Preferences...", func() {
		g.showPreferences()
	})

	fileMenu := fyne.NewMenu("File", newTabItem, fyne.NewMenuItemSeparator(), preferencesItem)
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
