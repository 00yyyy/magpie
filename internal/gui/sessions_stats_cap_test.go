package gui

import "testing"

// ?days is any number a caller sends, and each distinct value kept a whole
// sessions.Stats in statsMemo for good: 64 values, 64 snapshots. The memo
// keeps a few, the least recently asked let go, and the newest one stays.
func TestStatsMemoKeepsAFewDays(t *testing.T) {
	forgetStats()
	t.Cleanup(forgetStats)
	for d := 1; d <= 64; d++ {
		statsFor(d)
	}
	statsMemo.Lock()
	n, newest := len(statsMemo.m), statsMemo.m[64] != nil
	statsMemo.Unlock()
	if n > 8 {
		t.Fatalf("after 64 distinct ?days values the memo holds %d, want at most 8", n)
	}
	if !newest {
		t.Fatal("the value asked last was let go")
	}
}
