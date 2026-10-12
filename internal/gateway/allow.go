package gateway

import (
	"log"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strings"
	"sync"

	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/settings"
)

// The allow list (lc on Discord, magpie in Docker on a public host): the
// addresses other computers may reach magpie from at all, as IPs and CIDR
// ranges. A request from any other address is refused 403 before its key
// is looked at, so a gateway key that leaks is of no use from elsewhere.
// No list lets every address in, as before. This computer (loopback, and
// the computer's own addresses) is always let in.
//
// The address judged is the connection's own (r.RemoteAddr). A forwarding
// header (X-Forwarded-For and the rest) is never taken for it: anyone can
// write one, and magpie keeps no list of proxies to believe. A proxy or
// tunnel on this computer reaches magpie from loopback, so what it passes
// on is let in here, and still needs a gateway key (proxied); limit who
// reaches such a proxy at the proxy.

// AllowIPsEnv names the environment variable that sets the list, in place
// of Settings' (for Docker and `magpie web`).
const AllowIPsEnv = "MAGPIE_ALLOW_IPS"

// AllowIPs is the allow list in force, as written, and whether
// MAGPIE_ALLOW_IPS set it. Empty: no list, every address let in.
func AllowIPs() (list []string, fromEnv bool) {
	if v := strings.TrimSpace(os.Getenv(AllowIPsEnv)); v != "" {
		return settings.SplitAllowIPs(v), true
	}
	return settings.Load().AllowIPs, false
}

var warnedAllow sync.Map

// allowPrefixes is the list in force, read; on says one is set. An entry
// that doesn't read (a mistyped MAGPIE_ALLOW_IPS) lets nothing in and is
// logged once: the list stays on, so a typo never opens magpie to every
// address.
func allowPrefixes() (ps []netip.Prefix, on bool) {
	list, _ := AllowIPs()
	for _, e := range list {
		p, err := settings.ParseAllowIP(e)
		if err != nil {
			if _, done := warnedAllow.LoadOrStore(e, true); !done {
				log.Printf("%s: %v; it lets no address in", AllowIPsEnv, err)
			}
			continue
		}
		ps = append(ps, p)
	}
	return ps, len(list) > 0
}

// peerAddr is the address r's connection came from; false when
// RemoteAddr holds none.
func peerAddr(r *http.Request) (netip.Addr, bool) {
	if ap, err := netip.ParseAddrPort(r.RemoteAddr); err == nil {
		return ap.Addr().WithZone("").Unmap(), true
	}
	h, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		h = r.RemoteAddr
	}
	a, err := netip.ParseAddr(strings.Trim(h, "[]"))
	if err != nil {
		return netip.Addr{}, false
	}
	return a.WithZone("").Unmap(), true
}

// ownAddr: a is one of this computer's own addresses, which a connection
// comes from only when it was made here (an agent calling the address
// MAGPIE_ADDR names, which isn't loopback).
var ownAddr = func(a netip.Addr) bool {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return false
	}
	for _, ia := range addrs {
		if n, ok := ia.(*net.IPNet); ok {
			if b, ok := netip.AddrFromSlice(n.IP); ok && b.Unmap() == a {
				return true
			}
		}
	}
	return false
}

// AllowedFrom: a request from a may reach magpie under the allow list —
// there is none, a is this computer's, or a is in it.
func AllowedFrom(a netip.Addr) bool {
	ps, on := allowPrefixes()
	if !on || a.IsLoopback() {
		return true
	}
	for _, p := range ps {
		if p.Contains(a) {
			return true
		}
	}
	return ownAddr(a)
}

// allowedListFrom is AllowedFrom for a list not yet in force (Settings
// checking a new one won't shut out the page it was saved from).
func allowedListFrom(list []string, a netip.Addr) bool {
	if len(list) == 0 || a.IsLoopback() {
		return true
	}
	for _, e := range list {
		if p, err := settings.ParseAllowIP(e); err == nil && p.Contains(a) {
			return true
		}
	}
	return ownAddr(a)
}

// KeepsOut: the allow list list would refuse r, a request that came in
// from elsewhere; it names r's address.
func KeepsOut(list []string, r *http.Request) (string, bool) {
	a, ok := peerAddr(r)
	if !ok {
		return "", false
	}
	return a.String(), !allowedListFrom(list, a)
}

// allowed: r may reach magpie under the allow list; it names r's address
// when it may not.
func allowed(r *http.Request) (string, bool) {
	if _, on := allowPrefixes(); !on {
		return "", true
	}
	a, ok := peerAddr(r)
	if !ok {
		return r.RemoteAddr, false
	}
	return a.String(), AllowedFrom(a)
}

// notAllowed says why a request from addr was refused.
func notAllowed(addr string) string {
	return "magpie answers only the addresses on its allow list (" + AllowIPsEnv + ", or Settings → Network and sharing → Allowed addresses), and this request came from " + addr + ": add it there to let it in"
}

// AllowGuard refuses a request from an address the allow list leaves out,
// 403, before anything else looks at it: the page `magpie web` serves.
func AllowGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if addr, ok := allowed(r); !ok {
			log.Printf("refused %s %s from %s: not on the allow list", r.Method, r.URL.Path, addr)
			http.Error(w, notAllowed(addr), http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// refuseNotAllowed answers a gateway request from an address the allow
// list leaves out, in the error shape agents show; true when it did.
func refuseNotAllowed(w http.ResponseWriter, r *http.Request) bool {
	addr, ok := allowed(r)
	if ok {
		return false
	}
	log.Printf("refused %s %s from %s: not on the allow list", r.Method, r.URL.Path, addr)
	writeError(w, provider.Chat, http.StatusForbidden, notAllowed(addr))
	return true
}
