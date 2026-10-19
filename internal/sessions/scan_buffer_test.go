package sessions

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// A scan of a small session file takes a buffer the size of the file, not
// 1 MiB: a start reads thousands of them, and their buffers were most of
// what it allocated (#2776).
func TestScanOfASmallFileTakesASmallBuffer(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(p, []byte(`{"type":"user","n":1}`+"\n"+`{"type":"assistant","n":2}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	const scans = 200
	lines := 0
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	for range scans {
		if _, err := scanAt(p, 0, nil, func(b []byte, _, _ int64) bool { lines++; return true }); err != nil {
			t.Fatal(err)
		}
	}
	runtime.ReadMemStats(&after)
	if lines != 2*scans {
		t.Fatalf("read %d lines, want %d", lines, 2*scans)
	}
	// 16 KiB each, with room for the file handles and anything else
	if per := (after.TotalAlloc - before.TotalAlloc) / scans; per > 64<<10 {
		t.Fatalf("a scan of a two-line file allocated %d bytes", per)
	}
}

// A line longer than the buffer (the file grew after its size was read) is
// still handed over whole.
func TestScanReadsALineLongerThanItsBuffer(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(p, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	size := scanBuffer(f, 0)
	f.Close()
	long := `{"text":"` + strings.Repeat("x", 3*size) + `"}`
	if err := os.WriteFile(p, []byte(long+"\n{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var got []int
	if _, err := scanAt(p, 0, nil, func(b []byte, _, _ int64) bool { got = append(got, len(b)); return true }); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != len(long) || got[1] != 2 {
		t.Fatalf("lines %v, want [%d 2]", got, len(long))
	}
}
