package term

import (
	"strings"
	"testing"
	"time"
)

func collect(t *testing.T, input string) []Event {
	t.Helper()
	ch := Keys(strings.NewReader(input))
	var evs []Event
	timeout := time.After(2 * time.Second)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return evs
			}
			evs = append(evs, ev)
		case <-timeout:
			t.Fatal("timed out waiting for events")
		}
	}
}

func TestDecodeKeys(t *testing.T) {
	cases := []struct {
		in   string
		want []Event
	}{
		{"j", []Event{{KeyRune, 'j'}}},
		{"é", []Event{{KeyRune, 'é'}}},
		{"\x1b[A\x1b[B\x1b[C\x1b[D", []Event{{Key: KeyUp}, {Key: KeyDown}, {Key: KeyRight}, {Key: KeyLeft}}},
		{"\x1bOA\x1bOB", []Event{{Key: KeyUp}, {Key: KeyDown}}},
		{"\x1b[5~\x1b[6~\x1b[H\x1b[F", []Event{{Key: KeyPgUp}, {Key: KeyPgDn}, {Key: KeyHome}, {Key: KeyEnd}}},
		{"\x1b[1~\x1b[4~\x1b[3~", []Event{{Key: KeyHome}, {Key: KeyEnd}, {Key: KeyDelete}}},
		{"\x1b[1;5A", []Event{{Key: KeyUp}}}, // modified arrow
		{"\r\x7f\t\x03", []Event{{Key: KeyEnter}, {Key: KeyBackspace}, {Key: KeyTab}, {Key: KeyCtrlC}}},
		{"\x1b", []Event{{Key: KeyEsc}}},
		{"q\x1b", []Event{{KeyRune, 'q'}, {Key: KeyEsc}}},
	}
	for _, c := range cases {
		got := collect(t, c.in)
		if len(got) != len(c.want) {
			t.Errorf("%q: got %v, want %v", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%q: event %d = %v, want %v", c.in, i, got[i], c.want[i])
			}
		}
	}
}
