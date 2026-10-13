package cli

import (
	"testing"

	"github.com/yetone/magpie/internal/provider"
)

// #2008: `magpie provider quota-rest <id> until 00:00+08:00` sets the time
// of day the allowance comes back, a number of minutes after it takes its
// place, and default lets both go.
func TestQuotaRestUntil(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := provider.Save(provider.Provider{ID: "xk", Name: "xk", Chat: "https://example.invalid/v1", Key: "k"}); err != nil {
		t.Fatal(err)
	}
	find := func() provider.Provider {
		p, err := provider.Find("xk")
		if err != nil {
			t.Fatal(err)
		}
		return *p
	}
	if err := quotaRestCmd([]string{"xk", "until", "00:00+08:00"}); err != nil {
		t.Fatal(err)
	}
	if p := find(); p.QuotaResetAt != "00:00+08:00" {
		t.Fatalf("until: %q", p.QuotaResetAt)
	}
	if err := quotaRestCmd([]string{"xk", "until", "25:00"}); err == nil {
		t.Fatal("25:00 taken")
	}
	if err := quotaRestCmd([]string{"xk", "6h"}); err != nil {
		t.Fatal(err)
	}
	if p := find(); p.QuotaResetAt != "" || p.QuotaRest != 360 {
		t.Fatalf("6h: %q %d", p.QuotaResetAt, p.QuotaRest)
	}
	if err := quotaRestCmd([]string{"xk", "at", "8:00"}); err != nil {
		t.Fatal(err)
	}
	if err := quotaRestCmd([]string{"xk", "default"}); err != nil {
		t.Fatal(err)
	}
	if p := find(); p.QuotaResetAt != "" || p.QuotaRest != 0 {
		t.Fatalf("default: %q %d", p.QuotaResetAt, p.QuotaRest)
	}
}
