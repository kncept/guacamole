package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// judgeSystemPrompt is the system prompt sent to each magi during the voting
// phase, when it is asked to score the other magi's responses.
const judgeSystemPrompt = `You are an impartial judge evaluating the responses of other AI models to the same conversation. ` +
	`Score each response you are shown from 0 to 100, where 100 is the best possible response. ` +
	`Judge correctness, completeness, clarity, and usefulness. ` +
	`Respond with ONLY the requested score format and nothing else.`

// Magi is a panel of models that produce and judge each other's responses,
// keeping only the best. It replaces the single-model turn with a two-phase
// turn:
//
//  1. Inference — every magi answers the request's conversation.
//  2. Voting    — every magi scores the other magi's responses out of 100.
//  3. Collation — the scores are totaled per response and only the
//     highest-scoring response is kept.
//
// Run returns the kept response; the caller (Session) then executes it as
// usual — printing its text, running any tool calls, and looping on tool
// output.
//
// Magi is not safe for concurrent use: a conversation is ordered. A Magi
// built for a session is reused turn after turn.
type Magi struct {
	// Members are the magi, in display order. Each has its own Loop (and
	// therefore its own model). At least two are required.
	Members []MagiMember

	// OnPhase, when set, observes the panel at work with a short,
	// human-readable line per phase (inference, voting, collation). It is
	// called from the phase goroutines, so it must be safe for concurrent
	// use.
	OnPhase func(line string)
}

// MagiMember is one magi: a named model that both answers the conversation
// and judges the other magi's answers.
type MagiMember struct {
	// Name is the magi's display name (e.g. "Melchior").
	Name string
	// Loop runs the magi's exchanges. Required.
	Loop *Loop
	// Model names the model this magi uses; it is copied onto the magi's
	// requests.
	Model string
}

// Run executes one two-phase turn over req and returns the highest-scoring
// response. req carries the full conversation (Messages, which must end with
// the latest user or tool message), the SystemPrompt, and the Tools used for
// the inference phase; each magi answers with its own Model.
func (m *Magi) Run(ctx context.Context, req Request) (Response, error) {
	n := len(m.Members)
	if n < 2 {
		return Response{}, fmt.Errorf("ai: magi needs at least two members, got %d", n)
	}
	names := make([]string, n)
	for i, mem := range m.Members {
		names[i] = mem.Name
	}

	// Phase 1: inference — each magi answers the current input, in parallel.
	responses := make([]Response, n)
	err := runPhase(n, func(i int) (Response, error) {
		r := req
		r.Model = m.Members[i].Model
		return m.Members[i].Loop.Run(ctx, r)
	}, responses)
	if err != nil {
		return Response{}, fmt.Errorf("ai: magi inference failed: %w", err)
	}
	m.phase(fmt.Sprintf("answers in from %s", strings.Join(names, ", ")))

	// Phase 2: voting — each magi scores the other magi's responses.
	// votes[i] maps a member's name to member i's score of that member.
	votes := make([]map[string]int, n)
	err = runPhase(n, func(i int) (map[string]int, error) {
		others := make([]string, 0, n-1)
		for j := 0; j < n; j++ {
			if j != i {
				others = append(others, names[j])
			}
		}
		r := Request{
			Model:        m.Members[i].Model,
			SystemPrompt: judgeSystemPrompt,
			Prompt:       votePrompt(names[i], names, responses),
		}
		resp, err := m.Members[i].Loop.Run(ctx, r)
		if err != nil {
			return nil, err
		}
		return parseScores(resp.Text, others), nil
	}, votes)
	if err != nil {
		return Response{}, fmt.Errorf("ai: magi voting failed: %w", err)
	}

	// Phase 3: collation — total each magi's scores (from every other magi)
	// and keep the highest-scoring response. Ties keep the earlier magi.
	totals := make([]int, n)
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			if i == j {
				continue
			}
			totals[i] += votes[j][names[i]]
		}
	}
	best := 0
	for i := 1; i < n; i++ {
		if totals[i] > totals[best] {
			best = i
		}
	}
	m.phase(fmt.Sprintf("%s — %s wins", formatTotals(names, totals), names[best]))
	return responses[best], nil
}

