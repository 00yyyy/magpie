package sessions

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Desktop keeps its own name outside the CLI transcript. The session header
// is cliSessionId, including older CLI runs listed in priorCliSessionIds.
func TestDesktopSessionTitles(t *testing.T) {
	setup(t)
	data := t.TempDir()
	old := callDesktopDirs
	callDesktopDirs = func() []string { return []string{data} }
	t.Cleanup(func() { callDesktopDirs = old; Reset() })
	const current = "11111111-aaaa-4000-8000-000000000001"
	const prior = "11111111-aaaa-4000-8000-000000000002"
	const cowork = "11111111-aaaa-4000-8000-000000000003"
	path := filepath.Join(data, "claude-code-sessions", "account", "org", "local_desktop.json")
	writeDesktopTitle(t, path, `{"sessionId":"local_desktop","cliSessionId":"`+current+`","priorCliSessionIds":["`+prior+`"],"title":"检查会话标题","titleSource":"user","isArchived":false}`)
	writeDesktopTitle(t, filepath.Join(data, "local-agent-mode-sessions", "account", "org", "local_cowork.json"), `{"sessionId":"local_cowork","cliSessionId":"`+cowork+`","title":"Archived Cowork title","isArchived":true}`)
	keys := []string{"claude-desktop:" + current, "claude-desktop:" + prior, "claude-desktop:" + cowork, "claude:" + current, "codex:" + current, "claude-desktop:missing"}
	want := map[string]string{keys[0]: "检查会话标题", keys[1]: "检查会话标题", keys[2]: "Archived Cowork title"}
	if got := Titles(keys); !reflect.DeepEqual(got, want) {
		t.Fatalf("Desktop names without transcripts: got %v, want %v", got, want)
	}
	merged := MergeExternal(nil, []ExternalSession{{Agent: "claude-desktop", ID: current}, {Agent: "claude-desktop", ID: prior}, {Agent: "claude", ID: current}, {Agent: "claude-desktop", ID: "missing"}})
	for i, name := range []string{"检查会话标题", "检查会话标题", current, "missing"} {
		if s := merged[i]; s.Title != name || !s.Gateway || !s.ReadOnly || s.Path != "" || s.Resume != "" {
			t.Fatalf("gateway row %d: %+v, want title %q and gateway capabilities", i, s, name)
		}
	}
	// Desktop's saved title comes first; an absent title keeps the gateway's
	// recorded first prompt, including for other agents.
	for _, e := range []ExternalSession{
		{Agent: "claude-desktop", ID: current, Title: "Recorded prompt"},
		{Agent: "claude-desktop", ID: "missing", Title: "Recorded prompt"},
		{Agent: "claude", ID: current, Title: "Recorded prompt"},
	} {
		want := "Recorded prompt"
		if e.Agent == "claude-desktop" && e.ID == current {
			want = "检查会话标题"
		}
		if got := MergeExternal(nil, []ExternalSession{e})[0].Title; got != want {
			t.Fatalf("Desktop title / recorded prompt priority for %+v: %q, want %q", e, got, want)
		}
	}
	// A native Cowork transcript uses Desktop's displayed name too, and
	// still wins over a gateway projection of the same identity.
	writeDesktopTitle(t, filepath.Join(data, "local-agent-mode-sessions", "account", "org", "local_cowork", ".claude", "projects", "session", cowork+".jsonl"), `{"type":"ai-title","aiTitle":"CLI title","sessionId":"`+cowork+`"}`+"\n")
	native, ok := Get("claude-desktop:" + cowork)
	if !ok || native.Title != "Archived Cowork title" || native.Path == "" {
		t.Fatalf("native Desktop session: %+v, found %v", native, ok)
	}
	if got := MergeExternal([]Session{native}, []ExternalSession{{Agent: "claude-desktop", ID: cowork}}); len(got) != 1 || got[0].Gateway || got[0].Path != native.Path {
		t.Fatalf("native Desktop session did not win: %+v", got)
	}
	// Renaming needs no new model call, and an old CLI ID follows the rename.
	writeDesktopTitle(t, path, `{"sessionId":"local_desktop","cliSessionId":"`+current+`","priorCliSessionIds":["`+prior+`"],"title":"Renamed Desktop session"}`)
	if got := Titles(keys); got[keys[0]] != "Renamed Desktop session" || got[keys[1]] != "Renamed Desktop session" {
		t.Fatalf("rename did not reach current and prior IDs: %v", got)
	}
	// A partial rewrite must not keep a cached name or poison later reads.
	writeDesktopTitle(t, path, `{"cliSessionId":"`+current+`","title":`)
	if got := Titles(keys); got[keys[0]] != "" || got[keys[1]] != "" {
		t.Fatalf("partial metadata retained stale names: %v", got)
	}
	writeDesktopTitle(t, path, `{"cliSessionId":"`+current+`","title":"Recovered"}`)
	if got := Titles(keys)[keys[0]]; got != "Recovered" {
		t.Fatalf("metadata repair: %q", got)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if got := Titles(keys)[keys[0]]; got != "" {
		t.Fatalf("deleted metadata retained its name: %q", got)
	}
}

func TestDesktopSessionTitleIdentity(t *testing.T) {
	setup(t)
	data, threep := t.TempDir(), t.TempDir()
	old := callDesktopDirs
	callDesktopDirs = func() []string { return []string{data, threep} }
	t.Cleanup(func() { callDesktopDirs = old; Reset() })
	for i, root := range []string{data, threep} {
		name := []string{"Original", "Fork"}[i]
		writeDesktopTitle(t, filepath.Join(root, "claude-code-sessions", "account", "org", "local_session.json"), `{"sessionId":"local_session","cliSessionId":"shared","title":"`+name+`","priorCliSessionIds":["old"],"forkedFromSessionId":"local_parent"}`)
	}
	if got := Titles([]string{"claude-desktop:shared", "claude-desktop:old", "claude-desktop:local_parent"}); len(got) != 0 {
		t.Fatalf("ambiguous or fork-parent IDs were named: %v", got)
	}
	// An explicit current CLI ID takes precedence over another session's old ID.
	writeDesktopTitle(t, filepath.Join(threep, "claude-code-sessions", "account", "org", "local_session.json"), `{"cliSessionId":"old","title":"Current owner"}`)
	if got := Titles([]string{"claude-desktop:old"})["claude-desktop:old"]; got != "Current owner" {
		t.Fatalf("prior ID overrode its current owner: %q", got)
	}
	writeDesktopTitle(t, filepath.Join(threep, "claude-code-sessions", "account", "org", "local_session.json"), `{"cliSessionId":"old","title":""}`)
	if got := Titles([]string{"claude-desktop:old"})["claude-desktop:old"]; got != "" {
		t.Fatalf("an unnamed current owner inherited another session's prior title: %q", got)
	}
}

func writeDesktopTitle(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}
