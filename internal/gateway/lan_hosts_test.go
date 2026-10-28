package gateway

import (
	"net"
	"slices"
	"testing"
)

// A computer whose ISP assigns a static public address to its network
// card is reached there as well as at its private one: both are offered,
// in the interfaces' order. Loopback, link-local and IPv6 are not.
func TestLANHostsKeepAPublicAddress(t *testing.T) {
	t.Setenv("MAGPIE_PUBLIC_URL", "")
	t.Setenv("MAGPIE_ADDR", "")
	was := hostIPs
	t.Cleanup(func() { hostIPs = was })
	hostIPs = func() []net.IP {
		var out []net.IP
		for _, s := range []string{"203.236.50.70", "10.77.70.2", "100.64.0.1", "169.254.3.4", "127.0.0.2", "fe80::1", "2001:db8::5"} {
			out = append(out, net.ParseIP(s))
		}
		return out
	}
	want := []string{"203.236.50.70", "10.77.70.2", "100.64.0.1"}
	if got := LANHosts(); !slices.Equal(got, want) {
		t.Fatalf("LANHosts: %v, want %v", got, want)
	}
	var urls []string
	for _, h := range want {
		urls = append(urls, Scheme()+"://"+net.JoinHostPort(h, Port()))
	}
	if got := LANURLs(); !slices.Equal(got, urls) {
		t.Errorf("LANURLs: %v, want %v", got, urls)
	}
}
