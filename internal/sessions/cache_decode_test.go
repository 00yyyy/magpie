package sessions

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// The kept file decoded on a few goroutines is what one json.Unmarshal of
// it was, and is refused where that was.
func TestDecodeCacheIsUnmarshal(t *testing.T) {
	unmarshal := func(b []byte, zone string) (map[string]*state, bool) {
		var c cacheFile
		if json.Unmarshal(b, &c) != nil || c.Version != cacheVersion || c.Zone != zone || c.Files == nil {
			return nil, false
		}
		return c.Files, true
	}
	var many strings.Builder
	fmt.Fprintf(&many, `{"version":%d,"zone":"z","files":{`, cacheVersion)
	for i := range 3000 {
		if i > 0 {
			many.WriteString(",")
		}
		fmt.Fprintf(&many, `"/s/%d.jsonl":{"size":%d,"mod":%d,"title":"té %d","models":{"m":{"input":%d}},"claude":[{"key":"message:%d","m":{"request":"r%d"}}],"days":{"2026-10-%02d":{"models":{"m":{"output":%d}}}}}`, i, i, i*7, i, i, i, i, 1+i%28, i)
	}
	many.WriteString(`}}`)
	v := cacheVersion
	cases := map[string]string{
		"many":         many.String(),
		"escaped keys": fmt.Sprintf(`{"version":%d,"zone":"z","files":{"/a\"b\\cé.jsonl":{"size":1},"/n":null}}`, v),
		"twice":        fmt.Sprintf(`{"zone":"z","files":{"/a":{"size":1},"/a":{"size":2}},"version":%d}`, v),
		"empty":        fmt.Sprintf(`{"version":%d,"zone":"z","files":{}}`, v),
		"no files":     fmt.Sprintf(`{"version":%d,"zone":"z","files":null}`, v),
		"old version":  fmt.Sprintf(`{"version":%d,"zone":"z","files":{"/a":{"size":1}}}`, v-1),
		"other zone":   fmt.Sprintf(`{"version":%d,"zone":"y","files":{"/a":{"size":1}}}`, v),
		"bad file":     fmt.Sprintf(`{"version":%d,"zone":"z","files":{"/a":{"size":"one"}}}`, v),
		"bad late":     strings.Replace(many.String(), `"size":2999`, `"size":"x"`, 1),
		"cut":          many.String()[:many.Len()-5],
		"trailing":     many.String() + " x",
		"no comma":     fmt.Sprintf(`{"version":%d,"zone":"z","files":{"/a":{"size":1} "/b":{"size":2}}}`, v),
		"garbage":      "garbage",
		"empty bytes":  "",
	}
	for name, in := range cases {
		want, wantOK := unmarshal([]byte(in), "z")
		got, ok := decodeCache([]byte(in), "z")
		if ok != wantOK || !reflect.DeepEqual(got, want) {
			wj, _ := json.Marshal(want)
			gj, _ := json.Marshal(got)
			t.Errorf("%s: got %v %.300s, want %v %.300s", name, ok, gj, wantOK, wj)
		}
	}
}
