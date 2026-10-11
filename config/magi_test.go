package config

import "testing"

func TestMagiNamesOrder(t *testing.T) {
	want := []string{"Melchior", "Balthasar", "Casper"}
	got := MagiNames()
	if len(got) != len(want) {
		t.Fatalf("MagiNames() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("MagiNames()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestMagiConfigModelAndSetModel(t *testing.T) {
	var m MagiConfig
	for _, name := range MagiNames() {
		label := "provider / model-" + name
		m.SetModel(name, label)
		if got := m.Model(name); got != label {
			t.Errorf("Model(%q) = %q, want %q", name, got, label)
		}
	}
	// Unknown names are ignored rather than panicking.
	m.SetModel("Morgan", "x")
	if got := m.Model("Morgan"); got != "" {
		t.Errorf("Model(unknown) = %q, want empty", got)
	}
}

func TestMagiConfigAssigned(t *testing.T) {
	var m MagiConfig
	if m.Assigned() {
		t.Error("empty config should not be Assigned")
	}
	m.SetModel(MagiMelchior, "a / m")
	if m.Assigned() {
		t.Error("partially assigned config should not be Assigned")
	}
	m.SetModel(MagiBalthasar, "a / m")
	m.SetModel(MagiCasper, "a / m")
	if !m.Assigned() {
		t.Error("fully assigned config should be Assigned")
	}
}

func TestMagiModelOption(t *testing.T) {
	opt := MagiModelOption()
	if !opt.IsMagi() {
		t.Error("MagiModelOption should report IsMagi")
	}
	if opt.Label() != MagiModel {
		t.Errorf("Label() = %q, want %q", opt.Label(), MagiModel)
	}
	real := ModelOption{Provider: "OpenAI", ModelID: "gpt-4o", ModelName: "gpt-4o"}
	if real.IsMagi() {
		t.Error("a real model should not report IsMagi")
	}
}

func TestAllSelectableModelOptionsLeadsWithMagi(t *testing.T) {
	gcfg := &GConfig{ModelProviders: []ModelProvider{
		{Name: "OpenAI", APIType: ModelProviderTypeOpenAI, BaseURL: "https://api.openai.com/v1", APIKey: NewAPIKey("k"), Models: []string{"gpt-4o"}},
	}}

	got := AllSelectableModelOptions(gcfg)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (Magi plus one model)", len(got))
	}
	if got[0].Label() != MagiModel {
		t.Errorf("first option = %q, want %q", got[0].Label(), MagiModel)
	}
	if got[1].Label() != "OpenAI / gpt-4o" {
		t.Errorf("second option = %q, want the real model", got[1].Label())
	}
}
