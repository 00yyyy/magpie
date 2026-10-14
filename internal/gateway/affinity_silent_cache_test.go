package gateway

import (
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/yetone/magpie/internal/provider"
)

// silentCache is a vendor that keeps the prompt cached by itself and reports
// nothing of it, as OpenAI's automatic prefix cache and Gemini do: a turn
// that read nothing tells neither a read nor a write, and one that read some
// of it tells the read (OpenAI's prompt_tokens_details.cached_tokens). usage
// false is a vendor that reports nothing of a turn's tokens at all.
type silentCache struct {
	mu    sync.Mutex
	tried []string
	usage bool
}

func (c *silentCache) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	io.ReadAll(r.Body)
	key := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	c.mu.Lock()
	c.tried = append(c.tried, key)
	turn := len(c.tried)
	c.mu.Unlock()
	read := 0
	if turn > 1 {
		read = 2500 // of the cache the first turn built
	}
	told := ""
	if c.usage {
		told = `,"usage":{"prompt_tokens":3000,"completion_tokens":5,"prompt_tokens_details":{"cached_tokens":` + itoa(read) + `}}`
	}
	w.Header().Set("Content-Type", "application/json")
	io.WriteString(w, `{"id":"x","choices":[{"index":0,"message":{"role":"assistant","content":"from `+key+`"},"finish_reason":"stop"}]`+told+`}`)
}

// A vendor that caches without reporting a write (OpenAI's automatic prefix
// cache, Gemini) tells neither a read nor a write of the turn that built the
// cache, so the conversation's second turn moved as "no-cache": the whole
// prompt went afresh to another key and was paid for again. What such a
// vendor holds of the conversation after a turn is the prompt that turn was
// sent, which is what counts as written, so the second turn stays to read it.
// A vendor that reports no usage at all still moves: nothing there says a
// cache was built.
func TestAffinityKeepsACacheTheVendorKeptSilently(t *testing.T) {
	const (
		first = `{"model":"silent/m","messages":[{"role":"user","content":"hi"}]}`
		again = `{"model":"silent/m","messages":[{"role":"user","content":"hi"},{"role":"assistant","content":"yo"},{"role":"user","content":"more"}]}`
	)
	for _, c := range []struct {
		name  string
		usage bool
		why   string
	}{
		{"silent cache", true, "cache"},
		{"no usage at all", false, "no-cache"},
	} {
		fresh(t)
		v := &silentCache{usage: c.usage}
		serveOn(t, "silent", "k1", []string{"m"}, v, "k2")
		if err := provider.SetRouting("silent", provider.LeastUsed); err != nil {
			t.Fatal(err)
		}
		s := New()
		if code, body := postAs(t, s, "s1", first); code != 200 {
			t.Fatalf("%s: first turn %d %s", c.name, code, body)
		}
		if code, body := postAs(t, s, "s1", again); code != 200 {
			t.Fatalf("%s: second turn %d %s", c.name, code, body)
		}
		a := s.trace.routes[len(s.trace.routes)-1].Affinity
		if a == nil || a.Why != c.why || a.Kept != (c.why == "cache") {
			t.Fatalf("%s: affinity %+v, want %s", c.name, a, c.why)
		}
		if c.why == "cache" {
			if v.tried[1] != "k1" {
				t.Fatalf("%s: second turn went to %s, want k1 kept", c.name, v.tried[1])
			}
			if a.CacheRead != 0 || a.CacheWrite != 3000 {
				t.Fatalf("%s: trace tells read %d wrote %d, want the read 0 and this turn's prompt (3000) written", c.name, a.CacheRead, a.CacheWrite)
			}
		} else if v.tried[1] != "k2" {
			t.Fatalf("%s: second turn stayed on %s, want it moved to k2", c.name, v.tried[1])
		}
	}
}
