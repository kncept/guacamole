package app

import (
	"sort"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/kncept/guacamole/config"
	"github.com/kncept/guacamole/restfulai"
	"github.com/kncept/guacamole/sessionstore"
)

// preferenceSections are the entries of the Preferences navigation list, in
// display order. Each one names a screen built by screenFor.
var preferenceSections = []string{"Providers", "Roles", "Sessions"}

// preferencesScreen is the in-window Preferences screen: a Back button on
// top, a list of sections down the left side (a vertical tab list) and the
// selected section's screen on the right. It keeps the last viewed section so
// returning to the screen shows it again. It shares the main window with the
// sessions instead of living in its own window, so opening it replaces the
// window content.
type preferencesScreen struct {
	guac *Guac

	nav     *widget.List
	body    *fyne.Container
	section int
}

// showPreferences replaces the window content with the Preferences screen.
// Called from the pinned Preferences tab and from File → Preferences.
func (g *Guac) showPreferences() {
	if g.prefs == nil {
		g.prefs = &preferencesScreen{guac: g}
	}
	g.w.SetContent(g.prefs.content())
	// Select the last viewed section; OnSelected fills in its screen.
	g.prefs.nav.Select(g.prefs.section)
}

// content builds the Preferences screen: a header with a Back button over the
// section list and body.
func (p *preferencesScreen) content() fyne.CanvasObject {
	p.nav = widget.NewList(
		func() int { return len(preferenceSections) },
		func() fyne.CanvasObject { return widget.NewLabel("Providers") },
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			obj.(*widget.Label).SetText(preferenceSections[id])
		},
	)
	p.body = container.NewMax()
	p.nav.OnSelected = func(id widget.ListItemID) { p.showSection(int(id)) }

	split := container.NewHSplit(p.nav, p.body)
	split.Offset = 0.25

	header := container.NewHBox(
		widget.NewButtonWithIcon("Back", theme.NavigateBackIcon(), p.guac.backToSessions),
		widget.NewLabelWithStyle("Preferences", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
	)
	return container.NewBorder(header, nil, nil, nil, split)
}

// showSection installs the screen for preferenceSections[id] on the right
// hand side of the Preferences screen.
func (p *preferencesScreen) showSection(id int) {
	p.section = id
	p.body.Objects = []fyne.CanvasObject{p.screenFor(preferenceSections[id])}
	p.body.Refresh()
}

func (p *preferencesScreen) screenFor(name string) fyne.CanvasObject {
	switch name {
	case "Providers":
		return p.providersScreen()
	case "Roles":
		return p.rolesScreen()
	case "Sessions":
		return p.sessionsScreen()
	}
	return widget.NewLabel("Unknown preferences section: " + name)
}

