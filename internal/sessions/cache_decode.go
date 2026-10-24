package sessions

import (
	"encoding/json"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/tidwall/gjson"
)

// decodeCache is the kept file b as json.Unmarshal reads it, with the files
// decoded on a few goroutines: one Unmarshal of a long history's 40MB took
// 320 to 440ms of every start (#3493). False when b isn't valid JSON, is of
// another version or zone, or a file's parse doesn't decode.
func decodeCache(b []byte, zone string) (map[string]*state, bool) {
	if !gjson.ValidBytes(b) {
		return nil, false
	}
	// b is never changed, and nothing read from it is kept but cloned keys
	s := unsafe.String(unsafe.SliceData(b), len(b))
	head := gjson.GetMany(s, "version", "zone", "files")
	if head[0].Type != gjson.Number || head[0].Int() != cacheVersion || head[1].String() != zone || !head[2].IsObject() {
		return nil, false
	}
	var keys, raws []string
	head[2].ForEach(func(k, v gjson.Result) bool {
		keys = append(keys, strings.Clone(k.String()))
		raws = append(raws, v.Raw)
		return true
	})
	parsed := make([]*state, len(raws))
	var bad atomic.Bool
	one := func(i int) {
		if json.Unmarshal(unsafe.Slice(unsafe.StringData(raws[i]), len(raws[i])), &parsed[i]) != nil {
			bad.Store(true)
		}
	}
	if workers := min(runtime.GOMAXPROCS(0), 8, len(raws)/256); workers < 2 {
		for i := range raws {
			one(i)
		}
	} else {
		var next atomic.Int64
		var wg sync.WaitGroup
		for range workers {
			wg.Go(func() {
				for {
					i := int(next.Add(64)) - 64
					if i >= len(raws) || bad.Load() {
						return
					}
					for j := i; j < min(i+64, len(raws)); j++ {
						one(j)
					}
				}
			})
		}
		wg.Wait()
	}
	if bad.Load() {
		return nil, false
	}
	files := make(map[string]*state, len(keys))
	for i, k := range keys {
		files[k] = parsed[i] // a key given twice is its last, as Unmarshal has it
	}
	return files, true
}
