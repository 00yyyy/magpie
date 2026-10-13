package gateway

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/provider"
)

// tierAccounts signs Codex in to a Plus ChatGPT account, first, with a Pro
// one saved beside it, against a fake ChatGPT backend whose model lists
// offer gpt-6-luna's Ultrafast only to the Pro account. It returns the
// account and service tier each request was tried with, in order.
func tierAccounts(t *testing.T, proOut *bool) func() []string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	restingUntil.Lock()
	restingUntil.m = map[string]time.Time{}
	restingUntil.Unlock()
	claims := func(m map[string]any) string {
		b, _ := json.Marshal(m)
		return "h." + base64.RawURLEncoding.EncodeToString(b) + ".s"
	}
	auth := func(email, acct, plan string) map[string]any {
		return map[string]any{"auth_mode": "chatgpt", "tokens": map[string]any{
			"id_token": claims(map[string]any{"email": email,
				"https://api.openai.com/auth": map[string]any{"chatgpt_plan_type": plan}}),
			"access_token":  claims(map[string]any{"exp": time.Now().Add(time.Hour).Unix(), "who": acct}),
			"refresh_token": "r-" + acct, "account_id": acct}}
	}
	os.MkdirAll(filepath.Join(home, ".codex"), 0o755)
	os.WriteFile(filepath.Join(home, ".codex", "auth.json"), mustJSON(auth("plus@example.com", "acct-plus", "plus")), 0o600)
	os.MkdirAll(filepath.Dir(provider.Path()), 0o755)
	os.WriteFile(filepath.Join(filepath.Dir(provider.Path()), "logins.json"), mustJSON([]map[string]any{
		{"agent": "codex", "user": "pro@example.com", "plan": "pro", "on": true, "seen": time.Now(), "auth": auth("pro@example.com", "acct-pro", "pro")},
	}), 0o600)

	var mu sync.Mutex
	var tried []string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		acct := r.Header.Get("chatgpt-account-id")
		if strings.HasSuffix(r.URL.Path, "/models") {
			tiers := `[{"id":"priority","name":"Fast","description":""}]`
			if acct == "acct-pro" {
				tiers = `[{"id":"priority","name":"Fast","description":""},{"id":"ultrafast","name":"Ultrafast","description":""}]`
			}
			io.WriteString(w, `{"models":[{"slug":"gpt-6-luna","visibility":"list","service_tiers":`+tiers+`}]}`)
			return
		}
		var q struct {
			Tier string `json:"service_tier"`
		}
		json.Unmarshal(b, &q)
		mu.Lock()
		tried = append(tried, acct+":"+q.Tier)
		mu.Unlock()
		if acct == "acct-pro" && proOut != nil && *proOut {
			w.WriteHeader(429)
			io.WriteString(w, `{"error":{"message":"You've hit your usage limit"}}`)
			return
		}
		io.WriteString(w, sse(
			`data: {"type":"response.created","response":{"id":"r1","model":"gpt-6-luna"}}`,
			`data: {"type":"response.output_text.delta","delta":"pong"}`,
			`data: {"type":"response.completed","response":{"id":"r1","usage":{"input_tokens":7,"output_tokens":1}}}`))
	}))
	t.Cleanup(up.Close)
	old := provider.CodexBase
	provider.CodexBase = up.URL + "/backend-api/codex"
	t.Cleanup(func() { provider.CodexBase = old })

	p, err := provider.Find("codex")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Fetch(context.Background()); err != nil {
		t.Fatal(err)
	}
	return func() []string {
		mu.Lock()
		defer mu.Unlock()
		out := tried
		tried = nil
		return out
	}
}