// providersScreen lists the configured model providers, each with its name,
// type and base URL, and an "Add Provider" button at the bottom.
func (p *preferencesScreen) providersScreen() fyne.CanvasObject {
	providers := p.guac.config.ModelProviders

	rows := make([]fyne.CanvasObject, 0, len(providers)*3+2)
	if len(providers) == 0 {
		rows = append(rows, widget.NewLabel("No providers configured. Add one to get started."))
	}
	for i, prov := range providers {
		idx := i
		heading := prov.Name
		if prov.Type != "" {
			heading = heading + " (" + string(prov.Type) + ")"
		}
		rows = append(rows, widget.NewLabelWithStyle(heading,
			fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
		if prov.BaseURL != "" {
			rows = append(rows, widget.NewLabel(prov.BaseURL))
		}
		rows = append(rows, container.NewHBox(
			widget.NewButton("Edit", func() { p.showEditProvider(idx) }),
			widget.NewButton("Models", func() { p.showProviderModels(idx) }),
		))
		rows = append(rows, widget.NewSeparator())
	}
	rows = append(rows, widget.NewButton("Add Provider", func() {
		p.showAddProvider()
	}))
	return container.NewVScroll(container.NewVBox(rows...))
}

// rolesScreen lists the roles (system prompts). Selecting a role shows its
// system prompt underneath the list.
func (p *preferencesScreen) rolesScreen() fyne.CanvasObject {
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
func (p *preferencesScreen) sessionsScreen() fyne.CanvasObject {
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
func (p *preferencesScreen) sessionRow(dir, id string) fyne.CanvasObject {
	return container.NewBorder(nil, nil, nil,
		widget.NewButton("Delete", func() { p.deleteSession(dir, id) }),
		widget.NewLabel(id))
}

// deleteSession removes the saved session and rebuilds the section so the
// list no longer shows it.
func (p *preferencesScreen) deleteSession(dir, id string) {
	if err := sessionstore.Delete(dir, id); err != nil {
		dialog.ShowError(err, p.guac.w)
		return
	}
	p.showSection(p.section)
}

func paddedLabel(text string) fyne.CanvasObject {
	return container.NewVBox(widget.NewLabel(text))
}

// providerTypeOptions lists the provider types offered when adding a
// provider, with the custom OpenAI-compatible option first.
var providerTypeOptions = []string{
	string(config.ModelProviderTypeOpenAICompatible),
	string(config.ModelProviderTypeOpenAI),
	string(config.ModelProviderTypeNvidia),
	string(config.ModelProviderTypeOpenCode),
}

// showAddProvider replaces the main window content with the Add Provider
// screen, the same way the Preferences screen does. It has a Cancel button
// (top left) that returns to Preferences, and an Add button (top right) that
// saves the provider and returns. The type dropdown offers the custom
// "OpenAI Compatible" option first, then the preconfigured providers;
// "OpenAI Compatible" collects a name, base URL and optional API key, while
// the preconfigured providers only ask for an API key since their name and
// base URL are fixed.
func (p *preferencesScreen) showAddProvider() {
	p.showProviderEditor(-1)
}

func (p *preferencesScreen) showEditProvider(idx int) {
	p.showProviderEditor(idx)
}

func (p *preferencesScreen) showProviderEditor(idx int) {
	var (
		nameEntry    *widget.Entry
		baseURLEntry *widget.Entry
		apiKeyEntry  *widget.Entry
	)
	fields := container.NewVBox()

	// goBack returns to the Preferences screen (the Providers section is the
	// one the user came from, and it is re-rendered from the saved config).
	goBack := func() {
		p.guac.showPreferences()
	}

	// buildFields lays out the type-specific fields for selectedType.
	buildFields := func(selectedType string) {
		apiKeyEntry = widget.NewEntry()
		apiKeyEntry.SetPlaceHolder("API key")
		apiKeyEntry.Password = true

		var form *widget.Form
		if selectedType == string(config.ModelProviderTypeOpenAICompatible) {
			nameEntry = widget.NewEntry()
			nameEntry.SetPlaceHolder("Provider name")
			baseURLEntry = widget.NewEntry()
			baseURLEntry.SetPlaceHolder("http://localhost:8080/v1")
			form = &widget.Form{Items: []*widget.FormItem{
				{Text: "Name", Widget: nameEntry},
				{Text: "Base URL", Widget: baseURLEntry},
				{Text: "API Key (optional)", Widget: apiKeyEntry},
			}}
		} else {
			// Preconfigured provider: only the API key is user-supplied.
			form = &widget.Form{Items: []*widget.FormItem{
				{Text: "API Key", Widget: apiKeyEntry},
			}}
		}
		fields.Objects = []fyne.CanvasObject{form}
		fields.Refresh()
	}

	typeSelect := widget.NewSelect(providerTypeOptions, func(s string) {
		buildFields(s)
	})
	buildFields(providerTypeOptions[0])
	typeSelect.SetSelected(providerTypeOptions[0])

	if idx >= 0 && p.guac.config != nil {
		if prov, ok := p.guac.config.GetModelProvider(idx); ok {
			typeSelect.SetSelected(string(prov.Type))
			buildFields(string(prov.Type))
			apiKeyEntry.SetText(prov.APIKey)
			if prov.Type == config.ModelProviderTypeOpenAICompatible {
				nameEntry.SetText(prov.Name)
				baseURLEntry.SetText(prov.BaseURL)
			} else {
				nameEntry = nil
				baseURLEntry = nil
			}
			typeSelect.Disable()
		}
	}

	saveProvider := func() {
		providerType := config.ModelProviderType(typeSelect.Selected)
		provider := config.ModelProvider{Type: providerType}
		if providerType == config.ModelProviderTypeOpenAICompatible {
			provider.Name = nameEntry.Text
			provider.BaseURL = baseURLEntry.Text
			provider.APIKey = apiKeyEntry.Text
		} else {
			provider.Name = providerType.PreconfiguredName()
			provider.BaseURL = providerType.PreconfiguredBaseURL()
			provider.APIKey = apiKeyEntry.Text
		}
		if p.guac.config != nil {
			if idx >= 0 {
				if existing, ok := p.guac.config.GetModelProvider(idx); ok {
					provider.Models = existing.Models
				}
				p.guac.config.UpdateModelProvider(idx, provider)
			} else {
				p.guac.config.AddModelProvider(provider)
			}
			_ = p.guac.config.Save()
			p.guac.reloadModels()
		}
		goBack()
	}

	header := container.NewBorder(nil, nil,
		widget.NewButtonWithIcon("Cancel", theme.NavigateBackIcon(), goBack),
		widget.NewButtonWithIcon("Save", theme.ContentAddIcon(), saveProvider),
		widget.NewLabelWithStyle("Provider", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
	)

	body := container.NewVBox(
		widget.NewLabel("Provider type:"),
		typeSelect,
		fields,
	)
	p.guac.w.SetContent(container.NewBorder(header, nil, nil, nil,
		container.NewVScroll(body)))
}

func (p *preferencesScreen) showProviderModels(idx int) {
	prov, ok := p.guac.config.GetModelProvider(idx)
	if !ok {
		return
	}

	goBack := func() {
		p.guac.showPreferences()
	}

	rows := container.NewVBox()

	populate := func(models []string, prechecked map[string]bool) {
		rows.Objects = nil
		checks := make([]*widget.Check, 0, len(models))
		for _, m := range models {
			c := widget.NewCheck(m, nil)
			if prechecked != nil && prechecked[m] {
				c.Checked = true
			}
			checks = append(checks, c)
			rows.Add(c)
		}
		rows.Refresh()
	}

	checked := make(map[string]bool, len(prov.Models))
	for _, m := range prov.Models {
		checked[m] = true
	}

	var checks []*widget.Check

	fetchAndPopulate := func() {
		models, err := restfulai.ListModels(prov.BaseURL, prov.APIKey)
		if err != nil {
			dialog.ShowError(err, p.guac.w)
			return
		}
		sort.Strings(models)
		populate(models, checked)
		checks = []*widget.Check{}
		for _, obj := range rows.Objects {
			if c, ok := obj.(*widget.Check); ok {
				checks = append(checks, c)
			}
		}
	}

	save := func() {
		var models []string
		for _, c := range checks {
			if c.Checked {
				models = append(models, c.Text)
			}
		}
		prov.Models = models
		p.guac.config.UpdateModelProvider(idx, prov)
		if err := p.guac.config.Save(); err != nil {
			dialog.ShowError(err, p.guac.w)
		}
		p.guac.reloadModels()
		goBack()
	}

	models := make([]string, len(prov.Models))
	copy(models, prov.Models)
	sort.Strings(models)
	populate(models, checked)
	checks = []*widget.Check{}
	for _, obj := range rows.Objects {
		if c, ok := obj.(*widget.Check); ok {
			checks = append(checks, c)
		}
	}

	header := container.NewBorder(nil, nil,
		widget.NewButtonWithIcon("Cancel", theme.NavigateBackIcon(), goBack),
		container.NewHBox(widget.NewButtonWithIcon("Re-fetch Models", theme.DownloadIcon(), fetchAndPopulate), widget.NewButtonWithIcon("Save", theme.ContentAddIcon(), save)),
		widget.NewLabelWithStyle("Provider Models", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
	)

	// Automatically fetch the latest models from the provider when opening.
	go fetchAndPopulate()

	p.guac.w.SetContent(container.NewBorder(header, nil, nil, nil,
		container.NewVScroll(rows)))
}
