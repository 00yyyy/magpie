package sessions

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// testdata/claude-forks is Claude Code 2.1.312's own files, from a sandbox
// run with a fake API, trimmed of its request records and attachments'
// text: original.jsonl is a session of two prompts, fork-on.jsonl is
// `claude --resume <it> --fork-session` with a third prompt, and
// fork-other.jsonl another such fork with a different third prompt. Both
// forks start with every message of the original again, under the same
// uuids and their own sessionId.
const (
	ccOriginal  = "fb0bf5a9-e23f-4c29-b71c-a184c38bb005"
	ccForkOn    = "7e99b4bb-7d93-4b3c-9453-901a1a9298d1"
	ccForkOther = "c2141c85-7cf5-46c0-a389-d584029233fb"
)

// ccForkFixture writes the fixtures named (by file, to the id it is saved
// as) into dir/projects/-w-proj, oldest first, each a minute after the last
// and all of them over a minute old, so none is still being written.
// A copy saved under another id is the same file with its sessionId
// rewritten, as /branch or a background move leaves one before a turn.
func ccForkFixture(t *testing.T, dir string, files ...[2]string) {
	t.Helper()
	proj := filepath.Join(dir, "projects", "-w-proj")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	at := time.Now().Add(-time.Hour)
	for _, f := range files {
		b, err := os.ReadFile(filepath.Join("testdata", "claude-forks", f[0]))
		if err != nil {
			t.Fatal(err)
		}
		var from string
		switch f[0] {
		case "original.jsonl":
			from = ccOriginal
		case "fork-on.jsonl":
			from = ccForkOn
		default:
			from = ccForkOther
		}
		b = []byte(strings.ReplaceAll(string(b), from, f[1]))
		p := filepath.Join(proj, f[1]+".jsonl")
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
		at = at.Add(time.Minute)
		if err := os.Chtimes(p, at, at); err != nil {
			t.Fatal(err)
		}
	}
	Reset()
}

// ccRows is the ids of agent's sessions in the fixture's project, sorted.
func ccRows(agent string) []string {
	var ids []string
	for _, m := range ListAgent(agent) {
		if m.Cwd == "/w/proj" {
			ids = append(ids, m.ID)
		}
	}
	slices.Sort(ids)
	return ids
}

func ccPrompts(t *testing.T, agent, id string) []string {
	t.Helper()
	s, ok := Find(agent, id)
	if !ok {
		t.Fatalf("Find(%s, %s) found nothing", agent, id)
	}
	tr, err := TranscriptOf(s)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, p := range tr.Parts {
		if p.Role == "user" && p.Kind == "text" {
			out = append(out, p.Text)
		}
	}
	return out
}

var ccCopyIDs = []string{
	"11111111-1111-4111-8111-111111111111",
	"22222222-2222-4222-8222-222222222222",
	"33333333-3333-4333-8333-333333333333",
}

// Neo (#feedback): one Claude conversation listed as ten rows of the same
// title, size and message count, one per copy Claude Code had made of it.
// The copies and the session they were forked from are one row, the
// session that holds all of them, and it reads whole.
func TestClaudeForkCopiesAreOneRow(t *testing.T) {
	claude, _ := setup(t)
	before := agentCount("claude")
	files := [][2]string{{"original.jsonl", ccOriginal}}
	for _, id := range ccCopyIDs {
		files = append(files, [2]string{"original.jsonl", id})
	}
	files = append(files, [2]string{"fork-on.jsonl", ccForkOn})
	ccForkFixture(t, claude, files...)

	if got := ccRows("claude"); !slices.Equal(got, []string{ccForkOn}) {
		t.Fatalf("rows = %v, want only the fork that holds them all, %s", got, ccForkOn)
	}
	var listed []string
	for _, s := range List(0) {
		if s.Agent == "claude" && s.Cwd == "/w/proj" {
			listed = append(listed, s.ID)
		}
	}
	if !slices.Equal(listed, []string{ccForkOn}) {
		t.Errorf("List rows = %v, want [%s]", listed, ccForkOn)
	}
	if got := agentCount("claude"); got != before+1 {
		t.Errorf("Claude Code's count = %d, want %d: one conversation", got, before+1)
	}
	want := []string{"first prompt", "second prompt resumed", "third prompt forked"}
	if got := ccPrompts(t, "claude", ccForkOn); !slices.Equal(got, want) {
		t.Errorf("prompts = %q, want %q", got, want)
	}

	// what the conversation spent is counted once: three replies of 10 in,
	// 5 out, not again for each copy of the first two
	var tk Tokens
	for _, s := range StatsFor(0).Sessions {
		if s.Agent == "claude" && s.Cwd == "/w/proj" {
			tk.Input += s.Input
			tk.Output += s.Output
		}
	}
	if tk.Input != 30 || tk.Output != 15 {
		t.Errorf("tokens = %d in, %d out; want 30 in, 15 out", tk.Input, tk.Output)
	}
}

