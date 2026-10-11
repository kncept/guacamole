package config

// MagiModel is the label of the virtual "Magi" model. It sits at the top of
// the list of available models. It is not a real model: selecting it switches
// a session to Magi Mode, where the three configured magi compete and only
// the best response is kept.
const MagiModel = "Magi"

// Magi names, in the fixed display order used everywhere (the Preferences
// screen, the turn order, and the vote prompts).
const (
	MagiMelchior  = "Melchior"
	MagiBalthasar = "Balthasar"
	MagiCasper    = "Casper"
)

// MagiNames lists the three magi in fixed display order.
func MagiNames() []string {
	return []string{MagiMelchior, MagiBalthasar, MagiCasper}
}

// MagiConfig holds the Magi Mode settings: which real model each magi plays.
// The values are "Provider / Model" labels from the available models, and
// must never be MagiModel itself (a magi cannot be the virtual Magi).
type MagiConfig struct {
	Melchior  string `json:"melchior,omitempty"`
	Balthasar string `json:"balthasar,omitempty"`
	Casper    string `json:"casper,omitempty"`
}

// Model returns the "Provider / Model" label assigned to the magi with the
// given name, or "" when none is assigned (or the name is unknown).
func (m MagiConfig) Model(name string) string {
	switch name {
	case MagiMelchior:
		return m.Melchior
	case MagiBalthasar:
		return m.Balthasar
	case MagiCasper:
		return m.Casper
	default:
		return ""
	}
}

// SetModel assigns label to the magi with the given name. Unknown names are
// ignored.
func (m *MagiConfig) SetModel(name, label string) {
	switch name {
	case MagiMelchior:
		m.Melchior = label
	case MagiBalthasar:
		m.Balthasar = label
	case MagiCasper:
		m.Casper = label
	}
}

// Assigned reports whether every magi has a model assigned.
func (m MagiConfig) Assigned() bool {
	for _, name := range MagiNames() {
		if m.Model(name) == "" {
			return false
		}
	}
	return true
}
