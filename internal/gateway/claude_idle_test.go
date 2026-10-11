package gateway

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/proc"
)

func idleTestBridge() *subscriptionBridge {
	return &subscriptionBridge{runs: map[string]*subscriptionRun{}, calls: map[string]*subscriptionRun{}, idle: map[string]*subscriptionRun{}, shelf: map[string]*savedSession{}}
}

// A run kept again moves to its new key: left under the old one too, a request there would
// take a run already waiting elsewhere.
func TestClaudeIdleRunMoves(t *testing.T) {
	b := idleTestBridge()
	r := &subscriptionRun{bridge: b}
	b.keepIdle(r, "before", "conversation")
	b.keepIdle(r, "after", "conversation")
	if len(b.idle) != 1 || b.idle["after"] != r {
		t.Fatalf("one run kept under %d keys; the old one still there: %t", len(b.idle), b.idle["before"] != nil)
	}
	b.removeRun(r)
	if len(b.idle) != 0 {
		t.Fatal("an ended run is still there for another request")
	}
}

// A run another took the key of, kept again, leaves that other run where
// it waits.
func TestClaudeIdleReplacementKeepsItsKey(t *testing.T) {
	b := idleTestBridge()
	old := &subscriptionRun{bridge: b, closed: true}
	replacement := &subscriptionRun{bridge: b}
	b.keepIdle(old, "same", "")
	b.keepIdle(replacement, "same", "")
	b.keepIdle(old, "other", "")
	if b.idle["same"] != replacement || b.idle["other"] != old {
		t.Fatal("moving a run took out the one that replaced it")
	}
}

// X, Sma1lboy: magpie grew to 158 GB in two days. One run under two keys
// was the oldest on every pass of keepIdle's loop past idleMost, which
// took out one key only, so the loop never ended and grew its drop list
// with the bridge's lock held. Run in a child, which the old loop is taken
// down with rather than left spinning in the suite. The child fails on what
// the loop allocates (drop), not on a deadline: the loop that ends is done
// in microseconds, but a busy machine may take seconds to start the child.
func TestClaudeIdleEvictionEnds(t *testing.T) {
	const child = "MAGPIE_TEST_IDLE_EVICTION"
	if scenario := os.Getenv(child); scenario != "" {
		idleEvictionChild(t, scenario)
		return
	}
	for _, scenario := range []string{"rekey", "stale"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			cmd := proc.CommandContext(ctx, os.Args[0], "-test.run=^TestClaudeIdleEvictionEnds$", "-test.timeout=4m")
			cmd.Env = append(os.Environ(), child+"="+scenario)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("keepIdle past idleMost did not end: %v\n%s", err, out)
			}
		})
	}
}

func idleEvictionChild(t *testing.T, scenario string) {
	b := idleTestBridge()
	stale := &subscriptionRun{bridge: b, closed: true}
	if scenario == "stale" {
		// an entry whose run says it waits elsewhere
		b.idle["before"] = stale
	} else {
		b.keepIdle(stale, "before", "")
		b.keepIdle(stale, "after", "")
	}
	// the longest waiting, set after keepIdle stamps it: a coarse clock
	// (Windows) would otherwise give the runs below the same time
	stale.idleAt = time.Unix(1, 0)
	done := make(chan struct{})
	go func() {
		for i := 0; i < idleMost+2; i++ {
			b.keepIdle(&subscriptionRun{bridge: b, closed: true}, fmt.Sprint(i), "")
		}
		close(done)
	}()
	var at runtime.MemStats
	runtime.ReadMemStats(&at)
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-done:
			if len(b.idle) != idleMost || b.idle["before"] != nil || b.idle["after"] != nil {
				t.Fatalf("kept %d runs; the oldest still there: %t", len(b.idle), b.idle["before"] != nil || b.idle["after"] != nil)
			}
			return
		case <-tick.C:
			var now runtime.MemStats
			runtime.ReadMemStats(&now)
			if grew := now.TotalAlloc - at.TotalAlloc; grew > 256<<20 {
				// the loop holds b.mu and never returns: the process ends
				fmt.Fprintf(os.Stderr, "keepIdle allocated %d MiB without ending\n", grew>>20)
				os.Exit(2)
			}
		}
	}
}
