package app

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/kncept/guacamole/config"
	"github.com/kncept/guacamole/sessionstore"
)

// preferenceSections are the entries of the Preferences navigation list, in
// display order. Each one names a screen built by screenFor.
var preferenceSections = []string{"Models", "Roles", "Sessions"}

// preferencesWindow is the Preferences window: a list of sections down the
// left side (a vertical tab list) and the selected section's screen on the
// right. It keeps a reference to the Guac it belongs to, so every screen can
// read the application's live models, roles, and saved sessions.
type preferencesWindow struct {
	guac *Guac
	win  fyne.Window

	nav     *widget.List
	body    *fyne.Container
	section int
}

// showPreferences opens the Preferences window, re-raising it if it is
// already open so there is only ever one.
func (g *Guac) showPreferences() {
	if g.prefs == nil {
		g.prefs = newPreferencesWindow(g)
	}
	g.prefs.win.Show()
	g.prefs.win.RequestFocus()
}

func newPreferencesWindow(g *Guac) *preferencesWindow {
	p := &preferencesWindow{guac: g}

	p.nav = widget.NewList(
		func() int { return len(preferenceSections) },
		func() fyne.CanvasObject { return widget.NewLabel("Models") },
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			obj.(*widget.Label).SetText(preferenceSections[id])
		},
	)
	p.nav.OnSelected = func(id widget.ListItemID) { p.showSection(int(id)) }

	p.body = container.NewMax()
	split := container.NewHSplit(p.nav, p.body)
	split.Offset = 0.25

	p.win = g.a.NewWindow("Preferences")
	// Hide instead of close: a closed window's content is freed by Fyne,
	// which would leave the window blank the next time it is opened.
	p.win.SetCloseIntercept(func() { p.win.Hide() })
	p.win.SetContent(split)
	p.win.Resize(fyne.NewSize(720, 480))

	p.nav.Select(0)
	return p
}

// showSection installs the screen for preferenceSections[id] on the right
// hand side of the window.
func (p *preferencesWindow) showSection(id int) {
	p.section = id
	p.body.Objects = []fyne.CanvasObject{p.screenFor(preferenceSections[id])}
	p.body.Refresh()
}

func (p *preferencesWindow) screenFor(name string) fyne.CanvasObject {
	switch name {
	case "Models":
		return p.modelsScreen()
	case "Roles":
		return p.rolesScreen()
	case "Sessions":
		return p.sessionsScreen()
	}
	return widget.NewLabel("Unknown preferences section: " + name)
}

// modelsScreen lists every configured provider with the models it offers,
// grouped under a provider heading.
func (p *preferencesWindow) modelsScreen() fyne.CanvasObject {
	models := p.guac.models
	if len(models) == 0 {
		return paddedLabel("No models found. Add providers to your opencode configuration.")
	}

	rows := make([]fyne.CanvasObject, 0, len(models)*2)
	for i, m := range models {
		// models are sorted by label, so all of a provider's models sit
		// next to each other and a new heading is needed at each change.
		if i == 0 || m.ProviderID != models[i-1].ProviderID {
			if i > 0 {
				rows = append(rows, widget.NewSeparator())
			}
			rows = append(rows, widget.NewLabelWithStyle(providerHeading(m),
				fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
			if m.BaseURL != "" {
				rows = append(rows, widget.NewLabel(m.BaseURL))
			}
		}
		name := m.ModelName
		if m.ModelID != m.ModelName {
			name += " (" + m.ModelID + ")"
		}
		rows = append(rows, widget.NewLabel("- "+name))
	}
	return container.NewVScroll(container.NewVBox(rows...))
}

// providerHeading is the display name of the provider m comes from.
func providerHeading(m config.ModelOption) string {
	if m.Provider != "" {
		return m.Provider
	}
	return m.ProviderID
}

// rolesScreen lists the roles (system prompts). Selecting a role shows its
// system prompt underneath the list.
func (p *preferencesWindow) rolesScreen() fyne.CanvasObject {
	roles := p.guac.roles
	if len(roles) == 0 {
		return paddedLabel("No roles found.")
	}

	prompt := container.NewVBox()
	showPrompt := func(id widget.ListItemID) {
		r := roles[id]
		prompt.Objects = []fyne.CanvasObject{
			widget.NewLabelWithStyle(r.RoleName, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			widget.NewLabel(r.RoleSystemPrompt),
		}
		prompt.Refresh()
	}

	list := widget.NewList(
		func() int { return len(roles) },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			r := roles[id]
			label := r.RoleName
			if r.RoleSetName != "" {
				label += "  (" + r.RoleSetName + ")"
			}
			obj.(*widget.Label).SetText(label)
		},
	)
	list.OnSelected = showPrompt
	list.Select(0)

	split := container.NewVSplit(list, container.NewVScroll(prompt))
	split.Offset = 0.4
	return split
}

// sessionsScreen lists the sessions saved on disk, each with a delete
// button.
func (p *preferencesWindow) sessionsScreen() fyne.CanvasObject {
	dir := p.guac.config.SessionsDir
	ids, err := sessionstore.List(dir)
	if err != nil {
		return paddedLabel("Could not list saved sessions: " + err.Error())
	}
	if len(ids) == 0 {
		return paddedLabel("No saved sessions in " + dir)
	}

	rows := make([]fyne.CanvasObject, 0, len(ids))
	for _, id := range ids {
		rows = append(rows, p.sessionRow(dir, id))
	}
	return container.NewVScroll(container.NewVBox(rows...))
}

// sessionRow is one saved session: its ID with a delete button on the right.
func (p *preferencesWindow) sessionRow(dir, id string) fyne.CanvasObject {
	return container.NewBorder(nil, nil, nil,
		widget.NewButton("Delete", func() { p.deleteSession(dir, id) }),
		widget.NewLabel(id))
}

// deleteSession removes the saved session and rebuilds the section so the
// list no longer shows it.
func (p *preferencesWindow) deleteSession(dir, id string) {
	if err := sessionstore.Delete(dir, id); err != nil {
		dialog.ShowError(err, p.win)
		return
	}
	p.showSection(p.section)
}

func paddedLabel(text string) fyne.CanvasObject {
	return container.NewVBox(widget.NewLabel(text))
}
