package settings

import (
	"strings"
	"testing"
)

// settings.json keeps the model set for Desktop's subagents trimmed and
// refuses a bare name.
func TestDesktopSubagentModelSetting(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	for _, id := range []string{" fake/m1 ", " group/workers ", " "} {
		if err := Save(Settings{DesktopSubagentModel: id}); err != nil {
			t.Fatal(err)
		}
		if got := Load().DesktopSubagentModel; got != strings.TrimSpace(id) {
			t.Fatalf("saved model %q, want %q", got, strings.TrimSpace(id))
		}
	}
	if err := Save(Settings{DesktopSubagentModel: "gpt-6"}); err == nil || !strings.Contains(err.Error(), "the model for Claude Desktop's subagents") {
		t.Fatalf("bare model validation: %v", err)
	}
}
