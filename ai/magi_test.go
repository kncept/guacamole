package ai

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// funcProvider answers every request through respond, so a test can drive
// a magi's inference and voting responses independently.
type funcProvider struct {
	respond func(req Request) (Response, error)
}

func (p *funcProvider) Send(ctx context.Context, req Request) (Response, error) {
	return p.respond(req)
}

func (p *funcProvider) SendStream(ctx context.Context, req Request, onChunk func(Chunk) error) (Response, error) {
	return p.respond(req)
}

// textMagi builds a member that always answers text and casts the given votes
// (name → score) when asked to judge.
func textMagi(name, text string, votes map[string]int) MagiMember {
	return MagiMember{
		Name:  name,
		Model: name,
		Loop: NewLoop(&funcProvider{respond: func(req Request) (Response, error) {
			if req.SystemPrompt == judgeSystemPrompt {
				b, _ := json.Marshal(votes)
				return Response{Text: string(b), Finished: true}, nil
			}
			return Response{Text: text, Finished: true}, nil
		}}),
	}
}

func TestMagiKeepsHighestScoringResponse(t *testing.T) {
	// Totals: Melchior 60+70=130, Balthasar 90+80=170, Casper 50+40=90.
	magi := &Magi{Members: []MagiMember{
		textMagi("Melchior", "answer A", map[string]int{"Balthasar": 90, "Casper": 50}),
		textMagi("Balthasar", "answer B", map[string]int{"Melchior": 60, "Casper": 40}),
		textMagi("Casper", "answer C", map[string]int{"Melchior": 70, "Balthasar": 80}),
	}}

	resp, err := magi.Run(context.Background(), Request{Prompt: "hi"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if resp.Text != "answer B" {
		t.Errorf("winner = %q, want %q", resp.Text, "answer B")
	}
}

func TestMagiTieKeepsEarlierMagi(t *testing.T) {
	// Every magi scores the others 100, so all totals tie; Melchior, listed
	// first, wins.
	magi := &Magi{Members: []MagiMember{
		textMagi("Melchior", "answer A", map[string]int{"Balthasar": 100, "Casper": 100}),
		textMagi("Balthasar", "answer B", map[string]int{"Melchior": 100, "Casper": 100}),
		textMagi("Casper", "answer C", map[string]int{"Melchior": 100, "Balthasar": 100}),
	}}

	resp, err := magi.Run(context.Background(), Request{Prompt: "hi"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if resp.Text != "answer A" {
		t.Errorf("winner = %q, want %q", resp.Text, "answer A")
	}
}

func TestMagiNeedsTwoMembers(t *testing.T) {
	magi := &Magi{Members: []MagiMember{
		textMagi("Melchior", "answer A", nil),
	}}
	if _, err := magi.Run(context.Background(), Request{Prompt: "hi"}); err == nil {
		t.Fatal("expected an error for a one-member panel")
	}
}

func TestMagiOnPhaseReportsPhases(t *testing.T) {
	magi := &Magi{
		Members: []MagiMember{
			textMagi("Melchior", "answer A", map[string]int{"Balthasar": 90, "Casper": 50}),
			textMagi("Balthasar", "answer B", map[string]int{"Melchior": 60, "Casper": 40}),
			textMagi("Casper", "answer C", map[string]int{"Melchior": 70, "Balthasar": 80}),
		},
	}
	var lines []string
	magi.OnPhase = func(line string) { lines = append(lines, line) }

	if _, err := magi.Run(context.Background(), Request{Prompt: "hi"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("phase lines = %v, want 2", lines)
	}
	if !strings.Contains(lines[0], "Melchior") {
		t.Errorf("first line = %q, want the inference summary", lines[0])
	}
	if !strings.Contains(lines[1], "Balthasar") || !strings.Contains(lines[1], "wins") {
		t.Errorf("second line = %q, want the vote summary", lines[1])
	}
}

func TestMagiVotePromptExcludesJudge(t *testing.T) {
	responses := []Response{{Text: "A"}, {Text: "B"}, {Text: "C"}}
	names := []string{"Melchior", "Balthasar", "Casper"}

	prompt := votePrompt("Balthasar", names, responses)
	if strings.Contains(prompt, "--- Balthasar ---") {
		t.Errorf("vote prompt should not ask Balthasar to score itself:\n%s", prompt)
	}
	for _, want := range []string{"--- Melchior ---", "--- Casper ---", "A", "C"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("vote prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestParseScoresJSON(t *testing.T) {
	reply := `Here you go: {"Melchior": 88, "Casper": 71}`
	scores := parseScores(reply, []string{"Melchior", "Casper", "Balthasar"})
	if scores["Melchior"] != 88 || scores["Casper"] != 71 {
		t.Errorf("scores = %v", scores)
	}
	if _, ok := scores["Balthasar"]; ok {
		t.Errorf("unexpected score for Balthasar: %v", scores)
	}
}

func TestParseScoresFallback(t *testing.T) {
	reply := "Melchior: 88\nCasper: 71"
	scores := parseScores(reply, []string{"Melchior", "Casper"})
	if scores["Melchior"] != 88 || scores["Casper"] != 71 {
		t.Errorf("scores = %v", scores)
	}
}

// toolMagi answers with a tool call until the conversation ends in a tool
// result, then answers text. When judging, every other magi scores 100, so
// the first magi's answer always wins.
func toolMagi(name string) MagiMember {
	return MagiMember{
		Name:  name,
		Model: name,
		Loop: NewLoop(&funcProvider{respond: func(req Request) (Response, error) {
			if req.SystemPrompt == judgeSystemPrompt {
				b, _ := json.Marshal(map[string]int{"Melchior": 100, "Balthasar": 100, "Casper": 100})
				return Response{Text: string(b), Finished: true}, nil
			}
			if last, ok := lastRole(req.Messages); ok && last == RoleTool {
				return Response{Text: "final answer", Finished: true}, nil
			}
			return Response{Messages: []Message{{
				Role:          RoleAssistant,
				Name:          "echo",
				ToolCallID:    "call-1",
				ToolArguments: `{"value":"hi"}`,
			}}}, nil
		}}),
	}
}

func lastRole(messages []Message) (Role, bool) {
	if len(messages) == 0 {
		return "", false
	}
	return messages[len(messages)-1].Role, true
}

func TestMagiSessionExecutesWinningToolCall(t *testing.T) {
	// The session's own loop would fail if used; Magi Mode must not touch it.
	session := NewSession(NewLoop(&funcProvider{respond: func(Request) (Response, error) {
		return Response{}, errors.New("session loop must not be used in Magi Mode")
	}}))
	session.Magi = &Magi{Members: []MagiMember{
		toolMagi("Melchior"),
		toolMagi("Balthasar"),
		toolMagi("Casper"),
	}}

	var toolCalls int
	session.Tools = []Tool{{
		Name: "echo",
		Handler: func(ctx context.Context, args json.RawMessage) (string, error) {
			toolCalls++
			return "echoed", nil
		},
	}}

	var emitted strings.Builder
	session.Loop.OnChunk = func(chunk Chunk) error {
		emitted.WriteString(chunk.Delta)
		return nil
	}

	resp, err := session.Say(context.Background(), "please echo")
	if err != nil {
		t.Fatalf("Say: %v", err)
	}
	if resp.Text != "final answer" {
		t.Errorf("final text = %q, want %q", resp.Text, "final answer")
	}
	if toolCalls != 1 {
		t.Errorf("tool calls = %d, want 1", toolCalls)
	}
	if emitted.String() != "final answer" {
		t.Errorf("emitted text = %q, want %q", emitted.String(), "final answer")
	}
}
