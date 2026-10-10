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
	background := canvas.NewRectangle(themeColor(name))
	background.CornerRadius = 6
	return container.NewStack(background, label), label
}

// addUserMessage appends the user's message as a bubble and returns the
// thinking spinner shown against it while the query is in flight. The
// spinner is hidden when the turn ends, so the bubble spans the full width.
func (c *chat) addUserMessage(text string) *widget.ProgressBarInfinite {
	spinner := widget.NewProgressBarInfinite()
	spinner.Hide()
	bubble, _ := newBubble(text, true)
	c.log.Add(container.NewBorder(nil, nil, spinner, nil, bubble))
	c.scrollLog()
	spinner.Show()
	spinner.Start()
	return spinner
}

// addAIMessage appends an empty AI bubble that grows as reply chunks arrive.
// It is created lazily on the first chunk, so a failed turn leaves no empty
// bubble behind.
func (c *chat) addAIMessage() *widget.Label {
	bubble, label := newBubble("", false)
	c.log.Add(bubble)
	c.scrollLog()
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
	c.log.Add(label)
	c.scrollLog()
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
	c.log.Add(label)
	c.scrollLog()
}

// scrollLog keeps the newest message in view.
func (c *chat) scrollLog() {
	c.logScroll.ScrollToBottom()
}
