package settings

import "testing"

// Show in Dock's Hide (DockNever) is this computer's own, as Dock and
// DockWindow are: a sync or restored backup from a Mac that left it as it
// came never takes a Hide chosen here away, nor brings one.
func TestDockNeverIsMachineLocal(t *testing.T) {
	from := Settings{}
	from.KeepOwn(Settings{DockNever: true})
	if !from.DockNever {
		t.Fatal("a sync cleared this Mac's Hide")
	}
	from = Settings{DockNever: true}
	from.KeepOwn(Settings{})
	if from.DockNever {
		t.Fatal("a sync brought another Mac's Hide")
	}
}
