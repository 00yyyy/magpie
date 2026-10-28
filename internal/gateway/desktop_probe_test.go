package gateway

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tidwall/gjson"
	"github.com/yetone/magpie/internal/provider"
)

// relayNeedsCacheKey answers as CyberCaius's AnyRouter did a request
// without a prompt_cache_key.
func relayNeedsCacheKey(body []byte) (int, string) {
	if gjson.GetBytes(body, "prompt_cache_key").String() == "" {
		return 400, `{"error":{"message":"Arouter: prompt_cache_key is required (request id: x)","type":"invalid_request_error"},"type":"error"}`
	}
	return 0, ""
}

// Claude Desktop checks a gateway model as it starts with
// {"max_tokens":1,"messages":[{"role":"user","content":"."}]} on the first
// haiku it is listed. With a session on a relay's GPT model, magpie took
// that check for a title and sent it to the relay, which turned it away,
// and Desktop said the haiku model was rejected (CyberCaius on Discord).
// The check goes to the model it names; a title still goes to the
// session's model, and reaches the relay with a cache key it needs.
func TestClaudeDesktopProbeAsked(t *testing.T) {
	claude := &fake{t: t, reply: sse(
		`event: message_start`+"\n"+`data: {"type":"message_start","message":{"id":"m","type":"message","role":"assistant","model":"claude-haiku-5-5","content":[],"usage":{"input_tokens":1,"output_tokens":0}}}`,
		`event: message_stop`+"\n"+`data: {"type":"message_stop"}`)}
	relay := &fake{t: t, reply: deepseekReply, refuse: relayNeedsCacheKey}
	up := setup(t, provider.Anthropic, claude)
	_ = up
	ru := httptest.NewServer(relay)
	t.Cleanup(ru.Close)
	if err := provider.Save(provider.Provider{ID: "leee", Name: "leee", Key: "k", Anthropic: up.URL,
		Models: []string{"claude-haiku-5-5"}}); err != nil {
		t.Fatal(err)
	}
	if err := provider.Save(provider.Provider{ID: "anyrouter", Name: "AnyRouter", Key: "k", Chat: ru.URL + "/v1",
		Models: []string{"gpt-6.1-sol"}}); err != nil {
		t.Fatal(err)
	}
	desktopPicked.Lock()
	desktopPicked.model, desktopPicked.from = "", ""
	desktopPicked.Unlock()
	srv := New()
	send := func(body string) (int, string) {
		t.Helper()
		req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(body))
		req.Header.Set("x-api-key", Token+"-claude-desktop")
		req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh) Claude/2.7032.0 Chrome/140.0 Electron/38.0")
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		return rec.Code, rec.Body.String()
	}
	// the session's turn, on the relay's model
	if code, body := send(`{"model":"anyrouter/gpt-6.1-sol","max_tokens":32000,"stream":true,"tools":[{"name":"Bash","input_schema":{"type":"object"}}],"messages":[{"role":"user","content":"hi"}]}`); code != 200 {
		t.Fatalf("turn: %d %s", code, body)
	}
	first := gjson.GetBytes(relay.got, "prompt_cache_key").String()
	if first == "" || strings.Contains(strings.ToLower(first), "magpie") {
		t.Fatalf("turn's cache key %q", first)
	}

	// Desktop's check of its haiku, by the id it was listed
	haiku := "magpie-" + aliasNumber("leee/claude-haiku-5-5") + ".anthropic.claude-haiku-5-5[1m]"
	relay.calls, claude.got = 0, nil
	if code, body := send(`{"model":"` + haiku + `","max_tokens":1,"messages":[{"role":"user","content":"."}]}`); code != 200 {
		t.Fatalf("probe: %d %s", code, body)
	}
	if relay.calls != 0 || !bytes.Contains(claude.got, []byte(`"claude-haiku-5-5"`)) {
		t.Fatalf("probe went to the relay (%d calls); leee got %s", relay.calls, claude.got)
	}

	// a title still goes to the session's model, and the relay takes it
	relay.got = nil
	if code, body := send(`{"model":"` + haiku + `","max_tokens":200,"stream":true,"system":"You write short session titles.","messages":[{"role":"user","content":"hi"}]}`); code != 200 {
		t.Fatalf("title: %d %s", code, body)
	}
	if modelOf(relay.got) != "gpt-6.1-sol" {
		t.Fatalf("title went to %q", modelOf(relay.got))
	}
	// the same conversation keeps its key
	if code, body := send(`{"model":"anyrouter/gpt-6.1-sol","max_tokens":32000,"stream":true,"tools":[{"name":"Bash","input_schema":{"type":"object"}}],"messages":[{"role":"user","content":"hi"},{"role":"assistant","content":"ok"},{"role":"user","content":"more"}]}`); code != 200 {
		t.Fatalf("second turn: %d %s", code, body)
	}
	if k := gjson.GetBytes(relay.got, "prompt_cache_key").String(); k != first {
		t.Fatalf("second turn's key %q, first %q", k, first)
	}
}

