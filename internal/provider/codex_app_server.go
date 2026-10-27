package provider

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/yetone/magpie/internal/proc"
)

// The Codex app greys its Send for every model, magpie's too, while the
// ChatGPT account it is signed in to is used up (its composer's gate,
// XP in ChatGPT.app 26.1013's app-primary: authMethod "chatgpt" and
// rate_limit.allowed false). It reads authMethod from its app-server's
// account/read, which answers no account once magpie is Codex's provider
// (requires_openai_auth false), but it asks only as the app-server
// starts. So when magpie becomes Codex's provider for the used-up
// account (the agent package's AppOut), and again when it puts Codex
// back on the account after its reset, magpie ends the app's own
// app-server, the one it started with: the app starts another at once,
// reads account/read again, and sends (tried in ChatGPT.app 26.1013 on
// macOS; its window, threads and drafts stay). It does that on every
// system the app runs on, with nothing for the user to do, once no Codex
// turn is running: none through magpie, none open in Codex's sessions.
//
// The app-server is ended as a clean exit (proc.End): the app reads any
// other exit as a crash and shows it.

// CodexAppOnMagpie is set by the agent package: whether magpie is Codex's
// provider for its used-up ChatGPT account (AppOut), known false when
// Codex's config can't be read.
var CodexAppOnMagpie func() (on, known bool)

// codexAppEvery is how often KeepCodexAppOnItsProvider looks; codexAppIdle
// how long after Codex's last request through magpie a turn counts ended
// (a tool runs between two of a turn's requests); codexAppTries how often
// one app-server is ended before magpie says it can't.
var (
	codexAppEvery = 3 * time.Second
	codexAppIdle  = 10 * time.Second
	codexAppTries = 3
)

// codexTurnWindow is how long a session file written to last counts as a
// turn maybe still running when it has one started and not ended: a turn
// whose app-server died leaves one open for good.
const codexTurnWindow = 30 * time.Minute

// codexAppListWait bounds one listing of the app-servers and their homes:
// at a load of 200 a 10s bound ran out on the owner's Mac (the change was
// taken up at the next look, 9s later).
const codexAppListWait = 30 * time.Second

// codexAppServersOn are the pids of the Codex app's own app-servers that
// run on the Codex home home; vars for tests.
var (
	codexAppServersOn = func(ctx context.Context, home string) ([]int, error) {
		ps, err := listProcesses(ctx)
		if err != nil {
			return nil, err
		}
		var out []int
		for _, p := range ps {
			if isCodexAppOwnServer(p.Args) && codexAppHomeIs(ctx, p.PID, home) {
				out = append(out, p.PID)
			}
		}
		return out, nil
	}
	endCodexAppServer = proc.End
	codexTurnRunning  = func(home string, now time.Time) bool { return codexTurnOpen(home, now) }
)

// isCodexAppOwnServer is whether a command line is the Codex app's own
// app-server, which it talks to over stdio: one another program starts
// from the app's bundled codex names where it listens (`app-server
// --listen stdio://`, Miyi Helper on macOS), and is that program's to end.
func isCodexAppOwnServer(args string) bool {
	if !isCodexAppServer(args) {
		return false
	}
	_, rest := splitExe(args)
	for _, f := range strings.Fields(rest) {
		if f == "--listen" || strings.HasPrefix(f, "--listen=") {
			return false
		}
	}
	return true
}

// codexAppHomeIs is whether app-server pid runs on Codex home home: its
// CODEX_HOME, else its HOME's .codex. A system that doesn't tell
// (Windows) counts as yes: one Codex home is the rule there.
func codexAppHomeIs(ctx context.Context, pid int, home string) bool {
	env, err := proc.EnvOf(ctx, pid, "CODEX_HOME", "HOME")
	if errors.Is(err, proc.ErrNoEnv) {
		return true
	}
	if err != nil {
		return false
	}
	h := env["CODEX_HOME"]
	if h == "" && env["HOME"] != "" {
		h = filepath.Join(env["HOME"], ".codex")
	}
	return h != "" && sameDir(h, home)
}

// sameDir is whether a and b name one folder, through links too.
func sameDir(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if a == b {
		return true
	}
	ra, err1 := filepath.EvalSymlinks(a)
	rb, err2 := filepath.EvalSymlinks(b)
	return err1 == nil && err2 == nil && ra == rb
}

// codexApp is what KeepCodexAppOnItsProvider knows.
var codexApp struct {
	sync.Mutex
	seen, on bool
	// stale: the app-servers started before Codex's provider last changed,
	// each with how often it was ended
	stale map[int]int
	// stuck: one of them outlived codexAppTries ends, or may not be ended
	stuck bool
}

// CodexAppStuck is whether a Codex app is left on the provider it started
// with, which magpie couldn't restart: the user quits and opens it.
func CodexAppStuck() bool {
	c := &codexApp
	c.Lock()
	defer c.Unlock()
	return c.stuck
}

