package sessions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type desktopTitle struct {
	Session string   `json:"sessionId"`
	CLI     string   `json:"cliSessionId"`
	Prior   []string `json:"priorCliSessionIds"`
	Title   string   `json:"title"`
}

var desktopTitles = struct {
	sync.Mutex
	files map[string]desktopTitleFile
}{}

type desktopTitleFile struct {
	info os.FileInfo
	name desktopTitle
}

// DesktopTitles reads Code and Cowork names from Desktop's local metadata,
// even when the CLI transcript is absent. Only IDs explicitly recorded there
// are named; prior CLI runs keep their own usage identity and share the name.
func DesktopTitles(ids []string) map[string]string {
	out := map[string]string{}
	if len(ids) == 0 {
		return out
	}
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	desktopTitles.Lock()
	defer desktopTitles.Unlock()
	next := map[string]desktopTitleFile{}
	// A current ID takes precedence over a prior ID. Conflicting owners of
	// the same rank leave the name unknown instead of picking a profile.
	ranks := map[string]int{}
	add := func(id, name string, rank int) {
		if id == "" || !want[id] || rank < ranks[id] {
			return
		}
		if rank > ranks[id] {
			ranks[id], out[id] = rank, name
		} else if out[id] != name {
			out[id] = ""
		}
	}
	for _, dir := range DesktopDataDirs() {
		for _, kind := range []string{"claude-code-sessions", "local-agent-mode-sessions"} {
			paths, _ := SessionGlob(filepath.Join(dir, kind, "*", "*", "local_*.json"))
			for _, path := range paths {
				info, err := os.Stat(path)
				if err != nil || !info.Mode().IsRegular() || info.Size() > 8<<20 {
					continue
				}
				f, ok := desktopTitles.files[path]
				if !ok || !os.SameFile(f.info, info) || f.info.Size() != info.Size() || !f.info.ModTime().Equal(info.ModTime()) {
					b, err := os.ReadFile(path)
					var name desktopTitle
					if err != nil || json.Unmarshal(b, &name) != nil {
						continue
					}
					name.Title = title(name.Title)
					f = desktopTitleFile{info, name}
				}
				next[path] = f
				add(f.name.Session, f.name.Title, 2)
				add(f.name.CLI, f.name.Title, 2)
				for _, id := range f.name.Prior {
					add(id, f.name.Title, 1)
				}
			}
		}
	}
	desktopTitles.files = next
	for id, name := range out {
		if name == "" {
			delete(out, id)
		}
	}
	return out
}
