package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/settings"
)

// When the searcher picked in Settings fails or finds nothing and the
// next one searches in its place, the row of the search that answered
// names the picked one and why (Gearclickist on Discord: Grok was picked,
// Codex's model searched, and nothing said Grok had failed). Without it
// the fallback's row reads as an ordinary search by the other model.
func TestSearchInPlaceOfPickedSaysWhy(t *testing.T) {
	for _, tc := range []struct {
		name    string
		failing bool
	}{{"found nothing", false}, {"failed", true}} {
		t.Run(tc.name, func(t *testing.T) {
			searchSandbox(t)
			ds := newFakeDeepSeek(t)
			ds.blind = true
			picked := ds.URL
			if tc.failing {
				broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if strings.HasSuffix(r.URL.Path, "/models") {
						w.Write([]byte(`{"data":[{"id":"deepseek-flash"}]}`))
						return
					}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadRequest)
					w.Write([]byte(`{"error":{"message":"search quota used up"}}`))
				}))
				t.Cleanup(broken.Close)
				picked = broken.URL
			}
			glm, _ := newBlindModel(t)
			next := newFakeDeepSeek(t)
			searchesOn(t, provider.Responses, picked)
			searchesOn(t, provider.Anthropic, next.URL)
			save(t, provider.Provider{ID: "ds", Name: "DeepSeek", Key: "k", Responses: picked + "/v1", Models: []string{"deepseek-flash"}},
				provider.Provider{ID: "glm", Name: "GLM", Key: "k", Chat: glm.URL, Models: []string{"glm-flash"}},
				provider.Provider{ID: "ds2", Name: "DeepSeek 2", Key: "k", Anthropic: next.URL + "/anthropic", Models: []string{"deepseek-flash"}})
			st := settings.Load()
			st.Searcher = "ds/deepseek-flash"
			if err := settings.Save(st); err != nil {
				t.Fatal(err)
			}

			s := New()
			body := `{"model":"glm/glm-flash","max_tokens":3000,"stream":true,"messages":[{"role":"user","content":"用 web_search 工具搜索：今天上海天气"}],"tools":[{"type":"web_search_20250305","name":"web_search"}]}`
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body)))
			if rec.Code != 200 || len(next.searches()) != 1 {
				t.Fatalf("%d, the next searcher asked %q: %s", rec.Code, next.searches(), rec.Body.String())
			}
			var first, instead *Route
			for _, r := range s.Trace(context.Background(), 0, 0).Routes {
				if r.Kind != "web_search" {
					continue
				}
				r := r
				if strings.HasPrefix(r.Model, "ds2/") {
					instead = &r
				} else if strings.HasPrefix(r.Model, "ds/") {
					first = &r
				}
			}
			if instead == nil || instead.For == nil {
				t.Fatalf("no row for the search that answered: %+v", s.Trace(context.Background(), 0, 0).Routes)
			}
			f := instead.For
			if f.Instead != "ds/deepseek-flash" || !f.Picked || f.Agent == "" || f.Model != "glm/glm-flash" {
				t.Fatalf("the answering search's row is for %+v, want it to name the picked ds/deepseek-flash", *f)
			}
			if tc.failing && (f.NothingFound || !strings.Contains(f.Why, "search quota used up")) {
				t.Fatalf("why %q nothing %v, want the picked searcher's error", f.Why, f.NothingFound)
			}
			if !tc.failing && (!f.NothingFound || f.Why != "") {
				t.Fatalf("why %q nothing %v, want found nothing", f.Why, f.NothingFound)
			}
			// the picked one's own row is an ordinary search for the model
			if first != nil && first.For != nil && first.For.Instead != "" {
				t.Fatalf("the picked searcher's own row says it was in place of %q", first.For.Instead)
			}
		})
	}
}
