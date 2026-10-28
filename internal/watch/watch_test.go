package watch

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileReportsChanges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.md")
	if err := os.WriteFile(path, []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := File(ctx, path, 20*time.Millisecond)

	select {
	case <-ch:
		t.Fatal("event before any change")
	case <-time.After(100 * time.Millisecond):
	}

	if err := os.WriteFile(path, []byte("two, longer"), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("change not reported")
	}

	// Missing file (atomic save in progress) must not be reported as a change.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ch:
		t.Fatal("event for missing file")
	case <-time.After(100 * time.Millisecond):
	}
}
