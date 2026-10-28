package render

import (
	"os"
	"testing"
)

func readSample() (string, error) {
	b, err := os.ReadFile("../../testdata/sample.md")
	return string(b), err
}

func TestSampleDocumentRenders(t *testing.T) {
	src, err := readSample()
	if err != nil {
		t.Fatal(err)
	}
	// The sample has a 5-column table, which cannot fit in fewer than ~22 cells.
	for _, w := range []int{30, 50, 100} {
		res := colorRender(src, w)
		if len(res.Lines) < 20 || len(res.Headings) < 6 {
			t.Fatalf("width %d: suspiciously small output: %d lines, %d headings", w, len(res.Lines), len(res.Headings))
		}
		for _, l := range res.Lines {
			if Width(l) > w {
				t.Fatalf("width %d exceeded: %q", w, Strip(l))
			}
		}
	}
}
