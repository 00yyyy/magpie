package agent

import (
	"testing"

	"github.com/yetone/magpie/internal/gateway"
	"github.com/yetone/magpie/internal/provider"
)

// Claude Desktop's subagent model field: offered magpie's models once
// Desktop is on magpie, a pick connecting it as any model pick does, an
// alias kept by its catalog id, one not served now kept, a bare name
// refused. It alone is Live: Desktop's tiers and Codex's subagent model
// aren't, and a stopped WSL copy keeps the flag.
func TestDesktopSubagentModelField(t *testing.T) {
	home, _ := desktopSandbox(t)
	if err := provider.Save(provider.Provider{ID: "fake", Name: "Fake", Chat: "http://127.0.0.1:1/v1", Key: "k", Models: []string{"m1"}}); err != nil {
		t.Fatal(err)
	}
	a := claudeDesktop(home)
	f := a.Field("subagent_model")
	if f == nil || f.Label != "subagent model" || !f.Quiet || !f.Live {
		t.Fatalf("subagent field: %+v", f)
	}
	if f.Options(nil) != nil {
		t.Fatal("subagent options offered while unwired")
	}
	if err := a.Pick(f.Key, "fake/m1"); err != nil {
		t.Fatal(err)
	}
	if !a.Wired() || len(f.Options(nil)) == 0 {
		t.Fatal("subagent pick did not wire Desktop or offer its models")
	}
	if err := f.Set(gateway.DesktopID(provider.Entry{ID: "fake/m1"})); err != nil {
		t.Fatal(err)
	}
	if got := f.Get(); got != "fake/m1" {
		t.Fatalf("alias pick %q, want fake/m1", got)
	}
	if err := f.Set("gpt-6"); err == nil {
		t.Fatal("accepted a bare model")
	}
	if err := f.Set("missing/m"); err != nil || f.Get() != "missing/m" {
		t.Fatalf("restored model %q: %v", f.Get(), err)
	}
	if err := f.Set(""); err != nil || f.Get() != "" {
		t.Fatalf("clear %q: %v", f.Get(), err)
	}
	for _, tier := range gateway.DesktopTierNames {
		if a.Field(tier).Live {
			t.Fatalf("tier %s is Live", tier)
		}
	}
	if codex(home).Field("subagent_model").Live {
		t.Fatal("Codex's subagent model became Live")
	}
	stopped := asleep(a, wslKind{}, distro{Name: "Stopped"})
	if !stopped.Field("subagent_model").Live {
		t.Fatal("stopped WSL agent lost the Live flag")
	}
}
