package provider

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// own the same way, and shows it again once it is another account.
func TestGrokHideOwnSignIn(t *testing.T) {
	home := signIn(t)
	own := filepath.Join(home, ".grok")
	grokSignedIn(t, own, "me@x.ai")
	extra, _ := newGrokHome()
	grokSignedIn(t, extra, "two@x.ai")
	if _, err := addGrokLogin(extra); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(own, "auth.json"))
	if err := ForgetLogin("grok", "me@x.ai"); err != nil {
		t.Fatal(err)
	}
	if ls := Logins("grok"); len(ls) != 1 || ls[0].User != "two@x.ai" || !ls[0].Active {
		t.Fatalf("after hide %+v", ls)
	}
	if after, _ := os.ReadFile(filepath.Join(own, "auth.json")); !bytes.Equal(before, after) {
		t.Fatal("the CLI's own sign-in was touched")
	}
	grokSignedIn(t, own, "new@x.ai")
	if ls := Logins("grok"); len(ls) != 2 || ls[0].User != "two@x.ai" || ls[1].User != "new@x.ai" || !ls[1].Own {
		t.Fatalf("another account %+v", ls)
	}
}
