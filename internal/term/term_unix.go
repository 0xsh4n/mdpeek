//go:build linux || darwin || freebsd

package term

import (
	"errors"
	"os"
	"syscall"
	"unsafe"
)

type winsize struct{ Row, Col, X, Y uint16 }

func ioctl(fd, req uintptr, arg unsafe.Pointer) error {
	_, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, req, uintptr(arg))
	if e != 0 {
		return e
	}
	return nil
}

// IsTerminal reports whether f is attached to a terminal.
func IsTerminal(f *os.File) bool {
	var t syscall.Termios
	return ioctl(f.Fd(), getTermios, unsafe.Pointer(&t)) == nil
}

// Size returns the terminal size in columns and rows.
func Size(f *os.File) (cols, rows int, err error) {
	var ws winsize
	if err = ioctl(f.Fd(), syscall.TIOCGWINSZ, unsafe.Pointer(&ws)); err != nil {
		return 0, 0, err
	}
	if ws.Col == 0 || ws.Row == 0 {
		return 0, 0, errors.New("terminal reported a zero size")
	}
	return int(ws.Col), int(ws.Row), nil
}

// MakeRaw switches f into raw mode and returns a function restoring the
// previous settings.
func MakeRaw(f *os.File) (func(), error) {
	fd := f.Fd()
	var old syscall.Termios
	if err := ioctl(fd, getTermios, unsafe.Pointer(&old)); err != nil {
		return nil, err
	}
	raw := old
	raw.Iflag &^= syscall.IGNBRK | syscall.BRKINT | syscall.PARMRK | syscall.ISTRIP |
		syscall.INLCR | syscall.IGNCR | syscall.ICRNL | syscall.IXON
	raw.Lflag &^= syscall.ECHO | syscall.ECHONL | syscall.ICANON | syscall.ISIG | syscall.IEXTEN
	raw.Cflag &^= syscall.CSIZE | syscall.PARENB
	raw.Cflag |= syscall.CS8
	raw.Cc[syscall.VMIN] = 1
	raw.Cc[syscall.VTIME] = 0
	if err := ioctl(fd, setTermios, unsafe.Pointer(&raw)); err != nil {
		return nil, err
	}
	return func() { _ = ioctl(fd, setTermios, unsafe.Pointer(&old)) }, nil
}

// EnableVT is a no-op on Unix: terminals understand ANSI sequences natively.
func EnableVT(*os.File) (func(), error) { return func() {}, nil }

// OpenTTYInput opens the controlling terminal for reading keys. It is used
// when stdin is a pipe (cat README.md | mdpeek).
func OpenTTYInput() (*os.File, error) { return os.Open("/dev/tty") }
