package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const zcodeBuiltinJSON = `{"revision":%d,"config":{
 "providerConfigRules":{"providerRules":[
  {"providerId":"account:zai-individual-coding-plan","builtinModelIds":["GLM-5.3"]},
  {"providerId":"account:zai-start-plan","builtinModelIds":["GLM-5-Turbo"]}]},
 "modelConfigRules":{
  "modelRules":[
   {"modelMatch":".*","config":{"properties":{"contextWindow":200000},"optionSpecs":{"maxOutputTokens":{"max":32000},"reasoningLevel":{"values":["disabled","enabled"]}}}},
   {"modelMatch":".*glm-5\\.3(?:-flash)?(?:[.\\-:/\\[].*)?","config":{"properties":{"contextWindow":1000000},"optionSpecs":{"maxOutputTokens":{"max":128000},"reasoningLevel":{"values":["low","high","max"]}}}},
   {"modelMatch":".*glm-5\\.3-flash(?:[.\\-:/\\[].*)?","config":{"properties":{"inputFormat":{"supportsImage":true}}}}],
  "builtinProviderModelRules":[
   {"providerId":"account:zai-individual-coding-plan","modelId":"GLM-5.3-Flash","config":{"enabled":true}},
   {"providerId":"account:zai-individual-coding-plan","modelId":"GLM-5.1","config":{"enabled":false}},
   {"providerId":"account:zai-individual-coding-plan","modelId":"GLM-5-Turbo","config":{"enabled":true}},
   {"providerId":"account:bigmodel-individual-coding-plan","modelId":"GLM-X","config":{"enabled":true}}]}}}`

func zcodeBuiltinAt(rev int) string { return fmt.Sprintf(zcodeBuiltinJSON, rev) }

// A coding plan's models are its own in ZCode's built-in config, with what
// the rules say of each, from the later of the served and installed releases.
func TestZCodeFetchModels(t *testing.T) {
	var platform string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/client/configs":
			platform = r.URL.Query().Get("platform")
			json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"configs": map[string]string{"builtin_provider_config_json": "https://" + r.Host + "/cdn.json"}}})
		case "/cdn.json":
			w.Write([]byte(zcodeBuiltinAt(23)))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	oldAPI, oldFiles, oldClient := zcodeAPI, zcodeBuiltinFiles, http.DefaultClient
	defer func() { zcodeAPI, zcodeBuiltinFiles, http.DefaultClient = oldAPI, oldFiles, oldClient }()
	zcodeAPI, http.DefaultClient = srv.URL, srv.Client()
	zcodeBuiltinFiles = func() []string { return nil }

	got := func() string {
		ms, err := zcodeFetchModels(context.Background(), ZCodeZaiBase)
		if err != nil {
			t.Fatal(err)
		}
		var s []string
		for _, m := range ms {
			b, _ := json.Marshal([]any{m.ID, m.Context, m.Output, m.Images, m.Efforts})
			s = append(s, string(b))
		}
		return strings.Join(s, " ")
	}
	want := `["GLM-5.3",1000000,128000,false,["low","high","max"]] ["GLM-5.3-Flash",1000000,128000,true,["low","high","max"]] ["GLM-5-Turbo",200000,32000,false,["none","high"]]`
	if g := got(); g != want {
		t.Fatalf("got  %s\nwant %s", g, want)
	}
	if !strings.Contains(platform, "-") {
		t.Errorf("platform = %q", platform)
	}

	// an installed ZCode's later release wins; an earlier one doesn't
	dir := t.TempDir()
	f := filepath.Join(dir, "zcode-builtin.json")
	zcodeBuiltinFiles = func() []string { return []string{f} }
	os.WriteFile(f, []byte(strings.Replace(zcodeBuiltinAt(30), `"GLM-5.3"]`, `"GLM-5.3","GLM-6"]`, 1)), 0o600)
	if g := got(); !strings.Contains(g, `"GLM-6"`) {
		t.Fatalf("later local release not used: %s", g)
	}
	os.WriteFile(f, []byte(strings.Replace(zcodeBuiltinAt(10), `"GLM-5.3"]`, `"GLM-5.3","GLM-6"]`, 1)), 0o600)
	if g := got(); g != want {
		t.Fatalf("earlier local release used: %s", g)
	}

	// bigmodel.cn's plan is its own
	ms, err := zcodeFetchModels(context.Background(), ZCodeBigModelBase)
	if err != nil || len(ms) != 1 || ms[0].ID != "GLM-X" {
		t.Fatalf("%v %v", ms, err)
	}
}
