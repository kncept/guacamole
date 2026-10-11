// Package userinput defines how the app asks the user a question in the
// foreground (blocking) so the answer can be used to continue. A single
// interface backs every user prompt: permission checks, the model's
// user_question tool, and tool-round-limit decisions. Each client — the GUI,
// the CLI — provides one implementation and reuses it for all of them.
package userinput

// RequestForegroundUserInput asks the user a question in the foreground and
// returns their answer. Implementations render the question wherever the
// user can answer it — buttons in a GUI, a numbered menu on a terminal —
// and block until an answer is given.
type RequestForegroundUserInput interface {
	// Prompt asks the user a question. text is the prompt to display (for
	// example "Allow write access to directory X?"); options is the ordered
	// list of responses the user can pick from; allowFree also accepts a
	// free-text answer. Prompt returns the chosen option or free-text
	// answer, or an error if the question could not be asked (for example,
	// standard input closed mid-prompt).
	Prompt(text string, allowFree bool, options []string) (string, error)
}
