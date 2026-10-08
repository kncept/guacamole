package app

import (
	"fyne.io/fyne/v2"
	fyneapp "fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/kncept/guacamole/ai"
	"github.com/kncept/guacamole/config"
	"github.com/kncept/guacamole/roles"
	"github.com/kncept/guacamole/roles/definitions"
)

// chat represents a single chat session with its own output log and input field.
type chat struct {
	id        string
	log       *fyne.Container // VBox of message bubbles, newest at the bottom
	logScroll *container.Scroll
	input     *widget.Entry
	session   *ai.Session
}

// Guac holds the state and references for the Guacamole GUI application.
type Guac struct {
	a         fyne.App
	w         fyne.Window
	tabs      *container.DocTabs
	newTab    *container.TabItem
	prefsTab  *container.TabItem // pinned leftmost; swaps to the Preferences screen
	sessions  []*sessionTab
	activeIdx int
	count     int // monotonic counter for naming sessions
	models    []config.ModelOption
	roles     []definitions.Role
	config    *config.GConfig
	prefs     *preferencesScreen // in-window Preferences screen, lazily created
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

	// "Preferences" tab — pinned leftmost. Clicking it swaps the window
	// content to the Preferences screen instead of opening a session tab.
	g.prefsTab = container.NewTabItemWithIcon("", theme.SettingsIcon(), widget.NewLabel("Preferences"))
	g.tabs.SetItems(append([]*container.TabItem{g.prefsTab}, g.tabs.Items...))

	// DocTabs opens on index 0 by default, so without this the application
	// would start on the pinned Preferences tab. Select the first session
	// instead.
	g.tabs.Select(g.sessions[g.activeIdx].tab)

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

// backToSessions returns from a full-window screen (the Preferences screen) to
// the session tabs, highlighting the active session. Called by that screen's
// Back button.
func (g *Guac) backToSessions() {
	g.w.SetContent(g.tabs)
	if len(g.sessions) > 0 {
		if st := g.sessions[g.activeIdx]; st.tab != nil {
			// Select re-fires OnSelected (which picks activeIdx and
			// refreshes), and also moves the highlight off the pinned
			// Preferences tab so it can be clicked again.
			g.tabs.Select(st.tab)
		}
	}
	g.refreshContent()
}

// updateTab creates/refreshes the tab item for the session and appends it to
// its container. Each tab gets its own border: the banner on top, the
// scrollable log in the middle, and the session's input box at the bottom.
func (g *Guac) updateTab(st *sessionTab) {
	content := container.NewBorder(st.banner.box, st.inputBoxArea, nil, nil, st.chat.logScroll)
	if st.tab == nil {
		st.tab = container.NewTabItem(st.chat.id, content)
		st.tabs.Append(st.tab)
	} else {
		st.tab.Content = content
		content.Refresh()
	}
}

// onTabClosed handles the X button on a tab.
func (g *Guac) onTabClosed(item *container.TabItem) {
	if item == g.prefsTab || item.Text == "New Session" {
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
		// Rebuild the strip as [Preferences, <the one session>, New Session]:
		// the new session tab must sit before "New Session", and the
		// pinned Preferences tab must stay leftmost.
		items := make([]*container.TabItem, 0, 3)
		if g.prefsTab != nil {
			items = append(items, g.prefsTab)
		}
		items = append(items, st.tab, g.newTab)
		g.tabs.SetItems(items)
	}
	if g.activeIdx >= len(g.sessions) {
		g.activeIdx = len(g.sessions) - 1
	}
	if g.activeIdx < 0 {
		g.activeIdx = 0
	}
	g.tabs.Select(g.sessions[g.activeIdx].tab)
	g.refreshContent()
}

// onTabSelected handles switching between tabs.
func (g *Guac) onTabSelected(selected *container.TabItem) {
	if selected == g.prefsTab {
		g.showPreferences()
		return
	}
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

	preferencesItem := fyne.NewMenuItem("Preferences", func() {
		g.showPreferences()
	})

	fileMenu := fyne.NewMenu("File",
		newTabItem,
		fyne.NewMenuItemSeparator(),
		preferencesItem,
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Quit", func() {
			g.a.Quit()
		}),
	)
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
