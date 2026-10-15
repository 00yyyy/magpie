package library

import (
	"testing"

	"github.com/yetone/magpie/internal/agent"
)

// The Agents page calls cursor-agent Cursor CLI (@G_Z_Y on X), but the
// MCP servers and skills the Library gives it go to ~/.cursor, which the
// Cursor editor reads too: the Library's row is Cursor, and the agent
// itself is still Cursor CLI everywhere else.
func TestCursorLibraryRowIsCursor(t *testing.T) {
	sandbox(t)
	c := targetByID("cursor")
	if c == nil {
		t.Fatalf("no cursor target: %v", ids(Targets()))
	}
	if c.Agent.Name != "Cursor" {
		t.Errorf("the Library's row is %q, want Cursor", c.Agent.Name)
	}
	a, err := agent.Find("cursor")
	if err != nil {
		t.Fatal(err)
	}
	if a.Name != "Cursor CLI" {
		t.Errorf("the agent is %q after the Library named its row, want Cursor CLI", a.Name)
	}
	v, err := Read(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, av := range v.Agents {
		if av.ID == "cursor" && av.Name != "Cursor" {
			t.Errorf("the Library's view names it %q", av.Name)
		}
	}
}
