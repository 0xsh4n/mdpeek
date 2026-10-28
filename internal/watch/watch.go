// Package watch reports changes to a single file by polling. Polling keeps
// mdpeek dependency-free and behaves identically on Windows, Linux and macOS
// (including network and container filesystems where inotify-style APIs are
// unreliable).
package watch

import (
	"context"
	"os"
	"time"
)

// File sends on the returned channel whenever path's modification time or
// size changes. Sends never block; bursts of changes coalesce into one event.
func File(ctx context.Context, path string, every time.Duration) <-chan struct{} {
	ch := make(chan struct{}, 1)
	var lastMod time.Time
	var lastSize int64 = -1
	if fi, err := os.Stat(path); err == nil {
		lastMod, lastSize = fi.ModTime(), fi.Size()
	}
	go func() {
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				fi, err := os.Stat(path)
				if err != nil {
					continue // briefly missing during an atomic save
				}
				if !fi.ModTime().Equal(lastMod) || fi.Size() != lastSize {
					lastMod, lastSize = fi.ModTime(), fi.Size()
					select {
					case ch <- struct{}{}:
					default:
					}
				}
			}
		}
	}()
	return ch
}
