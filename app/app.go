package app

import (
	"fmt"

	"fyne.io/fyne/v2"
	fyneapp "fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/kncept/guacamole/ai"
	"github.com/kncept/guacamole/config"
	"github.com/kncept/guacamole/modelclient"
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
	// activeSpinner is the spinner-and-cancel row shown while a request is in
	// flight, or nil when no request is running.
	activeSpinner *spinnerWithCancel
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
	g.reloadModels()
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
	// Record the starting session's model as the stored default (a no-op when
	// it already matches, so nothing is written unless it changed).
	g.syncSelectedModel()

	g.buildMenu()
	g.refreshContent()
	g.w.Resize(fyne.NewSize(600, 400))
	return g
}

// Run shows the window and starts the event loop.
func (g *Guac) Run() {
	g.w.ShowAndRun()
}

// reloadModels rebuilds the available model list from the configured providers
// in the guac config. The "Provider / Model" dropdown is sourced from these
// providers, with the virtual "Magi" model leading the list.
//
// It also refreshes the dropdown in every open session, so a provider or model
// added in Preferences appears immediately without restarting the app.
func (g *Guac) reloadModels() {
	if g.config != nil {
		g.models = config.AllSelectableModelOptions(g.config)
	}
	g.refreshModelSelectors()
}

// realModelOptions returns the available models excluding the virtual Magi
// model. It backs the per-magi dropdowns in Preferences, which may only pick
// real models.
func (g *Guac) realModelOptions() []config.ModelOption {
	opts := make([]config.ModelOption, 0, len(g.models))
	for _, m := range g.models {
		if !m.IsMagi() {
			opts = append(opts, m)
		}
	}
	return opts
}

// modelOptionForLabel finds the real model whose label matches. The virtual
// Magi model never matches, so a magi can never be assigned the Magi model.
func (g *Guac) modelOptionForLabel(label string) (config.ModelOption, bool) {
	for _, m := range g.models {
		if !m.IsMagi() && m.Label() == label {
			return m, true
		}
	}
	return config.ModelOption{}, false
}

// refreshModelSelectors updates every open session's "Provider / Model"
// dropdown to match g.models. Each session keeps its current selection when
// that model is still available; otherwise it falls back to the stored
// default model (or the first model, and clears the selection when no models
// are configured).
func (g *Guac) refreshModelSelectors() {
	labels := labelsForModels(g.models)
	for _, st := range g.sessions {
		if st.banner == nil || st.banner.modelSel == nil {
			continue
		}
		sel := st.banner.modelSel
		selected := sel.Selected
		sel.Options = labels
		if selected == "" || !containsLabel(labels, selected) {
			selected = g.defaultModelLabel()
		}
		if selected == "" {
			sel.ClearSelected()
		} else {
			sel.SetSelected(selected)
		}
		sel.Refresh()
	}
}

// containsLabel reports whether labels contains want.
func containsLabel(labels []string, want string) bool {
	for _, l := range labels {
		if l == want {
			return true
		}
	}
	return false
}

// defaultModelLabel returns the model new sessions should start on: the model
// stored in the config when it is still available, otherwise the first real
// (non-Magi) available model. An unset or stale setting is not an error; it
// simply falls back to the first entry. Magi Mode is never the default unless
// it was explicitly stored *and* the panel is fully configured: a broken
// setting must not leave a new session unable to send.
func (g *Guac) defaultModelLabel() string {
	if g.config != nil && g.config.SelectedModel != "" {
		if g.config.SelectedModel == config.MagiModel {
			if g.magiConfigured() {
				return config.MagiModel
			}
		} else if containsLabel(labelsForModels(g.models), g.config.SelectedModel) {
			return g.config.SelectedModel
		}
	}
	return g.firstRealModelLabel()
}

// firstRealModelLabel returns the label of the first available real model, or
// "" when there are none.
func (g *Guac) firstRealModelLabel() string {
	for _, m := range g.models {
		if !m.IsMagi() {
			return m.Label()
		}
	}
	return ""
}

// magiConfigured reports whether the Magi panel can actually be built: every
// magi has a model assigned and each assignment still resolves to an available
// real model.
func (g *Guac) magiConfigured() bool {
	if g.config == nil || !g.config.Magi.Assigned() {
		return false
	}
	for _, name := range config.MagiNames() {
		if _, ok := g.modelOptionForLabel(g.config.Magi.Model(name)); !ok {
			return false
		}
	}
	return true
}

// activeSession returns the session highlighted in the tab strip, or nil when
// there is none.
func (g *Guac) activeSession() *sessionTab {
	if g.activeIdx < 0 || g.activeIdx >= len(g.sessions) {
		return nil
	}
	return g.sessions[g.activeIdx]
}