// phase reports a phase line to OnPhase, if set.
func (m *Magi) phase(line string) {
	if m.OnPhase != nil {
		m.OnPhase(line)
	}
}

// runPhase runs fn(i) for every index in parallel, storing each result in
// out[i]. It waits for all of them and returns the first error encountered.
// fn must not modify shared state.
func runPhase[T any](count int, fn func(int) (T, error), out []T) error {
	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		first error
	)
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			v, err := fn(i)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if first == nil {
					first = err
				}
				return
			}
			out[i] = v
		}(i)
	}
	wg.Wait()
	return first
}

// formatTotals renders the per-magi totals as "Name total" pairs.
func formatTotals(names []string, totals []int) string {
	parts := make([]string, len(names))
	for i, name := range names {
		parts[i] = fmt.Sprintf("%s %d", name, totals[i])
	}
	return strings.Join(parts, ", ")
}

// votePrompt builds the judge prompt for the magi named judge: it lists the
// other magi's responses and asks for a 0-100 score for each, as a JSON
// object keyed by magi name.
func votePrompt(judge string, names []string, responses []Response) string {
	var b strings.Builder
	b.WriteString("Below are the responses the other magi gave to the same conversation.\n")
	for i, name := range names {
		if name == judge {
			continue
		}
		fmt.Fprintf(&b, "--- %s ---\n%s\n\n", name, renderResponse(responses[i]))
	}
	b.WriteString("Score each response from 0 to 100 (100 is best). ")
	b.WriteString("Respond with ONLY a JSON object mapping each magi's name to its integer score, ")
	b.WriteString(`for example {"Balthasar": 82, "Casper": 74}.` + "\n")
	return b.String()
}

// renderResponse renders a response for display in the voting prompt: its
// text, plus one line per tool call it requested.
func renderResponse(resp Response) string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(resp.Text))
	for _, msg := range resp.Messages {
		if msg.Role == RoleAssistant && msg.Name != "" {
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			fmt.Fprintf(&b, "[tool call: %s %s]", msg.Name, msg.ToolArguments)
		}
	}
	if b.Len() == 0 {
		return "(empty response)"
	}
	return b.String()
}

// parseScores extracts a magi-name → score map from a judge's reply. It first
// tries the JSON object the prompt asked for; when that fails it scans for
// each expected name followed by a number. Names the judge left out are
// absent from the result (and count as 0 when totaled).
func parseScores(reply string, names []string) map[string]int {
	scores := map[string]int{}

	if obj := extractJSONObject(reply); obj != "" {
		var raw map[string]any
		if err := json.Unmarshal([]byte(obj), &raw); err == nil {
			for _, name := range names {
				if v, ok := raw[name]; ok {
					scores[name] = toInt(v)
				}
			}
			if len(scores) > 0 {
				return scores
			}
		}
	}

	for _, name := range names {
		if n, ok := scoreAfterName(reply, name); ok {
			scores[name] = n
		}
	}
	return scores
}

// extractJSONObject returns the first balanced {...} block in s, or "" when
// there is none.
func extractJSONObject(s string) string {
	start := strings.IndexByte(s, '{')
	if start < 0 {
		return ""
	}
	depth := 0
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}

// toInt coerces a decoded JSON score value to an int.
func toInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case string:
		i, _ := strconv.Atoi(strings.TrimSpace(n))
		return i
	}
	return 0
}

// scoreAfterName finds name in reply and returns the first integer that
// follows it, for judges that ignore the JSON instruction.
func scoreAfterName(reply, name string) (int, bool) {
	idx := strings.Index(reply, name)
	if idx < 0 {
		return 0, false
	}
	rest := reply[idx+len(name):]
	if len(rest) > 40 {
		rest = rest[:40]
	}
	loc := firstNumber.FindStringIndex(rest)
	if loc == nil {
		return 0, false
	}
	n, err := strconv.Atoi(rest[loc[0]:loc[1]])
	if err != nil {
		return 0, false
	}
	return n, true
}

// firstNumber matches the first integer in a string.
var firstNumber = regexp.MustCompile(`-?\d+`)
