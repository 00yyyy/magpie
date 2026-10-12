package gateway

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/provider"
)

// spawnDescription is the description of the spawn_agent tool in a
// Responses request body, "" when it has none.
func spawnDescription(t *testing.T, body []byte) string {
	t.Helper()
	var q struct {
		Tools []struct {
			Type        string `json:"type"`
			Name        string `json:"name"`
			Description string `json:"description"`
			Tools       []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
			} `json:"tools"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(body, &q); err != nil {
		t.Fatalf("%v: %.300s", err, body)
	}
	for _, tl := range q.Tools {
		// a translated request has it flat, multi_agent_v1__spawn_agent
		if tl.Name == "spawn_agent" || strings.HasSuffix(tl.Name, "__spawn_agent") {
			return tl.Description
		}
		for _, in := range tl.Tools {
			if in.Name == "spawn_agent" {
				return in.Description
			}
		}
	}
	return ""
}

// codexSpawnRequest is a real Codex 0.162.0 request, its spawn_agent
// naming the five models Codex names at most, on model.
func codexSpawnRequest(t *testing.T, model string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/workdir/codex-0.162.0.json")
	if err != nil {
		t.Fatal(err)
	}
	return string(withModel(b, model))
}

// Codex's spawn_agent names only the first five models of its catalog
// to the lead (codex-rs MAX_SPAWN_AGENT_MODEL_OVERRIDES), though it takes
// any model of it: with 40 of magpie's in the catalog, the lead saw none
// of them (Gearclickist on Discord: "Codex subagents on any model" on,
// and only a few models could be picked). The tool now names every model
// a subagent of that lead can work on, at CodexPath and at /v1; for a
// lead the ChatGPT backend answers in multi-agent V2, whose subagents'
// tasks only a ChatGPT account opens, only Codex's own.
func TestSpawnAgentNamesEveryModel(t *testing.T) {
	var models []string
	for i := 1; i <= 40; i++ {
		models = append(models, fmt.Sprintf("m%d", i))
	}
	f := &fake{t: t, reply: sse(`data: {"type":"response.created","response":{"id":"r1"}}`,
		`data: {"type":"response.completed","response":{"id":"r1","usage":{"input_tokens":9,"output_tokens":2}}}`)}
	up := setup(t, provider.Responses, f)
	if err := provider.Save(provider.Provider{ID: "fake", Name: "Fake", Key: "k", Responses: up.URL + "/v1", Models: models}); err != nil {
		t.Fatal(err)
	}
	var upstream []byte
	chatgpt(t, func(w http.ResponseWriter, r *http.Request) {
		upstream, _ = io.ReadAll(r.Body)
		w.Header()["Content-Type"] = nil
		io.WriteString(w, sse(`data: {"type":"response.created","response":{"id":"r1"}}`,
			`data: {"type":"response.completed","response":{"id":"r1","usage":{"input_tokens":9,"output_tokens":2}}}`))
	})

	namesAll := func(t *testing.T, desc string) {
		t.Helper()
		for _, m := range models {
			if !strings.Contains(desc, "- `fake/"+m+"`") {
				t.Fatalf("spawn_agent doesn't name fake/%s:\n%s", m, desc)
			}
		}
		// Codex's own five kept, and its text after the list
		head, tail, _ := strings.Cut(desc, spawnMore)
		list, rest, ok := strings.Cut(tail, "\n        Spawn a sub-agent")
		// what magpie adds goes upstream in the prompt, to OpenAI too:
		// it names neither magpie nor Codex and its limit
		added := desc[len(head) : len(desc)-len(rest)]
		for _, w := range []string{"magpie", "Magpie", "Codex", "five"} {
			if strings.Contains(added, w) {
				t.Fatalf("the added text names %q:\n%s", w, added)
			}
		}
		if !ok || !strings.Contains(head, "- `gpt-6.1-sol`") || !strings.Contains(head, "- `gpt-5.6-sol`") ||
			!strings.HasPrefix(rest, " for a well-scoped task.") {
			t.Fatalf("description:\n%s", desc)
		}
		for _, l := range strings.Split(strings.TrimPrefix(list, "\n"), "\n") {
			if !strings.HasPrefix(l, "- `") {
				t.Fatalf("line %q in the list:\n%s", l, desc)
			}
		}
	}

	t.Run("CodexPath, a lead on magpie's model", func(t *testing.T) {
		code, body := codexPost(t, codexSpawnRequest(t, "fake/m1"))
		if code != 200 {
			t.Fatalf("%d %s", code, body)
		}
		namesAll(t, spawnDescription(t, f.got))
	})
	t.Run("/v1, Codex with magpie as its provider", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(codexSpawnRequest(t, "fake/m2")))
		req.Header.Set("User-Agent", "codex_cli_rs/0.162.0")
		New().Handler().ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("%d %s", rec.Code, rec.Body.String())
		}
		namesAll(t, spawnDescription(t, f.got))
	})
	t.Run("a lead on Codex's own model, V2", func(t *testing.T) {
		code, body := codexPost(t, codexSpawnRequest(t, "gpt-6.1-sol"))
		if code != 200 {
			t.Fatalf("%d %s", code, body)
		}
		if d := spawnDescription(t, upstream); d == "" || strings.Contains(d, "fake/") || strings.Contains(d, spawnMore) {
			t.Fatalf("a sealed lead's subagents offered magpie's models:\n%s", d)
		}
	})
	t.Run("a lead on Codex's own model, V1", func(t *testing.T) {
		if err := provider.SetCodexAgentsV1(true); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { provider.SetCodexAgentsV1(false) })
		code, body := codexPost(t, codexSpawnRequest(t, "gpt-6.1-sol"))
		if code != 200 {
			t.Fatalf("%d %s", code, body)
		}
		namesAll(t, spawnDescription(t, upstream))
	})
}