// desktopProbe knows Desktop's check alone, not a title or a turn.
func TestDesktopProbe(t *testing.T) {
	for body, want := range map[string]bool{
		`{"model":"x","max_tokens":1,"messages":[{"role":"user","content":"."}]}`:                                            true,
		`{"model":"x","max_tokens":200,"messages":[{"role":"user","content":"."}]}`:                                          false,
		`{"model":"x","max_tokens":1,"system":"You write short session titles.","messages":[{"role":"user","content":"."}]}`: false,
		`{"model":"x","max_tokens":1,"messages":[{"role":"user","content":"hi"}]}`:                                           false,
		`{"model":"x","max_tokens":1,"tools":[{"name":"a"}],"messages":[{"role":"user","content":"."}]}`:                     false,
	} {
		if desktopProbe([]byte(body)) != want {
			t.Errorf("desktopProbe(%s) = %v", body, !want)
		}
	}
}

// magpie's own cache key goes only to OpenAI's API and a provider the user
// added by its address; a vendor's preset, a signed-in account and a
// plugin get none. A relay that turns the field away is asked again
// without it, once.
func TestOwnCacheKey(t *testing.T) {
	for _, c := range []struct {
		p    provider.Provider
		want bool
	}{
		{provider.Provider{ID: "relay", Chat: "https://relay.example/v1"}, true},
		{provider.Provider{ID: "openai", Preset: "openai", Responses: "https://api.openai.com/v1"}, true},
		{provider.Provider{ID: "deepseek", Preset: "deepseek", Chat: "https://api.deepseek.com/v1"}, false},
		{provider.Provider{ID: "codex", Account: &provider.Account{}, Responses: "https://chatgpt.com/backend-api/codex"}, false},
		{provider.Provider{ID: "relay-own", Chat: "https://relay.example/v1", ExtraBody: provider.ExtraBody{"prompt_cache_key": json.RawMessage(`"${sessionId}"`)}}, false},
		{provider.Provider{ID: "relay-forced", Chat: "https://relay.example/v1", ExtraBody: provider.ExtraBody{"!prompt_cache_key": json.RawMessage(`"k"`)}}, false},
	} {
		if got := ownCacheKey(c.p); got != c.want {
			t.Errorf("ownCacheKey(%s) = %v", c.p.ID, got)
		}
	}

	refusing := func(body []byte) (int, string) {
		if gjson.GetBytes(body, "prompt_cache_key").Exists() {
			return 400, `{"error":{"message":"Unrecognized request argument supplied: prompt_cache_key"}}`
		}
		return 0, ""
	}
	f := &fake{t: t, reply: deepseekReply, refuse: refusing}
	setup(t, provider.Chat, f)
	srv := New()
	for turn := range 2 {
		calls := f.calls
		req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(`{"model":"fake/m1","max_tokens":100,"stream":true,"messages":[{"role":"user","content":"hi"}]}`))
		req.Header.Set("x-api-key", Token)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("turn %d: %d %s", turn+1, rec.Code, rec.Body)
		}
		if n, want := f.calls-calls, 2-turn; n != want {
			t.Errorf("turn %d: %d upstream calls, want %d", turn+1, n, want)
		}
	}
}
