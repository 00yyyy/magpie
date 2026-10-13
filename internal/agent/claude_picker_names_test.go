package agent

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/yetone/magpie/internal/catalog"
	"github.com/yetone/magpie/internal/edit"
	"github.com/yetone/magpie/internal/provider"
)

// love1sbug on X: switching Names in agents' lists between Off, Not on
// names I set and On changed nothing in Claude Code's /model, as its
// modelPicker labelled every model by its name alone. Each row is now
// labelled as every agent's list names the model (provider.Labels), and
// written again when the setting changes; a description that would repeat
// the provider the label says is left "via magpie".
func TestClaudePickerFollowsSuffixMode(t *testing.T) {
	home := syncHome(t)
	if err := provider.Save(provider.Provider{ID: "relay", Name: "Relay", Key: "k", Chat: "http://127.0.0.1:1/v1", Models: []string{"glm-4.6", "glm-5"}}); err != nil {
		t.Fatal(err)
	}
	if err := provider.SetModelName("relay/glm-5", "My GLM"); err != nil {
		t.Fatal(err)
	}
	if err := provider.SaveGroup(provider.Group{ID: "mix", Name: "Mix", Members: []string{"relay/glm-4.6", "relay/glm-5"}}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".claude", "settings.json")
	writeFile(t, path, `{"theme":"dark"}`)
	if err := claude(home).Apply("model", "relay/glm-4.6"); err != nil {
		t.Fatal(err)
	}
	catalog.Changed = SyncCatalog
	t.Cleanup(func() { catalog.Changed = nil })
	rows := func() map[string][2]string {
		t.Helper()
		var p struct {
			Options []struct{ Model, Label, Description string }
		}
		raw, _ := edit.GetJSON(path, "modelPicker")
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			t.Fatalf("modelPicker: %v\n%s", err, readFile(path))
		}
		out := map[string][2]string{}
		for _, o := range p.Options {
			out[o.Model] = [2]string{o.Label, o.Description}
		}
		return out
	}
	for _, c := range []struct {
		mode            string
		glm, mine, mix  string
		glmNote, mixNot string
	}{
		{provider.SuffixOn, "GLM-4.6 · Relay", "My GLM · Relay", "Mix · routing group", "via magpie", "via magpie"},
		{provider.SuffixOwn, "GLM-4.6 · Relay", "My GLM", "Mix", "via magpie", "routing group · via magpie"},
		{provider.SuffixOff, "GLM-4.6", "My GLM", "Mix", "Relay · via magpie", "routing group · via magpie"},
		{provider.SuffixOn, "GLM-4.6 · Relay", "My GLM · Relay", "Mix · routing group", "via magpie", "via magpie"},
	} {
		if err := provider.SetSuffixMode(c.mode); err != nil {
			t.Fatal(err)
		}
		got := rows()
		want := map[string][2]string{
			"relay/glm-4.6": {c.glm, c.glmNote},
			"relay/glm-5":   {c.mine, ""},
			"group/mix":     {c.mix, c.mixNot},
		}
		for id, w := range want {
			g, ok := got[id]
			if !ok {
				for k := range got {
					if k == id+"[1m]" {
						g, ok = got[k], true
					}
				}
			}
			if !ok || g[0] != w[0] || w[1] != "" && g[1] != w[1] {
				t.Fatalf("%s: %s is %q, want %q (%v)", c.mode, id, g, w, got)
			}
		}
	}
}
