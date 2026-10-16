package gateway

import "strings"

// said writes events as tagged runs, |text:…|call:id/name…, for a test to
// compare.
func said(evs []Event) string {
	var b strings.Builder
	last := EventKind(-1)
	for _, ev := range evs {
		tag := map[EventKind]string{KText: "text", KThink: "think", KToolStart: "call", KToolArgs: "args", KStop: "stop", KError: "error"}[ev.Kind]
		if tag == "" {
			continue
		}
		if ev.Kind != last || ev.Kind == KToolStart {
			b.WriteString("|" + tag + ":")
		}
		last = ev.Kind
		switch ev.Kind {
		case KToolStart:
			b.WriteString(ev.ID + "/" + ev.Name)
		case KStop:
			b.WriteString(ev.Stop)
		default:
			b.WriteString(ev.Text)
		}
	}
	return b.String()
}
