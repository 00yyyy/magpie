package agent

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/yetone/magpie/internal/appdir"
	"github.com/yetone/magpie/internal/desktopdir"
	"github.com/yetone/magpie/internal/proc"
)

// An agent uninstalled outside magpie leaves its folder behind (~/.dsh,
// ~/.hermes/config.yaml, ~/.claude), and detection, which goes by that
// folder, still lists it. Such an agent is shown as its CLI gone, with
// nothing of its settings offered, since nothing would read them (#2177,
// mengyianx; #2173, SkyTodInfi: Hermes listed by its config.yaml alone).
// Only where magpie can tell the folder holds nothing a program still
// reads: an agent whose folder an app, an editor's extension or another
// program keeps too is gone only when none of those is here either.

// cliOnly are the agents with no install command magpie offers whose
// folder only their CLI keeps, and whose installer puts it where
// cliHeld looks: Hermes' install.sh clones itself into <home>/hermes-agent
// and links ~/.local/bin/hermes to it.
var cliOnly = map[string]bool{"hermes": true}

// cliHeld reports whether something here is the agent, or reads its
// folder as the agent does, so the folder is no leftover; known is false
// when magpie can't tell, which never counts as gone.
func (a *Agent) cliHeld() (held, known bool) {
	if a.Bin != "" && (binAt(a.Bin) != "" || proc.FindTool(a.Bin) != "") {
		if a.ID != "goose" {
			return true, true
		}
		// pressly's goose, a Go program, is not Block's (see goose's detect)
		for _, bin := range []string{binAt(a.Bin), proc.FindTool(a.Bin)} {
			if bin != "" && !goProgram(bin) {
				return true, true
			}
		}
	}
	if a.HasApp() {
		return true, true
	}
	if a.sign != nil && a.sign() {
		return true, true
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return false, false
	}
	// where the vendor's own installer puts it, off PATH until a new shell
	var at []string
	switch a.ID {
	case "claude":
		at = []string{filepath.Join(home, ".claude", "local", exe("claude"))}
	case "opencode":
		at = []string{filepath.Join(home, ".opencode", "bin", exe("opencode"))}
	case "codex":
		at = []string{filepath.Join(home, ".codex", "packages", "standalone")}
	case "hermes":
		at = []string{filepath.Join(a.Dir, "hermes-agent")}
	}
	for _, p := range at {
		if _, err := os.Stat(p); err == nil {
			return true, true
		}
	}
	if !cliShared[a.ID] {
		return false, true
	}
	// the others that keep a shared folder: editors' extensions, desktop
	// apps, and the IDEs and editors whose agents bring their own copy of
	// the program
	switch a.ID {
	case "claude":
		if extensionHere(home, "anthropic.claude-code") || appHere("Claude.app") ||
			desktopdir.Installed(desktopdir.OS, home, appdir.Getenv) {
			return true, true
		}
		return false, !jetBrainsHere(home) && !zedAgentsHere(home)
	case "codex":
		if extensionHere(home, "openai.chatgpt") {
			return true, true
		}
		return false, !jetBrainsHere(home) && !zedAgentsHere(home)
	case "cline":
		if extensionHere(home, "saoudrizwan.claude-dev") {
			return true, true
		}
		return false, !jetBrainsHere(home)
	case "opencode":
		// its desktop app: known by its bundle on a Mac only
		if runtime.GOOS != "darwin" {
			return false, false
		}
		return appHere("OpenCode.app", "OpenCode Beta.app", "OpenCode Dev.app"), true
	case "goose":
		// Goose Desktop: known by its bundle on a Mac only
		if runtime.GOOS != "darwin" {
			return false, false
		}
		return appHere("Goose.app"), true
	}
	// Gemini CLI's folder is Antigravity's and Gemini Code Assist's too,
	// and Gemini is listed only by its program (gemini.go)
	return false, false
}

// extensionHere reports whether an editor here has the extension id
// installed: VS Code's ~/.vscode/extensions/<id>-<version>, and the same
// in each fork's (~/.cursor, ~/.windsurf, ~/.vscode-oss, ~/.trae, …) and
// in VS Code Server's.
func extensionHere(home, id string) bool {
	es, err := os.ReadDir(home)
	if err != nil {
		return false
	}
	for _, e := range es {
		if !strings.HasPrefix(e.Name(), ".") {
			continue
		}
		xs, err := os.ReadDir(filepath.Join(home, e.Name(), "extensions"))
		if err != nil {
			continue
		}
		for _, x := range xs {
			if n := strings.ToLower(x.Name()); x.IsDir() && strings.HasPrefix(n, id+"-") {
				return true
			}
		}
	}
	return false
}

// appHere reports whether one of the bundles names is in a Mac's app
// folders.
func appHere(names ...string) bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	for _, dir := range appFolders() {
		for _, n := range names {
			if isDir(filepath.Join(dir, n)) {
				return true
			}
		}
	}
	return false
}

// jetBrainsHere reports whether a JetBrains IDE has kept its settings here:
// its AI Assistant's agents (Claude Agent, Codex) and Cline's plugin read
// the agents' folders, with a program of their own.
func jetBrainsHere(home string) bool {
	var dirs []string
	switch runtime.GOOS {
	case "darwin":
		dirs = []string{filepath.Join(home, "Library", "Application Support", "JetBrains")}
	case "windows":
		if d := appdir.Getenv("APPDATA"); d != "" {
			dirs = append(dirs, filepath.Join(d, "JetBrains"))
		}
		dirs = append(dirs, filepath.Join(home, "AppData", "Roaming", "JetBrains"))
	default:
		dirs = []string{filepath.Join(home, ".config", "JetBrains"), filepath.Join(home, ".local", "share", "JetBrains")}
	}
	for _, d := range dirs {
		if isDir(d) {
			return true
		}
	}
	return false
}

// zedAgentsHere reports whether Zed has fetched an external agent: its
// Claude Code and Codex adapters run their own copy of the program, which
// reads ~/.claude or ~/.codex.
func zedAgentsHere(home string) bool {
	var d string
	switch runtime.GOOS {
	case "darwin":
		d = filepath.Join(home, "Library", "Application Support", "Zed")
	case "windows":
		local := appdir.Getenv("LOCALAPPDATA")
		if local == "" {
			local = filepath.Join(home, "AppData", "Local")
		}
		d = filepath.Join(local, "Zed")
	default:
		d = filepath.Join(home, ".local", "share", "zed")
	}
	return isDir(filepath.Join(d, "external_agents"))
}
