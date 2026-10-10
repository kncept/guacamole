package app

import (
	"fmt"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/kncept/guacamole/config"
	"github.com/kncept/guacamole/modelclient"
	"github.com/kncept/guacamole/restfulai"
	"github.com/kncept/guacamole/sessionstore"
)

// preferenceSections are the entries of the Preferences navigation list, in
// display order. Each one names a screen built by screenFor.
var preferenceSections = []string{"Providers", "Roles", "Sessions", "Permissions"}

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

	// For permissions screen: track the rule editors to enable save/reset
	fsDirEntries     []*widget.Entry
	fsReadSelects    []*widget.Select
	fsWriteSelects   []*widget.Select
	webDomainEntries []*widget.Entry
	webPolicySelects []*widget.Select
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
	case "Permissions":
		return p.permissionsScreen()
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
		if prov.APIType != "" {
			heading = heading + " (API: " + string(prov.APIType) + ")"
		}
		rows = append(rows, widget.NewLabelWithStyle(heading,
			fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
		if prov.BaseURL != "" {
			rows = append(rows, widget.NewLabel("Base URL: "+prov.BaseURL))
		}
		rows = append(rows, container.NewHBox(
			widget.NewButton("Edit", func() { p.showEditProvider(idx) }),
			widget.NewButton("Models", func() { p.showProviderModels(idx) }),
			func() *widget.Button {
				btn := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() { p.confirmDeleteProvider(idx) })
				btn.Importance = widget.LowImportance
				return btn
			}(),
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

// confirmDeleteProvider shows a confirmation dialog before deleting a provider.
func (p *preferencesScreen) confirmDeleteProvider(idx int) {
	prov, ok := p.guac.config.GetModelProvider(idx)
	if !ok {
		return
	}
	dialog.ShowConfirm(
		"Delete Provider",
		fmt.Sprintf("Are you sure you want to delete the provider '%s'?", prov.Name),
		func(confirmed bool) {
			if confirmed {
				p.deleteProvider(idx)
			}
		},
		p.guac.w,
	)
}

// deleteProvider removes the provider at the given index and refreshes the screen.
func (p *preferencesScreen) deleteProvider(idx int) {
	if p.guac.config == nil {
		return
	}
	prov, ok := p.guac.config.GetModelProvider(idx)
	if !ok {
		return
	}
	p.guac.config.RemoveModelProvider(prov.Name)
	_ = p.guac.config.Save()
	p.guac.reloadModels()
	p.showSection(p.section)
}

func paddedLabel(text string) fyne.CanvasObject {
	return container.NewVBox(widget.NewLabel(text))
}

// providerNames lists the provider presets offered in the provider dropdown,
// in display order.
func providerNames() []string {
	presets := config.ProviderPresets()
	names := make([]string, len(presets))
	for i, pr := range presets {
		names[i] = string(pr)
	}
	return names
}

// apiTypeNames lists the API types a custom provider may choose. OpenCode is
// deliberately absent: it is applied automatically by the OpenCode preset.
func apiTypeNames() []string {
	types := config.SelectableAPITypes()
	names := make([]string, len(types))
	for i, t := range types {
		names[i] = string(t)
	}
	return names
}

// ensureType returns options with value added if it is missing, so an
// existing provider's API type can always be displayed even when it is not
// normally selectable.
func ensureType(options []string, value string) []string {
	for _, o := range options {
		if o == value {
			return options
		}
	}
	return append(options, value)
}

// showAddProvider replaces the main window content with the Add Provider
// screen, the same way the Preferences screen does. It has a Cancel button
// (top left) that returns to Preferences, and an Add button (top right) that
// saves the provider and returns. The type dropdown offers the custom
// "OpenAI-compatible" option first, then the preconfigured providers;
// "OpenAI Compatible" collects a name, base URL and optional API key, while
// the preconfigured providers only ask for an API key since their name and
// base URL are fixed.
func (p *preferencesScreen) showAddProvider() {
	p.showProviderEditor(-1)
}

func (p *preferencesScreen) showEditProvider(idx int) {
	p.showProviderEditor(idx)
}

// showProviderEditor shows the editor for adding or editing a provider. The
// first field chooses the provider: picking a preset fills in its connection
// details, which stay locked except for the name and API key. Custom leaves
// every field editable.
func (p *preferencesScreen) showProviderEditor(idx int) {
	var (
		nameEntry     *widget.Entry
		baseURLEntry  *widget.Entry
		apiKeyEntry   *widget.Entry
		apiTypeSelect *widget.Select
		presetSelect  *widget.Select
	)

	// goBack returns to the Preferences screen (the Providers section is the
	// one the user came from, and it is re-rendered from the saved config).
	goBack := func() {
		p.guac.showPreferences()
	}

	// applyPreset fills the fields for the selected preset. Preset connection
	// details are locked; Custom leaves them editable.
	applyPreset := func(presetName string) {
		preset := config.ProviderPreset(presetName)
		if d, ok := preset.Details(); ok {
			nameEntry.SetText(d.Name)
			baseURLEntry.SetText(d.BaseURL)
			apiTypeSelect.Options = []string{string(d.APIType)}
			apiTypeSelect.SetSelected(string(d.APIType))
			baseURLEntry.Disable()
			apiTypeSelect.Disable()
		} else {
			// Custom.
			baseURLEntry.Enable()
			apiTypeSelect.Options = apiTypeNames()
			if apiTypeSelect.Selected == "" || apiTypeSelect.Selected == string(config.ModelProviderTypeOpenCode) {
				apiTypeSelect.SetSelected(string(config.ModelProviderTypeOpenAI))
			}
			apiTypeSelect.Enable()
			baseURLEntry.SetText("")
		}
		apiTypeSelect.Refresh()
	}

	baseURLEntry = widget.NewEntry()
	baseURLEntry.SetPlaceHolder("https://api.example.com/v1")

	apiKeyEntry = widget.NewEntry()
	apiKeyEntry.SetPlaceHolder("API key (optional)")
	apiKeyEntry.Password = true

	apiTypeSelect = widget.NewSelect(apiTypeNames(), nil)

	presetSelect = widget.NewSelect(providerNames(), applyPreset)
	nameEntry = widget.NewEntry()
	nameEntry.SetPlaceHolder("Provider name")

	if idx >= 0 && p.guac.config != nil {
		if prov, ok := p.guac.config.GetModelProvider(idx); ok {
			// Pick the preset whose fixed details match the stored provider.
			preset := config.PresetForProvider(prov)
			presetSelect.SetSelected(string(preset))
			applyPreset(string(preset))

			// Presets cannot be changed once saved; Custom stays custom.
			presetSelect.Disable()

			// The stored values win over the preset defaults: name and API key
			// are always editable, and a Custom provider's details are restored.
			nameEntry.SetText(prov.Name)
			baseURLEntry.SetText(prov.BaseURL)
			apiKeyEntry.SetText(prov.APIKey.String())
			if preset == config.ProviderPresetCustom {
				apiTypeSelect.Options = ensureType(apiTypeNames(), string(prov.APIType))
				apiTypeSelect.SetSelected(string(prov.APIType))
				apiTypeSelect.Enable()
				apiTypeSelect.Refresh()
			}
		}
	} else {
		// Adding: start on the first preset so the form is filled in.
		presetSelect.SetSelected(string(config.ProviderPresetOpenCode))
		applyPreset(string(config.ProviderPresetOpenCode))
	}

	saveProvider := func() {
		preset := config.ProviderPreset(presetSelect.Selected)
		var provider config.ModelProvider
		if d, ok := preset.Details(); ok {
			provider = config.ModelProvider{
				Name:    nameEntry.Text,
				APIType: d.APIType,
				BaseURL: d.BaseURL,
				APIKey:  config.NewAPIKey(apiKeyEntry.Text),
			}
		} else {
			provider = config.ModelProvider{
				Name:    nameEntry.Text,
				APIType: config.ModelProviderType(apiTypeSelect.Selected),
				BaseURL: baseURLEntry.Text,
				APIKey:  config.NewAPIKey(apiKeyEntry.Text),
			}
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

	form := &widget.Form{Items: []*widget.FormItem{
		{Text: "Provider", Widget: presetSelect},
		{Text: "Name", Widget: nameEntry},
		{Text: "Base URL", Widget: baseURLEntry},
		{Text: "API Type", Widget: apiTypeSelect},
		{Text: "API Key", Widget: apiKeyEntry},
	}}
	p.guac.w.SetContent(container.NewBorder(header, nil, nil, nil,
		container.NewVScroll(form)))
}

func (p *preferencesScreen) showProviderModels(idx int) {
	prov, ok := p.guac.config.GetModelProvider(idx)
	if !ok {
		return
	}

	goBack := func() {
		p.guac.showPreferences()
	}

	// modelInfos and checks hold the models currently displayed and their
	// selection state, in the same order.
	var modelInfos []restfulai.ModelInfo
	var checks []*widget.Check

	// persist writes the currently checked models back to the provider, saves
	// the config immediately, and refreshes the open sessions' model dropdowns,
	// so a selection takes effect without pressing Save or restarting the app.
	persist := func() {
		var models []string
		for i, c := range checks {
			if c.Checked && i < len(modelInfos) {
				models = append(models, modelInfos[i].ID)
			}
		}
		prov.Models = models
		p.guac.config.UpdateModelProvider(idx, prov)
		if err := p.guac.config.Save(); err != nil {
			dialog.ShowError(err, p.guac.w)
		}
		p.guac.reloadModels()
	}

	rows := container.NewVBox()

	// populate rebuilds the visible model rows for infos. Checkboxes reflect the
	// models already saved on the provider, so re-fetching the catalog never
	// discards a selection.
	populate := func(infos []restfulai.ModelInfo) {
		modelInfos = infos
		checks = nil
		rows.Objects = nil

		hasSize := false
		hasFree := false
		hasDescription := false
		for _, m := range infos {
			if m.Size != "" {
				hasSize = true
			}
			if m.IsFree {
				hasFree = true
			}
			if m.Description != "" {
				hasDescription = true
			}
		}

		// Header row.
		headerParts := []fyne.CanvasObject{widget.NewLabelWithStyle("Model ID", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})}
		if hasSize {
			headerParts = append(headerParts, widget.NewLabelWithStyle("Size", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
		}
		if hasFree {
			headerParts = append(headerParts, widget.NewLabelWithStyle("Free", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
		}
		if hasDescription {
			headerParts = append(headerParts, widget.NewLabelWithStyle("Description", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
		}
		headerParts = append(headerParts, widget.NewLabelWithStyle("Select", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
		rows.Add(container.NewHBox(headerParts...))
		rows.Add(widget.NewSeparator())

		// One row per model, with a checkbox wired to persist on toggle.
		for _, m := range infos {
			checked := false
			for _, existing := range prov.Models {
				if existing == m.ID {
					checked = true
					break
				}
			}
			c := widget.NewCheck("", func(bool) { persist() })
			c.Checked = checked
			checks = append(checks, c)

			rowParts := []fyne.CanvasObject{widget.NewLabel(m.ID)}
			if hasSize {
				rowParts = append(rowParts, widget.NewLabel(m.Size))
			}
			if hasFree {
				freeLabel := ""
				if m.IsFree {
					freeLabel = "Yes"
				}
				rowParts = append(rowParts, widget.NewLabel(freeLabel))
			}
			if hasDescription {
				rowParts = append(rowParts, widget.NewLabel(m.Description))
			}
			rowParts = append(rowParts, c)

			rows.Add(container.NewHBox(rowParts...))
		}
		rows.Refresh()
	}

	// fetchAndPopulate lists the provider's models and repopulates the rows. It
	// runs the network call off the UI thread and applies the result on it.
	fetchAndPopulate := func() {
		infos, err := modelclient.ListModels(prov.APIType, prov.BaseURL, prov.APIKey.String())
		if err != nil {
			fyne.Do(func() { dialog.ShowError(err, p.guac.w) })
			return
		}
		fyne.Do(func() {
			populate(infos)
		})
	}

	// Initial population with the provider's existing models (just IDs), so the
	// current selection is visible immediately; the fetch then adds the full
	// catalog with any extra metadata.
	initialInfos := make([]restfulai.ModelInfo, len(prov.Models))
	for i, m := range prov.Models {
		initialInfos[i] = restfulai.ModelInfo{ID: m}
	}
	populate(initialInfos)

	header := container.NewBorder(nil, nil,
		widget.NewButtonWithIcon("Done", theme.NavigateBackIcon(), goBack),
		container.NewHBox(
			widget.NewButtonWithIcon("Re-fetch Models", theme.DownloadIcon(), fetchAndPopulate),
			widget.NewButtonWithIcon("Save", theme.ContentAddIcon(), func() {
				persist()
				goBack()
			}),
		),
		widget.NewLabelWithStyle("Provider Models", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
	)

	p.guac.w.SetContent(container.NewBorder(header, nil, nil, nil,
		container.NewVScroll(rows)))

	// Fetch the latest models from the provider when opening.
	go fetchAndPopulate()
}

func policyOptions() []string {
	return []string{string(config.PolicyAllow), string(config.PolicyAsk), string(config.PolicyDeny)}
}

func (p *preferencesScreen) permissionsScreen() fyne.CanvasObject {
	cfg := p.guac.config
	if cfg == nil {
		return paddedLabel("No config available")
	}

	// Initialize editor tracking slices
	p.fsDirEntries = nil
	p.fsReadSelects = nil
	p.fsWriteSelects = nil
	p.webDomainEntries = nil
	p.webPolicySelects = nil

	// Build the content rows
	rows := container.NewVBox()

	// Filesystem section
	fsAllowAll := widget.NewCheck("Allow all filesystem access", func(b bool) {
		cfg.Permissions.Filesystem.AllowAll = b
		_ = cfg.Save()
		p.rebuildPermissionsScreen()
	})
	fsAllowAll.Checked = cfg.Permissions.Filesystem.AllowAll

	rows.Add(widget.NewLabelWithStyle("Filesystem", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
	rows.Add(fsAllowAll)
	rows.Add(widget.NewLabel("Directory → Read / Write"))

	// Filesystem directory rules
	for i := range cfg.Permissions.Filesystem.Directories {
		p.addFilesystemRuleRow(rows, cfg, i)
	}

	addFs := widget.NewButton("Add Directory", func() {
		cfg.Permissions.Filesystem.Directories = append(cfg.Permissions.Filesystem.Directories, config.DirectoryPermission{})
		_ = cfg.Save()
		p.rebuildPermissionsScreen()
	})
	rows.Add(addFs)
	rows.Add(widget.NewSeparator())

	// Shell section
	shellPolicy := widget.NewSelect([]string{string(config.PolicyAllow), string(config.PolicyAsk), string(config.PolicyDeny)}, func(s string) {
		cfg.Permissions.Shell.Policy = config.Policy(s)
		_ = cfg.Save()
	})
	shellPolicy.SetSelected(string(cfg.Permissions.Shell.Policy))

	rows.Add(widget.NewLabelWithStyle("Shell", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
	rows.Add(shellPolicy)
	rows.Add(widget.NewSeparator())

	// Web section
	webAllowAll := widget.NewCheck("Allow all web access", func(b bool) {
		cfg.Permissions.Web.AllowAll = b
		_ = cfg.Save()
		p.rebuildPermissionsScreen()
	})
	webAllowAll.Checked = cfg.Permissions.Web.AllowAll

	rows.Add(widget.NewLabelWithStyle("Web", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
	rows.Add(webAllowAll)
	rows.Add(widget.NewLabel("Domain → Policy"))

	// Web domain rules
	for i := range cfg.Permissions.Web.Domains {
		p.addWebRuleRow(rows, cfg, i)
	}

	addWeb := widget.NewButton("Add Domain", func() {
		cfg.Permissions.Web.Domains = append(cfg.Permissions.Web.Domains, config.DomainPermission{})
		_ = cfg.Save()
		p.rebuildPermissionsScreen()
	})
	rows.Add(addWeb)
	rows.Add(widget.NewSeparator())

	// Reset all permissions
	resetAll := widget.NewButton("Reset all permissions to defaults", func() {
		cfg.Permissions = config.Permissions{}
		cfg.InitDefaults()
		_ = cfg.Save()
		p.rebuildPermissionsScreen()
	})
	rows.Add(resetAll)

	// Wrap in a scroll container
	content := container.NewVScroll(rows)

	// Create header with Reset and Save buttons for the permissions section
	header := container.NewBorder(nil, nil,
		widget.NewButtonWithIcon("Reset", theme.ContentClearIcon(), func() {
			cfg.Permissions = config.Permissions{}
			cfg.InitDefaults()
			_ = cfg.Save()
			p.rebuildPermissionsScreen()
		}),
		widget.NewButtonWithIcon("Save", theme.DocumentSaveIcon(), func() {
			_ = cfg.Save()
			dialog.ShowInformation("Saved", "Permissions saved successfully", p.guac.w)
		}),
		widget.NewLabelWithStyle("Permissions", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
	)

	return container.NewBorder(header, nil, nil, nil, content)
}

// rebuildPermissionsScreen refreshes the permissions section by re-showing it
func (p *preferencesScreen) rebuildPermissionsScreen() {
	p.showSection(p.section)
}

// addFilesystemRuleRow adds a row for editing a filesystem directory rule
func (p *preferencesScreen) addFilesystemRuleRow(rows *fyne.Container, cfg *config.GConfig, index int) {
	dir := &cfg.Permissions.Filesystem.Directories[index]
	dirEntry := widget.NewEntry()
	dirEntry.SetText(dir.Directory)
	readSel := widget.NewSelect(policyOptions(), func(s string) {
		dir.Read = config.Policy(s)
		_ = cfg.Save()
	})
	readSel.SetSelected(string(dir.Read))
	writeSel := widget.NewSelect(policyOptions(), func(s string) {
		dir.Write = config.Policy(s)
		_ = cfg.Save()
	})
	writeSel.SetSelected(string(dir.Write))

	// Track for potential future use
	p.fsDirEntries = append(p.fsDirEntries, dirEntry)
	p.fsReadSelects = append(p.fsReadSelects, readSel)
	p.fsWriteSelects = append(p.fsWriteSelects, writeSel)

	removeBtn := widget.NewButtonWithIcon("", theme.ContentRemoveIcon(), func() {
		cfg.Permissions.Filesystem.Directories = append(cfg.Permissions.Filesystem.Directories[:index], cfg.Permissions.Filesystem.Directories[index+1:]...)
		_ = cfg.Save()
		p.rebuildPermissionsScreen()
	})
	removeBtn.Importance = widget.LowImportance

	rows.Add(container.NewHBox(dirEntry, readSel, writeSel, removeBtn))
}

// addWebRuleRow adds a row for editing a web domain rule
func (p *preferencesScreen) addWebRuleRow(rows *fyne.Container, cfg *config.GConfig, index int) {
	d := &cfg.Permissions.Web.Domains[index]
	dEntry := widget.NewEntry()
	dEntry.SetText(d.Domain)
	polSel := widget.NewSelect(policyOptions(), func(s string) {
		d.Policy = config.Policy(s)
		_ = cfg.Save()
	})
	polSel.SetSelected(string(d.Policy))

	// Track for potential future use
	p.webDomainEntries = append(p.webDomainEntries, dEntry)
	p.webPolicySelects = append(p.webPolicySelects, polSel)

	removeBtn := widget.NewButtonWithIcon("", theme.ContentRemoveIcon(), func() {
		cfg.Permissions.Web.Domains = append(cfg.Permissions.Web.Domains[:index], cfg.Permissions.Web.Domains[index+1:]...)
		_ = cfg.Save()
		p.rebuildPermissionsScreen()
	})
	removeBtn.Importance = widget.LowImportance

	rows.Add(container.NewHBox(dEntry, polSel, removeBtn))
}
