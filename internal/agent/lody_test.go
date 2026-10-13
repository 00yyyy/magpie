package agent

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLodyLink(t *testing.T) {
	for _, base := range []string{"http://127.0.0.1:3425", "http://localhost:4567", "http://[::1]:4567"} {
		t.Run(base, func(t *testing.T) {
			u, err := url.Parse(LodyLink(base))
			if err != nil {
				t.Fatal(err)
			}
			if u.Scheme != "lody" || u.Host != "provider" || u.Path != "/import" || u.Query().Get("v") != "1" || len(u.Query()) != 2 {
				t.Fatalf("invalid import URL: %s", u)
			}
			data, err := base64.RawURLEncoding.DecodeString(u.Query().Get("data"))
			if err != nil {
				t.Fatal(err)
			}
			var got map[string]any
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatal(err)
			}
			want := map[string]any{
				"kind": "custom", "id": "magpie", "name": "Magpie",
				"auth": map[string]any{"method": "apiKey", "apiKey": "magpie-lody"},
				"endpoints": []any{
					map[string]any{"protocol": "anthropic-messages", "baseUrl": base, "targets": []any{"claude-code"}, "modelsUrl": base + "/v1/models"},
					map[string]any{"protocol": "openai-responses", "baseUrl": base + "/v1", "targets": []any{"codex"}, "modelsUrl": base + "/v1/models"},
					map[string]any{"protocol": "openai-chat", "baseUrl": base + "/v1", "targets": []any{"pi", "dsh", "kimi-code", "grok", "bub"}, "modelsUrl": base + "/v1/models"},
				},
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("wrong Lody envelope: %s", data)
			}
			if LodyLink(base+"/") != LodyLink(base) {
				t.Fatal("trailing slash changed the endpoint")
			}
		})
	}
	for raw, canonical := range map[string]string{"http://localhost:80": "http://localhost", "http://127.0.0.1:03425": "http://127.0.0.1:3425"} {
		if LodyLink(raw) != LodyLink(canonical) {
			t.Errorf("%s must use canonical origin %s", raw, canonical)
		}
	}
	for _, base := range []string{"", "https://127.0.0.1:3425", "http://192.168.1.2:3425", "http://example.com", "http://127.0.0.1.evil.test", "http://user:secret@127.0.0.1", "http://127.0.0.1/v1", "http://localhost?token=secret", "http://localhost#fragment", "http://localhost?", "http://localhost:0", "http://localhost:65536", "http://localhost:", "http://::1"} {
		if got := LodyLink(base); got != "" {
			t.Errorf("unsupported gateway %q generated %s", base, got)
		}
	}
}

func TestLodyDetectionAndRegistration(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("APPDATA", filepath.Join(home, "config"))
	a := lody(home)
	if a.Detected() {
		t.Fatal("detected Lody in an empty home")
	}
	for _, name := range []string{".lody", ".lody-oss"} {
		dir := filepath.Join(home, name)
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if !a.Detected() {
			t.Fatalf("missed %s", name)
		}
		if err := os.Remove(dir); err != nil {
			t.Fatal(err)
		}
	}
	if a.Added != nil || len(a.Fields) != 0 || a.Sync != nil || a.Unwire != nil {
		t.Fatal("import adapter must not infer status or edit Lody settings")
	}
	for _, a := range All() {
		if a.ID == "lody" {
			if a.Import == nil || a.Name != "Lody" || a.Icon != "lody" {
				t.Fatalf("bad registered adapter: %+v", a)
			}
			return
		}
	}
	t.Fatal("Lody missing from Agents")
}
