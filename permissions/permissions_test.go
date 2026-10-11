package permissions

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/kncept/guacamole/config"
	"github.com/kncept/guacamole/userinput"
)

// fakeInput returns scripted Prompt answers (denying when exhausted) and
// records every question it was asked. A scripted policy selects the first
// offered option that carries it — for a filesystem check that is the
// value-scoped option, e.g. "Always: Always allow file". config.PolicyAsk
// selects the one-time option (the unrecorded sentinel). Any other value
// (including "") is returned verbatim, so a test can name a specific option
// such as optionFileAsk.
type fakeInput struct {
	answers []config.Policy
	asks    []string
	options [][]string
}

var _ userinput.RequestForegroundUserInput = (*fakeInput)(nil)

func (f *fakeInput) Prompt(text string, allowFree bool, options []string) (string, error) {
	f.asks = append(f.asks, text)
	f.options = append(f.options, options)
	if len(f.answers) == 0 {
		return optionFor(options, config.PolicyDeny), nil
	}
	answer := f.answers[0]
	f.answers = f.answers[1:]
	switch answer {
	case config.PolicyAllow, config.PolicyDeny, config.PolicyAsk:
		return optionFor(options, answer), nil
	default:
		return string(answer), nil // "" scripts a closed prompt
	}
}

// optionFor returns the offered option the fake picks for a scripted policy.
// config.PolicyAsk picks the one-time option rather than the explicit
// "always ask" option; a test that wants the latter names it directly.
func optionFor(options []string, policy config.Policy) string {
	if policy == config.PolicyAsk {
		for _, option := range options {
			if option == optionDirectoryOnce || option == optionFileOnce || option == optionAllowOnce {
				return option
			}
		}
	}
	for _, option := range options {
		if policyOf(option) == policy {
			return option
		}
	}
	return ""
}

// policyOf reports the policy an option carries. The one-time options and
// the explicit "always ask" options all map to config.PolicyAsk.
func policyOf(option string) config.Policy {
	switch option {
	case optionDirectoryDeny, optionFileDenyFile, optionFileDenyDirectory, optionDenyForever:
		return config.PolicyDeny
	case optionDirectoryAllow, optionFileAllowFile, optionFileAllowDirectory, optionAllowAlways:
		return config.PolicyAllow
	case optionDirectoryAsk, optionFileAsk, optionDirectoryOnce, optionFileOnce, optionAllowOnce:
		return config.PolicyAsk
	default:
		return ""
	}
}

// contains reports whether want is one of the offered options.
func contains(options []string, want string) bool {
	for _, option := range options {
		if option == want {
			return true
		}
	}
	return false
}

// askText formats the question the manager puts to the user input for a
// permission check.
func askText(category, value string) string {
	return fmt.Sprintf("Allow %s access to %s?", category, value)
}

// newManager builds a manager over a fresh config in a temp HOME (so Save
// never touches the real ~/.guac), then applies tweak to the config.
func newManager(t *testing.T, input userinput.RequestForegroundUserInput, tweak func(*config.GConfig)) *PermissionsManager {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if tweak != nil {
		tweak(cfg)
	}
	return NewPermissionsManager(cfg, input)
}

func TestFilesystemAllowAllOverridesRules(t *testing.T) {
	input := &fakeInput{}
	m := newManager(t, input, func(cfg *config.GConfig) {
		cfg.Permissions.Filesystem.AllowAll = true
		cfg.Permissions.Filesystem.Directories = []config.DirectoryPermission{
			{Directory: "/etc", Read: config.PolicyDeny, Write: config.PolicyDeny},
		}
	})

	for _, access := range []AccessKind{AccessRead, AccessWrite} {
		allowed, err := m.IsAllowedPath("/etc/passwd", access)
		if err != nil {
			t.Fatalf("IsAllowedPath: %v", err)
		}
		if !allowed {
			t.Errorf("access %v allowed = false, want true (allow-all override)", access)
		}
	}
	if len(input.asks) != 0 {
		t.Errorf("granter asked %v, want no questions", input.asks)
	}
}

