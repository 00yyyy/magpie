package gui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/agent"
)

type importWindows struct {
	Windows
	opened string
}

func (w *importWindows) OpenURL(u string) error { w.opened = u; return nil }

func TestLodyImportHandoff(t *testing.T) {
	home := sandboxHome(t)
	if err := os.MkdirAll(filepath.Join(home, ".lody"), 0700); err != nil {
		t.Fatal(err)
	}
	var row *agentJSON
	s := state()
	for i := range s.Agents {
		if s.Agents[i].ID == "lody" {
			row = &s.Agents[i]
			break
		}
	}
	if row == nil || !strings.HasPrefix(row.Import, agent.LodyScheme) {
		t.Fatalf("missing Lody import row: %+v", row)
	}
	if row.Added != nil {
		t.Fatal("unknown import status must not be reported as not added")
	}
	w := &importWindows{}
	mux := http.NewServeMux()
	providerRoutes(mux, w)
	for _, link := range []string{row.Import, "lody://session/example", "lody://provider/delete?all=1", "file:///tmp/example"} {
		w.opened = ""
		body, _ := json.Marshal(map[string]string{"url": link})
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/open", strings.NewReader(string(body))))
		if link == row.Import {
			if w.opened != link {
				t.Fatal("Lody import was not handed to the desktop")
			}
		} else if w.opened != "" {
			t.Fatalf("opened unrelated URL %s", w.opened)
		}
	}
	entries, err := os.ReadDir(filepath.Join(home, ".lody"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("handoff modified Lody data: %v %v", entries, err)
	}
}