// persistSelectedModel stores label as the default model for new sessions,
// writing the config to disk only when the stored value actually changes.
func (g *Guac) persistSelectedModel(label string) {
	if g.config == nil {
		return
	}
	if g.config.SetSelectedModel(label) {
		_ = g.config.Save()
	}
}

// syncSelectedModel makes the stored default match the active session's model,
// so the app reopens on the model the user was last working with.
func (g *Guac) syncSelectedModel() {
	if st := g.activeSession(); st != nil && st.banner != nil && st.banner.modelSel != nil {
		g.persistSelectedModel(st.banner.modelSel.Selected)
	}
}

func (g *Guac) onModelChanged(st *sessionTab, label string) {
	for _, m := range g.models {
		if m.Label() != label {
			continue
		}
		sess := st.session()
		if m.IsMagi() {
			panel, err := g.magiPanelFor(st)
			if err != nil {
				// The panel cannot run without a model for every magi;
				// tell the user and fall back to a real model.
				dialog.ShowError(err, g.w)
				if fallback := g.firstRealModelLabel(); fallback != "" {
					st.banner.modelSel.SetSelected(fallback)
				}
				return
			}
			sess.Magi = panel
			sess.Model = config.MagiModel
		} else {
			loop := ai.NewLoop(st.loopProvider())
			loop.Stream = true
			sess.Loop = loop
			sess.Model = m.ModelID
			sess.Magi = nil
		}
		// Only the session the user is looking at should set the stored
		// default; background selector refreshes must not overwrite it.
		if st == g.activeSession() {
			g.persistSelectedModel(label)
		}
		return
	}
}

// magiPanelFor builds a Magi panel for st and wires its phase lines into st's
// log, so the user can watch the panel at work.
func (g *Guac) magiPanelFor(st *sessionTab) (*ai.Magi, error) {
	panel, err := g.buildMagiPanel()
	if err != nil {
		return nil, err
	}
	panel.OnPhase = func(line string) {
		// OnPhase runs on the panel's phase goroutines; the log is UI state.
		fyne.Do(func() {
			if st != nil && st.chat != nil {
				st.chat.addToolCall("Magi: " + line)
			}
		})
	}
	return panel, nil
}

// buildMagiPanel builds the three-magi panel from the models assigned in
// Preferences → Magi. It fails when a magi has no model assigned or when an
// assigned model is no longer configured, so the user gets a specific message
// instead of a mysterious turn failure.
func (g *Guac) buildMagiPanel() (*ai.Magi, error) {
	if g.config == nil {
		return nil, fmt.Errorf("magi: no configuration available")
	}
	panel := &ai.Magi{}
	for _, name := range config.MagiNames() {
		label := g.config.Magi.Model(name)
		if label == "" {
			return nil, fmt.Errorf("magi: %s has no model assigned — assign one in Preferences → Magi", name)
		}
		opt, ok := g.modelOptionForLabel(label)
		if !ok {
			return nil, fmt.Errorf("magi: %s's model %q is no longer available — update it in Preferences → Magi", name, label)
		}
		provider, err := modelclient.New(&config.ApiModelInterfaceDetails{
			APIType:        opt.ProviderType,
			BaseUrl:        opt.BaseURL,
			ApiKey:         opt.APIKey,
			ModelName:      opt.ModelID,
			AppAttribution: opt.AppAttribution,
		})
		if err != nil {
			return nil, fmt.Errorf("magi: %s: %w", name, err)
		}
		loop := ai.NewLoop(provider)
		// The magi do not stream: each returns a whole response, and the
		// session emits only the winning one.
		loop.Stream = false
		panel.Members = append(panel.Members, ai.MagiMember{Name: name, Loop: loop, Model: opt.ModelID})
	}
	return panel, nil
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
	// A Magi panel caches its member models, so a change made in
	// Preferences → Magi needs the panels rebuilt on the way back.
	g.refreshMagiPanels()
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

// refreshMagiPanels rebuilds the panel of every session currently in Magi Mode
// so model changes made in Preferences take effect immediately. A panel that
// can no longer be built (an assignment was cleared) is left as it was, so a
// running session is never broken by an unrelated edit.
func (g *Guac) refreshMagiPanels() {
	for _, st := range g.sessions {
		if st.chat == nil || st.chat.session == nil || st.chat.session.Magi == nil {
			continue
		}
		if panel, err := g.magiPanelFor(st); err == nil {
			st.chat.session.Magi = panel
		}
	}
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
		g.syncSelectedModel()
		g.refreshContent()
		return
	}
	for i, st := range g.sessions {
		if st.tab == selected {
			g.activeIdx = i
			g.syncSelectedModel()
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
		g.syncSelectedModel()
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
