package app

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// themeColor resolves a theme color for the current app theme variant.
func themeColor(name fyne.ThemeColorName) color.Color {
	settings := fyne.CurrentApp().Settings()
	return settings.Theme().Color(name, settings.ThemeVariant())
}

// newBubble renders a message as a rounded colored block: user messages use
// the primary color with trailing-aligned text, AI messages the subtle hover
// color with leading-aligned text. The label is returned so a streamed reply
// can update it in place.
func newBubble(text string, isUser bool) (*fyne.Container, *widget.Label) {
	var (
		name  fyne.ThemeColorName
		align fyne.TextAlign
	)
	if isUser {
		name, align = theme.ColorNamePrimary, fyne.TextAlignTrailing
	} else {
		name, align = theme.ColorNameHover, fyne.TextAlignLeading
	}
	label := widget.NewLabelWithStyle(text, align, fyne.TextStyle{})
	label.Wrapping = fyne.TextWrapWord
	// Selectable lets the user select the message text and copy it (drag to
	// select, then Ctrl/Cmd+C or right-click → Copy).
	label.Selectable = true

	background := canvas.NewRectangle(themeColor(name))
	background.CornerRadius = 6
	return container.NewStack(background, label), label
}

// spinnerWithCancel holds a spinner and its cancel button so they can be
// moved and removed together as one log entry.
type spinnerWithCancel struct {
	spinner      *widget.ProgressBarInfinite
	cancelButton *widget.Button
	container    *fyne.Container
}

// addUserMessage appends the user's message as a bubble and returns a
// spinnerWithCancel shown as the bottommost entry while the query is in
// flight. The cancel button's OnTapped callback should be set by the caller.
func (c *chat) addUserMessage(text string) *spinnerWithCancel {
	spinner := widget.NewProgressBarInfinite()

	// A red X sits to the left of the spinner so the user can abort the
	// in-flight request (the whole turn set, including tool rounds).
	cancelButton := widget.NewButtonWithIcon("", theme.CancelIcon(), nil)
	cancelButton.Importance = widget.DangerImportance

	// Use Border layout so the spinner expands to fill the full width
	// with the cancel button fixed on the left.
	spinnerRow := container.NewBorder(nil, nil, cancelButton, nil, spinner)

	bubble, _ := newBubble(text, true)
	c.log.Add(bubble)
	c.log.Add(spinnerRow)

	swc := &spinnerWithCancel{
		spinner:      spinner,
		cancelButton: cancelButton,
		container:    spinnerRow,
	}
	c.activeSpinner = swc
	c.scrollLog()
	spinner.Start()
	return swc
}

// removeSpinner removes the in-progress spinner (and its cancel button) from
// the log, if present.
func (c *chat) removeSpinner(swc *spinnerWithCancel) {
	if swc == nil {
		return
	}
	swc.spinner.Stop()
	c.log.Remove(swc.container)
	if c.activeSpinner == swc {
		c.activeSpinner = nil
	}
	c.scrollLog()
}

// ensureSpinnerAtBottom moves the active spinner row to the bottom of the log
// if it exists. Call this after adding any new content to the log.
func (c *chat) ensureSpinnerAtBottom() {
	if c.activeSpinner != nil {
		// Remove and re-add to move to bottom
		c.log.Remove(c.activeSpinner.container)
		c.log.Add(c.activeSpinner.container)
		c.scrollLog()
	}
}

// addAIMessage appends an empty AI bubble that grows as reply chunks arrive.
// It is created lazily on the first non-empty chunk, so a failed turn or an
// entirely empty response leaves no empty bubble behind.
func (c *chat) addAIMessage() *widget.Label {
	bubble, label := newBubble("", false)
	c.log.Add(bubble)
	c.ensureSpinnerAtBottom()
	return label
}

// addToolCall appends a tool-activity line for a call that is starting, and
// returns the label so the caller can show the outcome on it in place once
// the call finishes. The line is styled distinct from the message bubbles so
// tool use stays visible without looking like part of the conversation.
func (c *chat) addToolCall(summary string) *widget.Label {
	label := widget.NewLabelWithStyle("→ "+summary, fyne.TextAlignLeading, fyne.TextStyle{Italic: true})
	label.Wrapping = fyne.TextWrapWord
	label.Importance = widget.LowImportance
	label.Selectable = true
	c.log.Add(label)
	c.ensureSpinnerAtBottom()
	return label
}

// markToolError shows a failed tool call on its activity line. Successful
// calls leave the line untouched: the reply that follows shows the result.
func (c *chat) markToolError(label *widget.Label, summary string, err error) {
	if label == nil {
		return
	}
	label.SetText("→ " + summary + " — failed: " + err.Error())
	label.Importance = widget.DangerImportance
	c.scrollLog()
}

// addErrorMessage appends a centered error line for a failed turn.
func (c *chat) addErrorMessage(err error) {
	label := widget.NewLabelWithStyle("Error: "+err.Error(), fyne.TextAlignCenter, fyne.TextStyle{Italic: true})
	label.Wrapping = fyne.TextWrapWord
	label.Importance = widget.DangerImportance
	label.Selectable = true
	c.log.Add(label)
	c.ensureSpinnerAtBottom()
}

// scrollLog keeps the newest message in view.
func (c *chat) scrollLog() {
	c.logScroll.ScrollToBottom()
}
