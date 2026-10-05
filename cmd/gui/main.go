package main

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// session represents a single chat session with its own output log and input field.
type session struct {
	id       string
	output   *widget.Label
	input    *widget.Entry
	messages []string
}

// Track which session is currently active (index into sessions slice)
var activeSessionIdx = 0

var sessions []*session

func newSession() *session {
	id := fmt.Sprintf("Session %d", len(sessions)+1)
	s := &session{
		id:       id,
		output:   widget.NewLabel(""),
		input:    widget.NewEntry(),
		messages: []string{},
	}
	return s
}

func main() {
	a := app.New()
	w := a.NewWindow("Guacamole GUI")

	// Initialize with one default session
	if len(sessions) == 0 {
		sessions = append(sessions, newSession())
	}
	activeSessionIdx = 0

	// ---- Build the UI ----

	// Tab container - holds one tab per session
	tabContainer := container.NewAppTabs()

	// Initialize with the first session's tab
	updateTabForSession(tabContainer, sessions[activeSessionIdx])

	// ---- "New Session" tab ----
	// This tab is always the rightmost tab and creates a new session when clicked
	newSessionTab := container.NewTabItem("New Session", widget.NewLabel("Click to create new session"))
	tabContainer.Append(newSessionTab)

	// Set up tab selection handler
	tabContainer.OnSelected = func(selected *container.TabItem) {
		// Find which tab was selected and update active session index
		for i, item := range tabContainer.Items {
			if item == selected {
				// If it's the "New Session" tab (rightmost), create a new session
				if item.Text == "New Session" {
					s := newSession()
					sessions = append(sessions, s)
					activeSessionIdx = len(sessions) - 1
					updateTabForSession(tabContainer, s)
					// Remove and re-append "New Session" to keep it rightmost
					tabContainer.Remove(newSessionTab)
					tabContainer.Append(newSessionTab)
					// Select the newly created session (now second-to-last)
					tabContainer.SelectIndex(len(tabContainer.Items) - 2)
					// Recreate input box for the new active session
					inputBox := createInputBox(sessions[activeSessionIdx].input)
					windowContent := container.NewVBox(
						tabContainer,
						inputBox,
					)
					w.SetContent(windowContent)
					w.Resize(fyne.NewSize(600, 400))
				}
				// If it's a regular session tab (not "New Session"), update active session
				if item.Text != "New Session" && i < len(sessions) {
					activeSessionIdx = i
					// Recreate input box for the new active session
					inputBox := createInputBox(sessions[activeSessionIdx].input)
					windowContent := container.NewVBox(
						tabContainer,
						inputBox,
					)
					w.SetContent(windowContent)
					w.Resize(fyne.NewSize(600, 400))
				}
				break
			}
		}
	}

	// ---- "New Tab" menu item ----
	newTabItem := fyne.NewMenuItem("New Tab", func() {
		s := newSession()
		sessions = append(sessions, s)
		updateTabForSession(tabContainer, s)
		// Select the newly added tab
		tabContainer.SelectIndex(len(tabContainer.Items) - 1)
		// Set this new session as active and recreate input box
		activeSessionIdx = len(sessions) - 1
		inputBox := createInputBox(sessions[activeSessionIdx].input)
		windowContent := container.NewVBox(
			tabContainer,
			inputBox,
		)
		w.SetContent(windowContent)
		w.Resize(fyne.NewSize(600, 400))
	})

	// ---- File menu with New Tab ----
	fileMenu := fyne.NewMenu("File",
		newTabItem,
	)

	// ---- Help menu ----
	helpMenu := fyne.NewMenu("About",
		fyne.NewMenuItem("About", func() {
			w.SetContent(container.NewVBox(
				widget.NewLabel("Guacamole GUI"),
				widget.NewLabel("A simple Fyne GUI with tabbed sessions."),
			))
		}),
	)

	// ---- Menu bar ----
	mainMenu := fyne.NewMainMenu(fileMenu, helpMenu)
	w.SetMainMenu(mainMenu)

	// Initial input box for the first session
	inputBox := createInputBox(sessions[activeSessionIdx].input)

	// ---- Window content ----
	// Layout: tabbed output area on top, input area at bottom
	windowContent := container.NewVBox(
		tabContainer,
		inputBox,
	)

	w.SetContent(windowContent)
	w.Resize(fyne.NewSize(600, 400))
	w.ShowAndRun()
}

// createInputBox creates the input box (entry + submit button) for the given session's input widget.
func createInputBox(activeInput *widget.Entry) *fyne.Container {
	submitBtn := widget.NewButton("Submit", func() {
		text := activeInput.Text
		if strings.TrimSpace(text) == "" {
			return
		}
		addMessageToActive(sessions, activeSessionIdx, text)
		activeInput.SetText("")
	})

	return container.NewBorder(nil, nil, nil, submitBtn, activeInput)
}

// updateTabForSession creates/updates the tab for a given session and appends it to the tab container.
func updateTabForSession(tabContainer *container.AppTabs, s *session) {
	// Update the output label with all messages
	outputText := strings.Join(s.messages, "\n")
	s.output.SetText(outputText)

	// Create the tab with the session's output label
	tabItem := container.NewTabItem(s.id, s.output)
	tabContainer.Append(tabItem)
}

// addMessageToActive appends a prompt to the active session's output.
func addMessageToActive(sessions []*session, activeSessionIdx int, text string) {
	// Use the session at the given index
	s := sessions[activeSessionIdx]

	// Append the message
	s.messages = append(s.messages, text)

	// Update the output label with all messages
	outputText := strings.Join(s.messages, "\n")
	s.output.SetText(outputText)

	// Also print to console for debugging
	fmt.Printf("[%s] %s\n", s.id, text)
}