func TestFilesystemDirectoryRules(t *testing.T) {
	input := &fakeInput{}
	m := newManager(t, input, func(cfg *config.GConfig) {
		cfg.Permissions.Filesystem.Directories = []config.DirectoryPermission{
			{Directory: "/tmp", Read: config.PolicyAllow, Write: config.PolicyAllow},
			{Directory: "/etc", Read: config.PolicyDeny, Write: config.PolicyDeny},
		}
	})

	cases := []struct {
		path   string
		access AccessKind
		want   bool
	}{
		{"/tmp", AccessRead, true},
		{"/tmp/project/file.txt", AccessWrite, true},
		{"/etc", AccessRead, false},
		{"/etc/ssh/sshd_config", AccessWrite, false},
		// /tmp must not cover the sibling /tmpfoo.
		{"/tmpfoo", AccessRead, false},
	}
	for _, c := range cases {
		allowed, err := m.IsAllowedPath(c.path, c.access)
		if err != nil {
			t.Fatalf("IsAllowedPath(%s): %v", c.path, err)
		}
		if allowed != c.want {
			t.Errorf("IsAllowedPath(%s, %v) = %v, want %v", c.path, c.access, allowed, c.want)
		}
	}
	if len(input.asks) != 1 {
		t.Errorf("granter asked %v, want exactly one question (for /tmpfoo)", input.asks)
	}
}

func TestFilesystemMostSpecificRuleWins(t *testing.T) {
	input := &fakeInput{}
	m := newManager(t, input, func(cfg *config.GConfig) {
		cfg.Permissions.Filesystem.Directories = []config.DirectoryPermission{
			{Directory: "/project", Read: config.PolicyAllow, Write: config.PolicyAllow},
			{Directory: "/project/secret", Read: config.PolicyDeny, Write: config.PolicyDeny},
		}
	})

	allowed, err := m.IsAllowedPath("/project/secret/keys.pem", AccessRead)
	if err != nil {
		t.Fatalf("IsAllowedPath: %v", err)
	}
	if allowed {
		t.Error("read of /project/secret/keys.pem allowed = true, want false (more specific rule)")
	}
	if len(input.asks) != 0 {
		t.Errorf("granter asked %v, want no questions", input.asks)
	}
}

func TestFilesystemAskRecordsAnswer(t *testing.T) {
	input := &fakeInput{answers: []config.Policy{config.PolicyAllow}}
	m := newManager(t, input, nil)

	allowed, err := m.IsAllowedPath("/home/me/notes", AccessRead)
	if err != nil {
		t.Fatalf("IsAllowedPath: %v", err)
	}
	if !allowed {
		t.Fatal("read allowed = false, want true (granter allowed)")
	}
	if want := askText("filesystem read", "/home/me/notes"); len(input.asks) != 1 || input.asks[0] != want {
		t.Errorf("granter asks = %v, want [%v]", input.asks, want)
	}

	wantDirs := []config.DirectoryPermission{
		{Directory: "/home/me/notes", Read: config.PolicyAllow},
	}
	if !reflect.DeepEqual(m.config.Permissions.Filesystem.Directories, wantDirs) {
		t.Errorf("recorded directories = %v, want %v", m.config.Permissions.Filesystem.Directories, wantDirs)
	}

	// The read grant must be honoured without asking again, while write
	// still falls through to the granter (which now denies by default).
	if allowed, _ := m.IsAllowedPath("/home/me/notes/todo.md", AccessRead); !allowed {
		t.Error("second read allowed = false, want the recorded grant")
	}
	allowed, err = m.IsAllowedPath("/home/me/notes", AccessWrite)
	if err != nil {
		t.Fatalf("IsAllowedPath(write): %v", err)
	}
	if allowed {
		t.Error("write allowed = true, want false (write is still ask, granter denies)")
	}
	if want := askText("filesystem write", "/home/me/notes"); len(input.asks) != 2 || input.asks[1] != want {
		t.Errorf("granter asks = %v, want the write question second", input.asks)
	}
}

