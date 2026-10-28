// Package term contains the small amount of cross-platform terminal plumbing
// mdpeek needs: raw mode, window size, and key decoding. It depends only on
// the standard library.
package term

import (
	"bufio"
	"io"
	"time"
)

// Key identifies a decoded key press.
type Key int

const (
	KeyRune Key = iota
	KeyEnter
	KeyEsc
	KeyBackspace
	KeyTab
	KeyUp
	KeyDown
	KeyLeft
	KeyRight
	KeyPgUp
	KeyPgDn
	KeyHome
	KeyEnd
	KeyDelete
	KeyCtrlB
	KeyCtrlC
	KeyCtrlD
	KeyCtrlF
	KeyCtrlL
	KeyCtrlU
)

// Event is one key press. Rune is set when Key == KeyRune.
type Event struct {
	Key  Key
	Rune rune
}

const escWait = 30 * time.Millisecond

// Keys decodes r (a raw-mode terminal) into key events. The channel is closed
// when r returns an error or EOF.
func Keys(r io.Reader) <-chan Event {
	runes := make(chan rune, 64)
	go func() {
		defer close(runes)
		br := bufio.NewReader(r)
		for {
			c, _, err := br.ReadRune()
			if err != nil {
				return
			}
			runes <- c
		}
	}()
	out := make(chan Event, 16)
	go func() {
		defer close(out)
		decode(runes, out, escWait)
	}()
	return out
}

func decode(in <-chan rune, out chan<- Event, wait time.Duration) {
	for c := range in {
		switch {
		case c == 0x1b:
			select {
			case c2, ok := <-in:
				if !ok {
					out <- Event{Key: KeyEsc}
					return
				}
				switch c2 {
				case '[':
					if ev, ok := decodeCSI(in, wait); ok {
						out <- ev
					}
				case 'O':
					select {
					case c3, ok := <-in:
						if ok {
							if ev, ok := finalKey(c3, ""); ok {
								out <- ev
							}
						}
					case <-time.After(wait):
					}
				default:
					// Alt+key: ignore.
				}
			case <-time.After(wait):
				out <- Event{Key: KeyEsc}
			}
		case c == '\r' || c == '\n':
			out <- Event{Key: KeyEnter}
		case c == 0x7f || c == 0x08:
			out <- Event{Key: KeyBackspace}
		case c == '\t':
			out <- Event{Key: KeyTab}
		case c == 0x03:
			out <- Event{Key: KeyCtrlC}
		case c == 0x04:
			out <- Event{Key: KeyCtrlD}
		case c == 0x02:
			out <- Event{Key: KeyCtrlB}
		case c == 0x06:
			out <- Event{Key: KeyCtrlF}
		case c == 0x0c:
			out <- Event{Key: KeyCtrlL}
		case c == 0x15:
			out <- Event{Key: KeyCtrlU}
		case c < 0x20:
			// other control characters: ignore
		default:
			out <- Event{Key: KeyRune, Rune: c}
		}
	}
}

func decodeCSI(in <-chan rune, wait time.Duration) (Event, bool) {
	var params []rune
	for {
		select {
		case c, ok := <-in:
			if !ok {
				return Event{}, false
			}
			if c >= 0x40 && c <= 0x7e {
				return finalKey(c, string(params))
			}
			params = append(params, c)
		case <-time.After(wait):
			return Event{}, false
		}
	}
}

func finalKey(final rune, params string) (Event, bool) {
	switch final {
	case 'A':
		return Event{Key: KeyUp}, true
	case 'B':
		return Event{Key: KeyDown}, true
	case 'C':
		return Event{Key: KeyRight}, true
	case 'D':
		return Event{Key: KeyLeft}, true
	case 'H':
		return Event{Key: KeyHome}, true
	case 'F':
		return Event{Key: KeyEnd}, true
	case '~':
		n := 0
		for _, d := range params {
			if d < '0' || d > '9' {
				break
			}
			n = n*10 + int(d-'0')
		}
		switch n {
		case 1, 7:
			return Event{Key: KeyHome}, true
		case 4, 8:
			return Event{Key: KeyEnd}, true
		case 3:
			return Event{Key: KeyDelete}, true
		case 5:
			return Event{Key: KeyPgUp}, true
		case 6:
			return Event{Key: KeyPgDn}, true
		}
	}
	return Event{}, false
}
