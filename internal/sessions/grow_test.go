package sessions

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// grower reads one session file as refresh does, each parse on from the
// one kept before, and checks every such parse against a whole read of the
// same file: a summary read on from where it was is the one a whole read
// makes. start is where the last parse began reading the file.
type grower struct {
	t     *testing.T
	files func() []file
	path  string
	old   *state
	start int64
}

func (g *grower) file() file {
	g.t.Helper()
	for _, f := range g.files() {
		if f.path == g.path {
			return f
		}
	}
	g.t.Fatalf("no file %s", g.path)
	return file{}
}

// read parses the file on from the last parse and says where it started.
func (g *grower) read() int64 {
	g.t.Helper()
	f := g.file()
	g.start = -1
	scanHook = func(p string, off int64) {
		if p == g.path && g.start < 0 {
			g.start = off
		}
	}
	defer func() { scanHook = nil }()
	s := parse(f, g.old)
	scanHook = nil
	whole := parse(f, nil)
	got, _ := json.Marshal(s)
	want, _ := json.Marshal(whole)
	if !bytes.Equal(got, want) {
		g.t.Fatalf("read on from %d:\n%s\nread whole:\n%s", g.start, got, want)
	}
	g.old = s
	return g.start
}

func (g *grower) size() int64 {
	g.t.Helper()
	fi, err := os.Stat(g.path)
	if err != nil {
		g.t.Fatal(err)
	}
	return fi.Size()
}

func (g *grower) append(s string) {
	g.t.Helper()
	f, err := os.OpenFile(g.path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		g.t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(s); err != nil {
		g.t.Fatal(err)
	}
}

// grows runs the cases every reader of an appended file meets: a line
// appended is the only thing read; half a line waits for the rest and is
// counted once; a file cut short, or replaced by one as long, is read whole
// again. line is a whole line the file may take, with its newline.
func checkGrows(t *testing.T, g *grower, line string) {
	t.Helper()
	orig, err := os.ReadFile(g.path)
	if err != nil {
		t.Fatal(err)
	}
	if at := g.read(); at != 0 {
		t.Fatalf("first read started at %d", at)
	}
	// a line appended: only it is read
	was := g.size()
	g.append(line)
	if at := g.read(); at != was {
		t.Fatalf("after a line appended to %d bytes, read from %d", was, at)
	}
	// half a line, still being written: it waits for the rest
	was = g.size()
	half := len(line) / 2
	g.append(line[:half])
	if at := g.read(); at != was {
		t.Fatalf("with half a line at %d, read from %d", was, at)
	}
	g.append(line[half:])
	if at := g.read(); at != was {
		t.Fatalf("the half line finished at %d, read from %d", was, at)
	}
	// cut short: read whole again
	if err := os.WriteFile(g.path, orig, 0o644); err != nil {
		t.Fatal(err)
	}
	if at := g.read(); at != 0 {
		t.Fatalf("cut short, read from %d", at)
	}
	// replaced by another as long, its first line changed: read whole
	g.append(line)
	g.read()
	b, _ := os.ReadFile(g.path)
	other := append([]byte(nil), b...)
	other[len(orig)-2] ^= 1 // a line it had, changed in place
	if bytes.Equal(other, b) {
		t.Fatal("not changed")
	}
	if err := os.WriteFile(g.path, other, 0o644); err != nil {
		t.Fatal(err)
	}
	if at := g.read(); at != 0 {
		t.Fatalf("rewritten in place, read from %d", at)
	}
}

// Grok Build appends to a session's updates.jsonl: a turn that ended is
// read alone, not the session again.
func TestGrokReadsOnlyWhatWasAppended(t *testing.T) {
	inZone(t, 0)
	home := setupAgents(t)
	dir := filepath.Join(home, ".grok", "sessions", "%2Fwork%2Fgrok", grokMain)
	g := &grower{t: t, files: grokFiles, path: filepath.Join(dir, "updates.jsonl")}
	turn := `{"timestamp":1790496140,"method":"_x.ai/session/update","params":{"sessionId":"` + grokMain + `","update":{"sessionUpdate":"turn_completed","usage":{"inputTokens":200,"outputTokens":6,"cachedReadTokens":100,"cacheCreationTokens":0,"modelUsage":{"grok-4.7-build":{"inputTokens":200,"outputTokens":6,"cachedReadTokens":100,"cacheCreationTokens":0}}}}}}` + "\n"
	checkGrows(t, g, turn)

	// its summary rewritten, a title given: nothing of the updates read
	g.read()
	summary := filepath.Join(dir, "summary.json")
	b, _ := os.ReadFile(summary)
	os.WriteFile(summary, bytes.Replace(b, []byte(`"generated_title": "Parser port"`), []byte(`"generated_title": "Port of the parser"`), 1), 0o644)
	if at, end := g.read(), g.size(); at != end || g.old.Named != "Port of the parser" {
		t.Fatalf("summary changed: read from %d of %d, named %q", at, end, g.old.Named)
	}
	// the model its turns are counted under changed: read whole
	b, _ = os.ReadFile(summary)
	os.WriteFile(summary, bytes.Replace(b, []byte(`"current_model_id": "grok-4.7"`), []byte(`"current_model_id": "grok-5"`), 1), 0o644)
	if at := g.read(); at != 0 {
		t.Fatalf("model changed: read from %d", at)
	}
}

// A first prompt still coming in chunks when a read ends goes on in the
// next one.
func TestGrokPromptAcrossReads(t *testing.T) {
	inZone(t, 0)
	home := setupAgents(t)
	dir := filepath.Join(home, ".grok", "sessions", "%2Fwork%2Fgrok", "0190aaaa-0000-7000-8000-0000000000ee")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "summary.json"), []byte(`{"info":{"id":"0190aaaa-0000-7000-8000-0000000000ee","cwd":"/w"},"created_at":"2026-09-27T08:00:00Z"}`), 0o644)
	chunk := func(ts int, text string) string {
		return fmt.Sprintf(`{"timestamp":%d,"params":{"update":{"sessionUpdate":"user_message_chunk","content":{"type":"text","text":%q}}}}`+"\n", ts, text)
	}
	p := filepath.Join(dir, "updates.jsonl")
	os.WriteFile(p, []byte(chunk(1790496001, "Port the ")), 0o644)
	g := &grower{t: t, files: grokFiles, path: p}
	g.read()
	g.append(chunk(1790496002, "parser"))
	g.read()
	g.append(`{"timestamp":1790496003,"params":{"update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"ok"}}}}` + "\n")
	g.read()
	g.append(chunk(1790496004, "and more"))
	if g.read(); g.old.Title != "Port the parser" {
		t.Fatalf("title %q", g.old.Title)
	}
}
