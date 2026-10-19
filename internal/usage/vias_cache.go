package usage

import (
	"slices"
	"sync"
	"time"
)

var viasCache struct {
	sync.Mutex
	version uint64
	since   time.Time
	clients string // the clients counted as agents (#1958)
	value   map[string][]Via
}

// ViaIndex is Vias' answer: the one copy every caller shares, handed out
// whole and read only through Of, which gives each session's list as a
// copy of its own. Each ask cloned every session's list, ~66 MB a time
// over ~720k calls, where a page reads the few sessions it lists (#2776).
type ViaIndex struct{ m map[string][]Via }

// Of is what the gateway sent one session's calls to ("agent|id"), the
// most calls first; nil for a session it sent none of.
func (x ViaIndex) Of(key string) []Via { return slices.Clone(x.m[key]) }

// Len is how many sessions have a list.
func (x ViaIndex) Len() int { return len(x.m) }

func cachedVias(since time.Time) ViaIndex {
	snapshot := readLogSnapshot()
	clients := clientAgentsKey()
	viasCache.Lock()
	defer viasCache.Unlock()
	value := viasCache.value
	if value == nil || viasCache.version != snapshot.version || !viasCache.since.Equal(since) || viasCache.clients != clients {
		value = buildVias(snapshot, since)
		if snapshot.info != nil && snapshot.off == snapshot.info.Size() {
			viasCache.version, viasCache.since, viasCache.value, viasCache.clients = snapshot.version, since, value, clients
		}
	}
	return ViaIndex{value}
}