// A fork that went on with a turn of its own is a session of its own, as
// Claude Code's /resume lists it; the session both were forked from is in
// each of them.
func TestClaudeForkThatWentOnIsItsOwnRow(t *testing.T) {
	claude, _ := setup(t)
	ccForkFixture(t, claude,
		[2]string{"original.jsonl", ccOriginal},
		[2]string{"fork-on.jsonl", ccForkOn},
		[2]string{"fork-other.jsonl", ccForkOther})
	want := []string{ccForkOther, ccForkOn}
	slices.Sort(want)
	if got := ccRows("claude"); !slices.Equal(got, want) {
		t.Fatalf("rows = %v, want the two forks %v", got, want)
	}
	if got := ccPrompts(t, "claude", ccForkOther); !slices.Equal(got, []string{"first prompt", "second prompt resumed", "a different turn"}) {
		t.Errorf("the other fork's prompts = %q", got)
	}
}

// Deleting the conversation takes its copies along, or the next list
// would show the newest of them in its place; restoring brings them back.
func TestClaudeDeleteTakesCopiesAlong(t *testing.T) {
	claude, _ := setup(t)
	ccForkFixture(t, claude,
		[2]string{"original.jsonl", ccOriginal},
		[2]string{"original.jsonl", ccCopyIDs[0]},
		[2]string{"fork-on.jsonl", ccForkOn})
	if got := ccRows("claude"); !slices.Equal(got, []string{ccForkOn}) {
		t.Fatalf("rows = %v", got)
	}
	tr, err := Delete("claude", ccForkOn)
	if err != nil {
		t.Fatal(err)
	}
	if got := ccRows("claude"); len(got) != 0 {
		t.Errorf("after delete, rows = %v: a copy came back", got)
	}
	proj := filepath.Join(claude, "projects", "-w-proj")
	for _, id := range []string{ccOriginal, ccCopyIDs[0], ccForkOn} {
		if _, err := os.Stat(filepath.Join(proj, id+".jsonl")); !os.IsNotExist(err) {
			t.Errorf("%s left behind: %v", id, err)
		}
	}
	if _, err := Restore(tr.Key); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{ccOriginal, ccCopyIDs[0], ccForkOn} {
		if _, err := os.Stat(filepath.Join(proj, id+".jsonl")); err != nil {
			t.Errorf("%s not restored: %v", id, err)
		}
	}
	if got := ccRows("claude"); !slices.Equal(got, []string{ccForkOn}) {
		t.Errorf("after restore, rows = %v", got)
	}
}

// Qoder writes Claude Code's files, and its copies are one row too.
func TestQoderForkCopiesAreOneRow(t *testing.T) {
	for _, agent := range []string{"qoder", "qoder-cn"} {
		t.Run(agent, func(t *testing.T) {
			setup(t)
			ccForkFixture(t, QoderDir(agent),
				[2]string{"original.jsonl", ccOriginal},
				[2]string{"original.jsonl", ccCopyIDs[0]},
				[2]string{"fork-on.jsonl", ccForkOn})
			if got := ccRows(agent); !slices.Equal(got, []string{ccForkOn}) {
				t.Fatalf("rows = %v, want [%s]", got, ccForkOn)
			}
		})
	}
}

func agentCount(agent string) int {
	for _, a := range Agents() {
		if a.Agent == agent {
			return a.Count
		}
	}
	return 0
}

// The gateway may know a copy's id from its requests: that is the listed
// session's, not a gateway-only session of its own.
func TestMergeExternalKnowsCopies(t *testing.T) {
	native := []Session{{Agent: "claude", ID: ccForkOn, Copies: []string{ccOriginal}}}
	out := MergeExternal(native, []ExternalSession{{Agent: "claude", ID: ccOriginal, Title: "x"}})
	if len(out) != 1 {
		t.Errorf("rows = %d, want 1: the copy came back from the gateway", len(out))
	}
}