func TestFilesystemAskAnswerAllowsOnceWithoutRecording(t *testing.T) {
	input := &fakeInput{answers: []config.Policy{config.PolicyAsk, config.PolicyAsk}}
	m := newManager(t, input, nil)

	for i := 0; i < 2; i++ {
		allowed, err := m.IsAllowedPath("/tmp", AccessRead)
		if err != nil {
			t.Fatalf("IsAllowedPath: %v", err)
		}
		if !allowed {
			t.Errorf("call %d allowed = false, want true (allow this time)", i+1)
		}
	}
	if len(input.asks) != 2 {
		t.Errorf("granter asked %d times, want 2 (an ask answer is never recorded)", len(input.asks))
	}
	if len(m.config.Permissions.Filesystem.Directories) != 0 {
		t.Errorf("recorded directories = %v, want none", m.config.Permissions.Filesystem.Directories)
	}
}

func TestFilesystemBlankAnswerDenies(t *testing.T) {
	input := &fakeInput{answers: []config.Policy{""}}
	m := newManager(t, input, nil)

	allowed, err := m.IsAllowedPath("/tmp", AccessWrite)
	if err != nil {
		t.Fatalf("IsAllowedPath: %v", err)
	}
	if allowed {
		t.Error("allowed = true, want false (closed prompt defaults to deny)")
	}
	wantDirs := []config.DirectoryPermission{
		{Directory: "/tmp", Write: config.PolicyDeny},
	}
	if !reflect.DeepEqual(m.config.Permissions.Filesystem.Directories, wantDirs) {
		t.Errorf("recorded directories = %v, want %v", m.config.Permissions.Filesystem.Directories, wantDirs)
	}
}

