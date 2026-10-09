package permissions

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/kncept/guacamole/config"
)

// fakeGranter returns scripted answers (defaulting to deny when exhausted)
// and records every question it was asked.
type fakeGranter struct {
	answers []config.Policy
	asks    [][2]string
}

func (g *fakeGranter) AskForAccess(category string, value string) config.Policy {
	g.asks = append(g.asks, [2]string{category, value})
	if len(g.answers) == 0 {
		return config.PolicyDeny
	}
	answer := g.answers[0]
	g.answers = g.answers[1:]
	return answer
}

// newManager builds a manager over a fresh config in a temp HOME (so Save
// never touches the real ~/.guac), then applies tweak to the config.
func newManager(t *testing.T, granter PermissionGranter, tweak func(*config.GConfig)) *PermissionsManager {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if tweak != nil {
		tweak(cfg)
	}
	return NewPermissionsManager(cfg, granter)
}

func TestFilesystemAllowAllOverridesRules(t *testing.T) {
	granter := &fakeGranter{}
	m := newManager(t, granter, func(cfg *config.GConfig) {
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
	if len(granter.asks) != 0 {
		t.Errorf("granter asked %v, want no questions", granter.asks)
	}
}

func TestFilesystemDirectoryRules(t *testing.T) {
	granter := &fakeGranter{}
	m := newManager(t, granter, func(cfg *config.GConfig) {
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
	if len(granter.asks) != 1 {
		t.Errorf("granter asked %v, want exactly one question (for /tmpfoo)", granter.asks)
	}
}

func TestFilesystemMostSpecificRuleWins(t *testing.T) {
	granter := &fakeGranter{}
	m := newManager(t, granter, func(cfg *config.GConfig) {
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
	if len(granter.asks) != 0 {
		t.Errorf("granter asked %v, want no questions", granter.asks)
	}
}

func TestFilesystemAskRecordsAnswer(t *testing.T) {
	granter := &fakeGranter{answers: []config.Policy{config.PolicyAllow}}
	m := newManager(t, granter, nil)

	allowed, err := m.IsAllowedPath("/home/me/notes", AccessRead)
	if err != nil {
		t.Fatalf("IsAllowedPath: %v", err)
	}
	if !allowed {
		t.Fatal("read allowed = false, want true (granter allowed)")
	}
	if want := [2]string{"filesystem read", "/home/me/notes"}; len(granter.asks) != 1 || granter.asks[0] != want {
		t.Errorf("granter asks = %v, want [%v]", granter.asks, want)
	}

	wantDirs := []config.DirectoryPermission{
		{Directory: "/home/me/notes", Read: config.PolicyAllow, Write: config.PolicyAsk},
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
	if want := [2]string{"filesystem write", "/home/me/notes"}; len(granter.asks) != 2 || granter.asks[1] != want {
		t.Errorf("granter asks = %v, want the write question second", granter.asks)
	}
}

func TestFilesystemAskAnswerAllowsOnceWithoutRecording(t *testing.T) {
	granter := &fakeGranter{answers: []config.Policy{config.PolicyAsk, config.PolicyAsk}}
	m := newManager(t, granter, nil)

	for i := 0; i < 2; i++ {
		allowed, err := m.IsAllowedPath("/tmp", AccessRead)
		if err != nil {
			t.Fatalf("IsAllowedPath: %v", err)
		}
		if !allowed {
			t.Errorf("call %d allowed = false, want true (allow this time)", i+1)
		}
	}
	if len(granter.asks) != 2 {
		t.Errorf("granter asked %d times, want 2 (an ask answer is never recorded)", len(granter.asks))
	}
	if len(m.config.Permissions.Filesystem.Directories) != 0 {
		t.Errorf("recorded directories = %v, want none", m.config.Permissions.Filesystem.Directories)
	}
}

func TestFilesystemBlankAnswerDenies(t *testing.T) {
	granter := &fakeGranter{answers: []config.Policy{""}}
	m := newManager(t, granter, nil)

	allowed, err := m.IsAllowedPath("/tmp", AccessWrite)
	if err != nil {
		t.Fatalf("IsAllowedPath: %v", err)
	}
	if allowed {
		t.Error("allowed = true, want false (closed prompt defaults to deny)")
	}
	wantDirs := []config.DirectoryPermission{
		{Directory: "/tmp", Read: config.PolicyAsk, Write: config.PolicyDeny},
	}
	if !reflect.DeepEqual(m.config.Permissions.Filesystem.Directories, wantDirs) {
		t.Errorf("recorded directories = %v, want %v", m.config.Permissions.Filesystem.Directories, wantDirs)
	}
}

func TestShellPolicyIsGeneric(t *testing.T) {
	granter := &fakeGranter{}
	for _, policy := range []config.Policy{config.PolicyAllow, config.PolicyDeny} {
		m := newManager(t, granter, func(cfg *config.GConfig) {
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
	if len(granter.asks) != 0 {
		t.Errorf("granter asked %v, want no questions for a settled policy", granter.asks)
	}
}

func TestShellAskRecordsAnswerForAllCommands(t *testing.T) {
	granter := &fakeGranter{answers: []config.Policy{config.PolicyAllow}}
	m := newManager(t, granter, nil)

	allowed, err := m.IsAllowedShellCommand("ls -la")
	if err != nil {
		t.Fatalf("IsAllowedShellCommand: %v", err)
	}
	if !allowed {
		t.Fatal("allowed = false, want true (granter allowed)")
	}
	if want := [2]string{"shell", "ls -la"}; len(granter.asks) != 1 || granter.asks[0] != want {
		t.Errorf("granter asks = %v, want [%v]", granter.asks, want)
	}
	if m.config.Permissions.Shell.Policy != config.PolicyAllow {
		t.Errorf("shell policy = %q, want the recorded %q", m.config.Permissions.Shell.Policy, config.PolicyAllow)
	}

	// One grant covers every shell command from now on.
	if allowed, _ := m.IsAllowedShellCommand("anything at all"); !allowed {
		t.Error("second command allowed = false, want the recorded policy")
	}
	if len(granter.asks) != 1 {
		t.Errorf("granter asked %d times, want 1", len(granter.asks))
	}
}

func TestWebAllowAllOverridesRules(t *testing.T) {
	granter := &fakeGranter{}
	m := newManager(t, granter, func(cfg *config.GConfig) {
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
	if len(granter.asks) != 0 {
		t.Errorf("granter asked %v, want no questions", granter.asks)
	}
}

func TestWebDomainRules(t *testing.T) {
	granter := &fakeGranter{}
	m := newManager(t, granter, func(cfg *config.GConfig) {
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
	if len(granter.asks) != 2 {
		t.Errorf("granter asks = %v, want one question per uncovered domain", granter.asks)
	}
	for i, want := range [][2]string{{"web", "notexample.com"}, {"web", "unknown.example"}} {
		if granter.asks[i] != want {
			t.Errorf("granter ask %d = %v, want %v", i, granter.asks[i], want)
		}
	}
}

func TestWebAskRecordsDomain(t *testing.T) {
	granter := &fakeGranter{answers: []config.Policy{config.PolicyAllow}}
	m := newManager(t, granter, nil)

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
	if len(granter.asks) != 1 {
		t.Errorf("granter asked %d times, want 1", len(granter.asks))
	}
}

func TestFilesystemPermissionsAccessor(t *testing.T) {
	want := config.FilesystemPermissions{
		AllowAll: false,
		Directories: []config.DirectoryPermission{
			{Directory: "/tmp", Read: config.PolicyAllow, Write: config.PolicyDeny},
		},
	}
	m := newManager(t, &fakeGranter{}, func(cfg *config.GConfig) {
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
	granter := &fakeGranter{answers: []config.Policy{config.PolicyAllow}}
	m := newManager(t, granter, nil)

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
		{Directory: absCWD, Read: config.PolicyAllow, Write: config.PolicyAsk},
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
	if len(granter.asks) != 1 {
		t.Errorf("granter asked %d times, want 1 (second call should use recorded rule)", len(granter.asks))
	}
}
