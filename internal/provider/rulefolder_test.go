package provider

import (
	"slices"
	"strings"
	"testing"
)

func folderEnv(t *testing.T, fold bool, home string) {
	t.Helper()
	oldFold, oldHome := foldCase, ruleHome
	foldCase, ruleHome = fold, func() string { return home }
	t.Cleanup(func() { foldCase, ruleHome = oldFold, oldHome })
}

// A folder is under a rule's folder at a path's boundary only, ~ is the
// home folder, a Windows drive is one with its WSL mount and a WSL
// distro's share with the distro's own path, and case counts only where
// the system's folders have it.
func TestFolderUnder(t *testing.T) {
	folderEnv(t, false, "/home/me")
	for _, tc := range []struct {
		folder, root string
		want         bool
	}{
		{"/a/b", "/a/b", true},
		{"/a/b/c", "/a/b", true},
		{"/a/b/", "/a/b", true},
		{"/a/b", "/a/b/", true},
		{"/a/bc", "/a/b", false},
		{"/a", "/a/b", false},
		{"/a/b/../x", "/a/b", false},
		{"/A/B", "/a/b", false}, // Linux: case counts
		{"/home/me/work/acme", "~/work", true},
		{"/home/me/work", "~", true},
		{"/home/mex", "~", false},
		{"/home/me/x", "~/", true},
		{`C:\Users\me\work\acme`, `C:\Users\me\work`, true},
		{`c:\users\ME\work\acme`, `C:\Users\me\work`, true}, // a drive: no case, from either side
		{"/mnt/c/Users/me/work/acme", `C:\Users\me\work`, true},
		{`C:\Users\me\work`, "/mnt/c/users/me/work", true},
		{"/mnt/c/Users/me/workx", `C:\Users\me\work`, false},
		{"/home/me/proj", `\\wsl$\Ubuntu\home\me`, true},
		{"/home/me/proj", `\\wsl.localhost\Ubuntu-24.04\home\me\proj`, true},
		{"/x", "/", true},
		{"", "/", false}, // the agent didn't say: no rule matches it
		{"", "~", false},
		{"relative/x", "/", false},
	} {
		if got := FolderUnder(tc.folder, tc.root); got != tc.want {
			t.Errorf("FolderUnder(%q, %q) = %v, want %v", tc.folder, tc.root, got, tc.want)
		}
	}
	folderEnv(t, true, "/Users/Me")
	for _, tc := range []struct {
		folder, root string
		want         bool
	}{
		{"/users/me/Work/Acme", "~/work", true}, // macOS, Windows: no case
		{"/Users/Me/Workshop", "~/work", false},
	} {
		if got := FolderUnder(tc.folder, tc.root); got != tc.want {
			t.Errorf("fold: FolderUnder(%q, %q) = %v, want %v", tc.folder, tc.root, got, tc.want)
		}
	}
	folderEnv(t, true, "")
	if FolderUnder("/x/y", "~") {
		t.Error("~ with no home folder known matched")
	}
}

// A rule's folders are whole paths, kept as typed but for a trailing
// slash; a pause rule can't have them; folders alone are a condition.
func TestRuleFoldersCleaned(t *testing.T) {
	members := []string{"a/m", "b/m"}
	out, err := cleanRules([]Rule{{Use: "b/m", Folders: []string{" ~/work/acme/ ", "", "~/work/acme", `C:\work\`, "/", `C:\`}}}, members)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"~/work/acme", `C:\work`, "/", `C:\`}; !slices.Equal(out[0].Folders, want) {
		t.Errorf("folders %q, want %q", out[0].Folders, want)
	}
	if c := out[0].Conditions(); !slices.Equal(c, []string{`folder ~/work/acme|C:\work|/|C:\`}) {
		t.Errorf("conditions %q", c)
	}
	for _, bad := range []Rule{
		{Use: "b/m", Folders: []string{"work/acme"}},
		{Use: "b/m", Folders: []string{"~work"}},
		{Use: "b/m", Pause: true, Folders: []string{"/x"}},
		{Use: "b/m", Folders: []string{" "}}, // nothing left: no condition
	} {
		if _, err := cleanRules([]Rule{bad}, members); err == nil {
			t.Errorf("%+v was taken", bad)
		}
	}
}

// A rule by folder matches a request from under one of its folders, with
// its other conditions, and never one whose folder isn't known.
func TestRuleMatchesByFolder(t *testing.T) {
	folderEnv(t, false, "/home/me")
	r := Rule{Use: "b/m", Folders: []string{"~/work/acme", "/srv/oss"}}
	for _, tc := range []struct {
		q    RuleRequest
		want bool
	}{
		{RuleRequest{Folder: "/home/me/work/acme"}, true},
		{RuleRequest{Folder: "/home/me/work/acme/web"}, true},
		{RuleRequest{Folder: "/srv/oss/x"}, true},
		{RuleRequest{Folder: "/home/me/work/acme-old"}, false},
		{RuleRequest{Folder: "/home/me/work"}, false},
		{RuleRequest{}, false},
	} {
		if got := r.Matches(tc.q); got != tc.want {
			t.Errorf("%+v: %v", tc.q, got)
		}
	}
	both := Rule{Use: "b/m", Folders: []string{"/srv/oss"}, Tokens: 1000}
	if both.Matches(RuleRequest{Folder: "/srv/oss", Tokens: 10}) || !both.Matches(RuleRequest{Folder: "/srv/oss", Tokens: 2000}) {
		t.Error("a folder and a length: both must hold")
	}
	if MatchRule([]Rule{{Use: "a/m", Agents: []string{"codex"}}, r}, RuleRequest{Agent: "claude", Folder: "/srv/oss"}) != 1 {
		t.Error("the rule by folder after one by agent")
	}
}

// folders= is typed as other words are, comma separated, quoted when a
// folder has a space, and read back the same.
func TestParseRuleFolders(t *testing.T) {
	g := Group{ID: "g", Members: []string{"a/m", "b/x"}}
	r, _, _, err := ParseRule(g, RuleWords(`use=x folders="~/My Project,/srv/oss/" tokens=10k`))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(r.Folders, []string{"~/My Project", "/srv/oss"}) {
		t.Fatalf("folders %q", r.Folders)
	}
	line := r.Line()
	if !strings.Contains(line, `folders="~/My Project,/srv/oss"`) {
		t.Errorf("line %s", line)
	}
	back, _, _, err := ParseRule(g, RuleWords(line))
	if err != nil || !slices.Equal(back.Folders, r.Folders) {
		t.Errorf("read back %q: %v", back.Folders, err)
	}
	if r, _, _, err := ParseRule(g, RuleWords(`use=x folder=~/a`)); err != nil || r.Line() != "use=b/x folders=~/a" {
		t.Errorf("%q %v", r.Line(), err)
	}
	for _, bad := range []string{`use=x folders=`, `use=x folders=work`, `pause=x time=09:00-18:00 folders=/a`} {
		if _, _, _, err := ParseRule(g, RuleWords(bad)); err == nil {
			t.Errorf("%s was taken", bad)
		}
	}
}
