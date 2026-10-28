//go:build windows

package term

import (
	"errors"
	"os"
	"syscall"
	"unsafe"
)

var (
	kernel32      = syscall.NewLazyDLL("kernel32.dll")
	procGetMode   = kernel32.NewProc("GetConsoleMode")
	procSetMode   = kernel32.NewProc("SetConsoleMode")
	procScreenBuf = kernel32.NewProc("GetConsoleScreenBufferInfo")
	procSetOutCP  = kernel32.NewProc("SetConsoleOutputCP")
	procSetInCP   = kernel32.NewProc("SetConsoleCP")
)

const (
	enableProcessedInput  = 0x0001
	enableLineInput       = 0x0002
	enableEchoInput       = 0x0004
	enableVTInput         = 0x0200
	enableProcessedOutput = 0x0001
	enableVTProcessing    = 0x0004
)

type coord struct{ X, Y int16 }
type smallRect struct{ Left, Top, Right, Bottom int16 }
type screenInfo struct {
	Size    coord
	Cursor  coord
	Attrs   uint16
	Window  smallRect
	MaxSize coord
}

func getMode(h syscall.Handle) (uint32, error) {
	var m uint32
	r, _, e := procGetMode.Call(uintptr(h), uintptr(unsafe.Pointer(&m)))
	if r == 0 {
		return 0, e
	}
	return m, nil
}

func setMode(h syscall.Handle, m uint32) error {
	r, _, e := procSetMode.Call(uintptr(h), uintptr(m))
	if r == 0 {
		return e
	}
	return nil
}

// IsTerminal reports whether f is a console.
func IsTerminal(f *os.File) bool {
	_, err := getMode(syscall.Handle(f.Fd()))
	return err == nil
}

// Size returns the visible console window size.
func Size(f *os.File) (cols, rows int, err error) {
	var info screenInfo
	r, _, e := procScreenBuf.Call(f.Fd(), uintptr(unsafe.Pointer(&info)))
	if r == 0 {
		return 0, 0, e
	}
	cols = int(info.Window.Right-info.Window.Left) + 1
	rows = int(info.Window.Bottom-info.Window.Top) + 1
	if cols <= 0 || rows <= 0 {
		return 0, 0, errors.New("console reported a zero size")
	}
	return cols, rows, nil
}

// MakeRaw puts the console input handle into raw, VT-input mode and returns
// a function restoring the previous mode.
func MakeRaw(f *os.File) (func(), error) {
	h := syscall.Handle(f.Fd())
	old, err := getMode(h)
	if err != nil {
		return nil, err
	}
	raw := (old &^ (enableEchoInput | enableLineInput | enableProcessedInput)) | enableVTInput
	if err := setMode(h, raw); err != nil {
		return nil, err
	}
	procSetInCP.Call(65001)
	return func() { _ = setMode(h, old) }, nil
}

// EnableVT turns on ANSI escape processing (and UTF-8) for the console
// output handle f, returning a function that restores the old mode.
func EnableVT(f *os.File) (func(), error) {
	h := syscall.Handle(f.Fd())
	old, err := getMode(h)
	if err != nil {
		return func() {}, err
	}
	if err := setMode(h, old|enableProcessedOutput|enableVTProcessing); err != nil {
		return func() {}, err
	}
	procSetOutCP.Call(65001)
	return func() { _ = setMode(h, old) }, nil
}

// OpenTTYInput opens the console input device, for when stdin is a pipe.
func OpenTTYInput() (*os.File, error) { return os.OpenFile("CONIN$", os.O_RDWR, 0) }
