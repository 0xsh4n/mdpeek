//go:build !(linux || darwin || freebsd || windows)

package term

import (
	"errors"
	"os"
)

var errUnsupported = errors.New("interactive terminal mode is not supported on this platform")

func IsTerminal(*os.File) bool          { return false }
func Size(*os.File) (int, int, error)   { return 0, 0, errUnsupported }
func MakeRaw(*os.File) (func(), error)  { return nil, errUnsupported }
func EnableVT(*os.File) (func(), error) { return func() {}, nil }
func OpenTTYInput() (*os.File, error)   { return nil, errUnsupported }