func TestFilesystemFileOffersFileAndDirectoryScopes(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(file, []byte("hi"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	input := &fakeInput{answers: []config.Policy{config.Policy(optionFileAllowDirectory)}}
	m := newManager(t, input, nil)

	allowed, err := m.IsAllowedPath(file, AccessRead)
	if err != nil {
		t.Fatalf("IsAllowedPath: %v", err)
	}
	if !allowed {
		t.Fatal("read allowed = false, want true (always allow directory)")
	}
	if len(input.options) != 1 {
		t.Fatalf("prompts = %v, want exactly one", input.options)
	}
	wantOptions := []string{
		optionFileDenyFile,
		optionFileDenyDirectory,
		optionFileAsk,
		optionFileOnce,
		optionFileAllowFile,
		optionFileAllowDirectory,
	}
	if !reflect.DeepEqual(input.options[0], wantOptions) {
		t.Errorf("file prompt options = %v, want %v", input.options[0], wantOptions)
	}

	// "Always: Always allow directory" records the containing directory,
	// so a sibling is covered without another prompt.
	wantDirs := []config.DirectoryPermission{
		{Directory: dir, Read: config.PolicyAllow},
	}
	if !reflect.DeepEqual(m.config.Permissions.Filesystem.Directories, wantDirs) {
		t.Errorf("recorded directories = %v, want %v", m.config.Permissions.Filesystem.Directories, wantDirs)
	}
	if allowed, _ := m.IsAllowedPath(filepath.Join(dir, "other.txt"), AccessRead); !allowed {
		t.Error("sibling read allowed = false, want the recorded directory grant")
	}
	if len(input.asks) != 1 {
		t.Errorf("granter asked %d times, want 1", len(input.asks))
	}
}

func TestFilesystemFileAllowFileRecordsOnlyThatFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(file, []byte("hi"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	input := &fakeInput{answers: []config.Policy{config.PolicyAllow}}
	m := newManager(t, input, nil)

	if allowed, _ := m.IsAllowedPath(file, AccessRead); !allowed {
		t.Fatal("read allowed = false, want true (always allow file)")
	}
	wantDirs := []config.DirectoryPermission{
		{Directory: file, Read: config.PolicyAllow},
	}
	if !reflect.DeepEqual(m.config.Permissions.Filesystem.Directories, wantDirs) {
		t.Errorf("recorded directories = %v, want %v", m.config.Permissions.Filesystem.Directories, wantDirs)
	}
}

func TestFilesystemFileDenyDirectoryRecordsParent(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(file, []byte("hi"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	input := &fakeInput{answers: []config.Policy{config.Policy(optionFileDenyDirectory)}}
	m := newManager(t, input, nil)

	if allowed, _ := m.IsAllowedPath(file, AccessRead); allowed {
		t.Error("read allowed = true, want false (always deny directory)")
	}
	wantDirs := []config.DirectoryPermission{
		{Directory: dir, Read: config.PolicyDeny},
	}
	if !reflect.DeepEqual(m.config.Permissions.Filesystem.Directories, wantDirs) {
		t.Errorf("recorded directories = %v, want %v", m.config.Permissions.Filesystem.Directories, wantDirs)
	}
	// The recorded directory rule covers the file too, so no second prompt.
	if allowed, _ := m.IsAllowedPath(file, AccessRead); allowed {
		t.Error("second read allowed = true, want false (recorded deny)")
	}
	if len(input.asks) != 1 {
		t.Errorf("granter asked %d times, want 1", len(input.asks))
	}
}

func TestFilesystemFileAskRecordsAskRule(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(file, []byte("hi"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	input := &fakeInput{answers: []config.Policy{
		config.Policy(optionFileAsk),
		config.Policy(optionFileAsk),
	}}
	m := newManager(t, input, nil)

	for i := 0; i < 2; i++ {
		if allowed, _ := m.IsAllowedPath(file, AccessRead); !allowed {
			t.Errorf("call %d allowed = false, want true (always ask allowed after asking)", i+1)
		}
	}
	wantDirs := []config.DirectoryPermission{
		{Directory: file, Read: config.PolicyAsk},
	}
	if !reflect.DeepEqual(m.config.Permissions.Filesystem.Directories, wantDirs) {
		t.Errorf("recorded directories = %v, want %v (always ask is recorded)", m.config.Permissions.Filesystem.Directories, wantDirs)
	}
	if len(input.asks) != 2 {
		t.Errorf("granter asked %d times, want 2 (an ask rule still prompts)", len(input.asks))
	}
}

func TestFilesystemDirectoryAskRecordsAskRule(t *testing.T) {
	dir := t.TempDir()
	input := &fakeInput{answers: []config.Policy{
		config.Policy(optionDirectoryAsk),
		config.Policy(optionDirectoryAsk),
	}}
	m := newManager(t, input, nil)

	for i := 0; i < 2; i++ {
		if allowed, _ := m.IsAllowedPath(dir, AccessRead); !allowed {
			t.Errorf("call %d allowed = false, want true (always ask allowed after asking)", i+1)
		}
	}
	wantDirs := []config.DirectoryPermission{
		{Directory: dir, Read: config.PolicyAsk},
	}
	if !reflect.DeepEqual(m.config.Permissions.Filesystem.Directories, wantDirs) {
		t.Errorf("recorded directories = %v, want %v (always ask is recorded)", m.config.Permissions.Filesystem.Directories, wantDirs)
	}
	if len(input.asks) != 2 {
		t.Errorf("granter asked %d times, want 2 (an ask rule still prompts)", len(input.asks))
	}
}

func TestFilesystemFileThisTimeRecordsNothing(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(file, []byte("hi"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	input := &fakeInput{answers: []config.Policy{
		config.Policy(optionFileOnce),
		config.Policy(optionFileOnce),
	}}
	m := newManager(t, input, nil)

	for i := 0; i < 2; i++ {
		if allowed, _ := m.IsAllowedPath(file, AccessRead); !allowed {
			t.Errorf("call %d allowed = false, want true (this time)", i+1)
		}
	}
	if len(input.asks) != 2 {
		t.Errorf("granter asked %d times, want 2 (this time is never recorded)", len(input.asks))
	}
	if len(m.config.Permissions.Filesystem.Directories) != 0 {
		t.Errorf("recorded directories = %v, want none", m.config.Permissions.Filesystem.Directories)
	}
}

func TestFilesystemDirectoryPromptOffersDirectoryOptions(t *testing.T) {
	dir := t.TempDir()
	input := &fakeInput{answers: []config.Policy{config.PolicyAllow}}
	m := newManager(t, input, nil)

	if allowed, _ := m.IsAllowedPath(dir, AccessRead); !allowed {
		t.Fatal("read allowed = false, want true (always allow)")
	}
	if len(input.options) != 1 {
		t.Fatalf("prompts = %v, want exactly one", input.options)
	}
	wantOptions := []string{
		optionDirectoryDeny,
		optionDirectoryAsk,
		optionDirectoryOnce,
		optionDirectoryAllow,
	}
	if !reflect.DeepEqual(input.options[0], wantOptions) {
		t.Errorf("directory prompt options = %v, want %v", input.options[0], wantOptions)
	}
}

// TestFilesystemParentResponseAvoidsPrompt checks that once a parent
// directory has answered an access kind, checks below it use that answer
// without asking again.
func TestFilesystemParentResponseAvoidsPrompt(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "sub", "notes.txt")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(file, []byte("hi"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	input := &fakeInput{}
	m := newManager(t, input, func(cfg *config.GConfig) {
		cfg.Permissions.Filesystem.Directories = []config.DirectoryPermission{
			{Directory: dir, Read: config.PolicyAllow, Write: config.PolicyDeny},
		}
	})

	if allowed, _ := m.IsAllowedPath(file, AccessRead); !allowed {
		t.Error("read allowed = false, want true (inherited parent allow)")
	}
	if allowed, _ := m.IsAllowedPath(file, AccessWrite); allowed {
		t.Error("write allowed = true, want false (inherited parent deny)")
	}
	if len(input.asks) != 0 {
		t.Errorf("granter asked %v, want no questions (parent already answered)", input.asks)
	}
}

// TestFilesystemChildOverridesParent checks that a more specific rule wins
// for the access kind it answered, while the other kind still inherits from
// the parent.
func TestFilesystemChildOverridesParent(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(file, []byte("hi"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	input := &fakeInput{}
	m := newManager(t, input, func(cfg *config.GConfig) {
		cfg.Permissions.Filesystem.Directories = []config.DirectoryPermission{
			{Directory: dir, Read: config.PolicyAllow, Write: config.PolicyAllow},
			{Directory: file, Read: config.PolicyDeny},
		}
	})

	if allowed, _ := m.IsAllowedPath(file, AccessRead); allowed {
		t.Error("read allowed = true, want false (child deny overrides parent allow)")
	}
	if allowed, _ := m.IsAllowedPath(file, AccessWrite); !allowed {
		t.Error("write allowed = false, want true (child did not answer write, parent allow applies)")
	}
	if len(input.asks) != 0 {
		t.Errorf("granter asked %v, want no questions (both answers resolved from rules)", input.asks)
	}
}

// TestFilesystemChildRuleDoesNotShadowParentOtherAccess checks that a rule
// answering one access kind does not silently answer the other kind with an
// ask, which would shadow the parent's response for that other kind.
func TestFilesystemChildRuleDoesNotShadowParentOtherAccess(t *testing.T) {
	dir := t.TempDir()
	other := filepath.Join(dir, "other.txt")
	if err := os.WriteFile(other, []byte("hi"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	input := &fakeInput{}
	m := newManager(t, input, func(cfg *config.GConfig) {
		cfg.Permissions.Filesystem.Directories = []config.DirectoryPermission{
			{Directory: dir, Read: config.PolicyAllow},
			{Directory: other, Write: config.PolicyAllow},
		}
	})

	if allowed, _ := m.IsAllowedPath(other, AccessRead); !allowed {
		t.Error("read allowed = false, want true (child write rule must not shadow parent read)")
	}
	if allowed, _ := m.IsAllowedPath(other, AccessWrite); !allowed {
		t.Error("write allowed = false, want true (child write rule)")
	}
	if len(input.asks) != 0 {
		t.Errorf("granter asked %v, want no questions", input.asks)
	}
}

func TestShellPolicyIsGeneric(t *testing.T) {
	input := &fakeInput{}
	for _, policy := range []config.Policy{config.PolicyAllow, config.PolicyDeny} {
		m := newManager(t, input, func(cfg *config.GConfig) {
			cfg.Permissions.Shell.Policy = policy
		})

		allowed, err := m.IsAllowedShellCommand("rm -rf /tmp/whatever")
		if err != nil {
			t.Fatalf("IsAllowedShellCommand: %v", err)
		}
		if allowed != (policy == config.PolicyAllow) {
			t.Errorf("policy %q: allowed = %v", policy, allowed)
		}
	}
	if len(input.asks) != 0 {
		t.Errorf("granter asked %v, want no questions for a settled policy", input.asks)
	}
}

func TestShellAskRecordsAnswerForAllCommands(t *testing.T) {
	input := &fakeInput{answers: []config.Policy{config.PolicyAllow}}
	m := newManager(t, input, nil)

	allowed, err := m.IsAllowedShellCommand("ls -la")
	if err != nil {
		t.Fatalf("IsAllowedShellCommand: %v", err)
	}
	if !allowed {
		t.Fatal("allowed = false, want true (granter allowed)")
	}
	if want := askText("shell", "ls -la"); len(input.asks) != 1 || input.asks[0] != want {
		t.Errorf("granter asks = %v, want [%v]", input.asks, want)
	}
	if m.config.Permissions.Shell.Policy != config.PolicyAllow {
		t.Errorf("shell policy = %q, want the recorded %q", m.config.Permissions.Shell.Policy, config.PolicyAllow)
	}

	// One grant covers every shell command from now on.
	if allowed, _ := m.IsAllowedShellCommand("anything at all"); !allowed {
		t.Error("second command allowed = false, want the recorded policy")
	}
	if len(input.asks) != 1 {
		t.Errorf("granter asked %d times, want 1", len(input.asks))
	}
}

func TestWebAllowAllOverridesRules(t *testing.T) {
	input := &fakeInput{}
	m := newManager(t, input, func(cfg *config.GConfig) {
		cfg.Permissions.Web.AllowAll = true
		cfg.Permissions.Web.Domains = []config.DomainPermission{
			{Domain: "evil.example", Policy: config.PolicyDeny},
		}
	})

	allowed, err := m.IsAllowedDomain("evil.example")
	if err != nil {
		t.Fatalf("IsAllowedDomain: %v", err)
	}
	if !allowed {
		t.Error("allowed = false, want true (allow-all override)")
	}
	if len(input.asks) != 0 {
		t.Errorf("granter asked %v, want no questions", input.asks)
	}
}

func TestWebDomainRules(t *testing.T) {
	input := &fakeInput{}
	m := newManager(t, input, func(cfg *config.GConfig) {
		cfg.Permissions.Web.Domains = []config.DomainPermission{
			{Domain: "example.com", Policy: config.PolicyAllow},
			{Domain: "blocked.example", Policy: config.PolicyDeny},
		}
	})

	cases := []struct {
		domain string
		want   bool
	}{
		{"example.com", true},
		{"api.example.com", true}, // subdomains match
		{"notexample.com", false}, // suffix without a dot boundary does not match
		{"blocked.example", false},
		{"unknown.example", false}, // no rule: granter denies by default
	}
	for _, c := range cases {
		allowed, err := m.IsAllowedDomain(c.domain)
		if err != nil {
			t.Fatalf("IsAllowedDomain(%s): %v", c.domain, err)
		}
		if allowed != c.want {
			t.Errorf("IsAllowedDomain(%s) = %v, want %v", c.domain, allowed, c.want)
		}
	}
	if len(input.asks) != 2 {
		t.Errorf("granter asks = %v, want one question per uncovered domain", input.asks)
	}
	for i, want := range []string{askText("web", "notexample.com"), askText("web", "unknown.example")} {
		if input.asks[i] != want {
			t.Errorf("input ask %d = %v, want %v", i, input.asks[i], want)
		}
	}
}

func TestWebAskRecordsDomain(t *testing.T) {
	input := &fakeInput{answers: []config.Policy{config.PolicyAllow}}
	m := newManager(t, input, nil)

	if allowed, _ := m.IsAllowedDomain("docs.example.org"); !allowed {
		t.Fatal("allowed = false, want true (granter allowed)")
	}
	want := []config.DomainPermission{{Domain: "docs.example.org", Policy: config.PolicyAllow}}
	if !reflect.DeepEqual(m.config.Permissions.Web.Domains, want) {
		t.Errorf("recorded domains = %v, want %v", m.config.Permissions.Web.Domains, want)
	}

	if allowed, _ := m.IsAllowedDomain("docs.example.org"); !allowed {
		t.Error("second check allowed = false, want the recorded grant")
	}
	if len(input.asks) != 1 {
		t.Errorf("granter asked %d times, want 1", len(input.asks))
	}
}

func TestFilesystemPermissionsAccessor(t *testing.T) {
	want := config.FilesystemPermissions{
		AllowAll: false,
		Directories: []config.DirectoryPermission{
			{Directory: "/tmp", Read: config.PolicyAllow, Write: config.PolicyDeny},
		},
	}
	m := newManager(t, &fakeInput{}, func(cfg *config.GConfig) {
		cfg.Permissions.Filesystem = want
	})

	got := m.FilesystemPermissions()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("FilesystemPermissions() = %v, want %v", got, want)
	}
}

func TestDomainCovers(t *testing.T) {
	cases := []struct {
		rule   string
		domain string
		want   bool
	}{
		{"", "anything.example", true},
		{"*", "anything.example", true},
		{"example.com", "example.com", true},
		{"example.com", "api.example.com", true},
		{"example.com", "API.EXAMPLE.COM", true},
		{"*.example.com", "api.example.com", true},
		{"example.com", "notexample.com", false},
		{"ample.com", "example.com", false},
	}
	for _, c := range cases {
		if got := domainCovers(c.rule, c.domain); got != c.want {
			t.Errorf("domainCovers(%q, %q) = %v, want %v", c.rule, c.domain, got, c.want)
		}
	}
}

func TestFilesystemRelativePathConvertedToAbsolute(t *testing.T) {
	// This test verifies that relative paths (like ".") are resolved to absolute paths
	// before checking permissions and recording decisions.
	input := &fakeInput{answers: []config.Policy{config.PolicyAllow}}
	m := newManager(t, input, nil)

	// Get the current working directory as absolute path
	absCWD, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("filepath.Abs: %v", err)
	}

	// Use "." as the path - it should be converted to absolute
	allowed, err := m.IsAllowedPath(".", AccessRead)
	if err != nil {
		t.Fatalf("IsAllowedPath: %v", err)
	}
	if !allowed {
		t.Fatal("read allowed = false, want true (granter allowed)")
	}

	// The recorded directory should be the absolute path, not "."
	wantDirs := []config.DirectoryPermission{
		{Directory: absCWD, Read: config.PolicyAllow},
	}
	if !reflect.DeepEqual(m.config.Permissions.Filesystem.Directories, wantDirs) {
		t.Errorf("recorded directories = %v, want %v (should use absolute path, not '.')", m.config.Permissions.Filesystem.Directories, wantDirs)
	}

	// Now verify that using the absolute path directly also works (no new prompt)
	allowed, err = m.IsAllowedPath(absCWD, AccessRead)
	if err != nil {
		t.Fatalf("IsAllowedPath(absPath): %v", err)
	}
	if !allowed {
		t.Error("second read with absolute path allowed = false, want true")
	}
	if len(input.asks) != 1 {
		t.Errorf("granter asked %d times, want 1 (second call should use recorded rule)", len(input.asks))
	}
}