// KeepCodexAppOnItsProvider runs codexAppStep every codexAppEvery till
// ctx ends. busy says how many of Codex's requests are in flight through
// magpie and when the last ended.
func KeepCodexAppOnItsProvider(ctx context.Context, busy func() (int, time.Time)) {
	t := time.NewTicker(codexAppEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			codexAppStep(ctx, now, busy)
		}
	}
}

// codexAppStep notes the app-servers running as magpie becomes Codex's
// provider for the used-up account, or stops being it, and ends each of
// them once no Codex turn is running. Started with magpie on, the app may
// have opened before: its app-servers are ended too. It says which pids it
// ended.
func codexAppStep(ctx context.Context, now time.Time, busy func() (int, time.Time)) []int {
	if CodexAppOnMagpie == nil {
		return nil
	}
	on, known := CodexAppOnMagpie()
	if !known {
		return nil
	}
	home := codexHome()
	c := &codexApp
	c.Lock()
	changed := !c.seen && on || c.seen && c.on != on
	if !changed {
		c.seen, c.on = true, on
	}
	c.Unlock()
	if changed {
		lctx, cancel := context.WithTimeout(ctx, codexAppListWait)
		pids, err := codexAppServersOn(lctx, home)
		cancel()
		if err != nil {
			// unknown: the change is taken up again at the next look
			log.Printf("codex: listing the Codex app's app-servers: %v", err)
			return nil
		}
		c.Lock()
		c.seen, c.on = true, on
		c.stale, c.stuck = map[int]int{}, false
		for _, p := range pids {
			c.stale[p] = 0
		}
		c.Unlock()
	}
	c.Lock()
	n := len(c.stale)
	c.Unlock()
	if n == 0 {
		return nil
	}
	lctx, cancel := context.WithTimeout(ctx, codexAppListWait)
	pids, err := codexAppServersOn(lctx, home)
	cancel()
	if err != nil {
		return nil
	}
	running := map[int]bool{}
	for _, p := range pids {
		running[p] = true
	}
	c.Lock()
	for p := range c.stale {
		if !running[p] {
			delete(c.stale, p) // it read the provider afresh as it started
		}
	}
	n = len(c.stale)
	c.Unlock()
	if n == 0 {
		return nil
	}
	if inFlight, last := busy(); inFlight > 0 || now.Sub(last) < codexAppIdle {
		return nil
	}
	if codexTurnRunning(home, now) {
		return nil
	}
	var ended []int
	c.Lock()
	defer c.Unlock()
	for p, tries := range c.stale {
		if tries >= codexAppTries {
			c.stuck = true
			continue
		}
		c.stale[p] = tries + 1
		if err := endCodexAppServer(p); err != nil {
			log.Printf("codex: restarting the Codex app's app-server %d on its provider: %v", p, err)
			if errors.Is(err, proc.ErrDenied) {
				c.stale[p], c.stuck = codexAppTries, true
			}
			continue
		}
		ended = append(ended, p)
	}
	return ended
}

// codexTurnOpen is whether a Codex session under home has a turn started
// and not ended (task_started after its last task_complete or
// turn_aborted) in a file written to within codexTurnWindow of now: the
// app-server running it would cut it short.
func codexTurnOpen(home string, now time.Time) bool {
	open := false
	_ = filepath.WalkDir(filepath.Join(home, "sessions"), func(path string, d fs.DirEntry, err error) error {
		if err != nil || open {
			if open {
				return fs.SkipAll
			}
			return nil
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".jsonl") {
			return nil
		}
		info, err := d.Info()
		if err != nil || now.Sub(info.ModTime()) > codexTurnWindow {
			return nil
		}
		open = sessionTurnOpen(path)
		return nil
	})
	return open
}

// sessionTurnTail is how much of a session file's end sessionTurnOpen reads.
const sessionTurnTail = 512 << 10

// sessionTurnOpen is whether the session file at path ends inside a turn.
// A file that can't be read counts as open: a turn cut short costs more
// than an app greyed a few seconds more.
func sessionTurnOpen(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return !errors.Is(err, fs.ErrNotExist)
	}
	defer f.Close()
	if info, err := f.Stat(); err == nil && info.Size() > sessionTurnTail {
		if _, err := f.Seek(info.Size()-sessionTurnTail, io.SeekStart); err != nil {
			return true
		}
	}
	b, err := io.ReadAll(f)
	if err != nil {
		return true
	}
	started := bytes.LastIndex(b, []byte(`"type":"task_started"`))
	ended := max(bytes.LastIndex(b, []byte(`"type":"task_complete"`)), bytes.LastIndex(b, []byte(`"type":"turn_aborted"`)))
	return started > ended
}