// yxinyu715 on X: Codex's Ultrafast goes first to the account whose plan
// offers it, here the Pro one saved second, and Fast keeps the accounts'
// order. When the Pro one can't answer, the Plus one is asked for Fast,
// the nearest its plan offers.
func TestUltrafastGoesToAnAccountOfferingIt(t *testing.T) {
	proOut := false
	tried := tierAccounts(t, &proOut)
	s := New()
	codex := func(tier string) string {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", CodexPath+"/responses", strings.NewReader(`{"model":"gpt-6-luna","stream":true,"input":"ping","service_tier":"`+tier+`"}`))
		req.Header.Set("Authorization", "Bearer chatgpt-token")
		req.Header.Set("chatgpt-account-id", "acct-plus")
		req.Header.Set("User-Agent", "codex_cli_rs/0.160.1")
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), "pong") {
			t.Fatalf("%s: status %d: %s", tier, rec.Code, rec.Body.String())
		}
		return strings.Join(tried(), ",")
	}
	if got := codex("ultrafast"); got != "acct-pro:ultrafast" {
		t.Errorf("ultrafast tried %s", got)
	}
	if got := codex("priority"); got != "acct-plus:priority" {
		t.Errorf("priority tried %s", got)
	}
	proOut = true
	if got := codex("ultrafast"); got != "acct-pro:ultrafast,acct-plus:priority" {
		t.Errorf("pro out: ultrafast tried %s", got)
	}

	// an agent asking through magpie's own endpoint is ordered the same
	proOut = false
	if code, body := post(t, "/v1/responses", `{"model":"codex/gpt-6-luna","input":"ping","service_tier":"ultrafast"}`); code != 200 || !strings.HasPrefix(strings.Join(tried(), ","), "acct-pro:") {
		t.Errorf("/v1/responses: status %d: %s", code, body)
	}
}

// #1784, zhenghaowen0967: OMP, with gpt-6.1-sol switched fast in its
// picker (fastPicks.omp), asked for Ultrafast and went as priority, the
// fast mode's rewrite writing over the client's own, faster ask. It goes
// as Ultrafast to the account that offers it, and as priority to one that
// doesn't; a request that asked for no tier is sent fast as before.
func TestFastPickKeepsTheClientsUltrafast(t *testing.T) {
	proOut := false
	tried := tierAccounts(t, &proOut)
	if err := provider.SetFastPick("omp", "codex/gpt-6-luna", true); err != nil {
		t.Fatal(err)
	}
	// at chatgpt.com, where Ultrafast is sent (buildResponses), reaching
	// the fake backend
	to, _ := url.Parse(provider.CodexBase)
	provider.CodexBase = "https://chatgpt.com/backend-api/codex"
	s := New()
	s.client = &http.Client{Transport: qoderRoundTrip(func(r *http.Request) (*http.Response, error) {
		r.URL.Scheme, r.URL.Host = to.Scheme, to.Host
		return http.DefaultTransport.RoundTrip(r)
	})}
	omp := func(tier string) string {
		t.Helper()
		r := postFrom(t, s, "omp", "/v1/responses", `{"model":"codex/gpt-6-luna","input":"ping"`+tier+`}`)
		if len(r.Tries) == 0 || !r.Tries[0].Fast {
			t.Errorf("%s: not traced as sent fast: %+v", tier, r.Tries)
		}
		return strings.Join(tried(), ",")
	}
	if got := omp(`,"service_tier":"ultrafast"`); got != "acct-pro:ultrafast" {
		t.Errorf("ultrafast tried %s", got)
	}
	proOut = true
	if got := omp(`,"service_tier":"ultrafast"`); got != "acct-pro:ultrafast,acct-plus:priority" {
		t.Errorf("pro out: ultrafast tried %s", got)
	}
	proOut = false
	if got := omp(""); got != "acct-plus:priority" {
		t.Errorf("no tier asked: tried %s", got)
	}
	if b := withFast(provider.Chat, []byte(`{"model":"m","service_tier":"ultrafast"}`)); askedTier(b) != "ultrafast" {
		t.Errorf("chat: %s", b)
	}
}
